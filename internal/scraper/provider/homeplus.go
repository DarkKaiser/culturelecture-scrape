package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/PuerkitoBio/goquery"
	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/notify-server/pkg/strutil"
)

const homeplusLectureSearchPageSize = 20

type Homeplus struct {
	name           string
	cultureBaseUrl string

	storeCodeMap        map[string]string // 점포
	lectureGroupCodeMap map[string]string // 강좌군
}

type homeplusStoreSearchResult struct {
	RstCode    int    `json:"RstCode"`
	RstMessage string `json:"RstMessage"`
	Data       struct {
		StoreList []struct {
			StoreAreaName        string      `json:"StoreAreaName"`
			StoreCode            string      `json:"StoreCode"`
			StoreName            string      `json:"StoreName"`
			RegionHQID           int         `json:"RegionHQID"`
			SortingNumber        int         `json:"SortingNumber"`
			RealStoreCode        string      `json:"RealStoreCode"`
			PhoneNumber          string      `json:"PhoneNumber"`
			FaxNumber            interface{} `json:"FaxNumber"`
			ZipCode              string      `json:"ZipCode"`
			Address1             string      `json:"Address1"`
			Address2             string      `json:"Address2"`
			AddressPrevVer       string      `json:"AddressPrevVer"`
			OperaterName         interface{} `json:"OperaterName"`
			OperatorMobileNumber string      `json:"OperatorMobileNumber"`
		} `json:"StoreList"`
		MyStoreList []struct {
			StoreAreaName        interface{} `json:"StoreAreaName"`
			StoreCode            string      `json:"StoreCode"`
			StoreName            string      `json:"StoreName"`
			RegionHQID           int         `json:"RegionHQID"`
			SortingNumber        int         `json:"SortingNumber"`
			RealStoreCode        interface{} `json:"RealStoreCode"`
			PhoneNumber          interface{} `json:"PhoneNumber"`
			FaxNumber            interface{} `json:"FaxNumber"`
			ZipCode              interface{} `json:"ZipCode"`
			Address1             interface{} `json:"Address1"`
			Address2             interface{} `json:"Address2"`
			AddressPrevVer       interface{} `json:"AddressPrevVer"`
			OperaterName         interface{} `json:"OperaterName"`
			OperatorMobileNumber interface{} `json:"OperatorMobileNumber"`
		} `json:"MyStoreList"`
	} `json:"Data"`
}

func NewHomeplus() *Homeplus {
	return &Homeplus{
		name: "홈플러스",

		cultureBaseUrl: "https://mschool.homeplus.co.kr",

		storeCodeMap: map[string]string{
			"0035": "광양점",
			"0030": "순천점",
		},

		lectureGroupCodeMap: map[string]string{
			"MH|EL|IF": "Kids 전체",
			"BB":       "Baby 전체",
		},
	}
}

func (h *Homeplus) ScrapeCultureLectures(mainC chan<- []domain.Lecture) error {
	log.Printf("%s 문화센터 강좌 수집을 시작합니다.", h.name)

	// 점포가 유효한지 확인한다.
	validStore, err := h.validCultureLectureStore()
	if err != nil {
		return fmt.Errorf("%s 문화센터 점포 검증 실패: %v", h.name, err)
	}
	if !validStore {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(점포코드 불일치)", h.name)
	}
	// 강좌군이 유효한지 확인한다.
	validLG, err := h.validCultureLectureGroup()
	if err != nil {
		return fmt.Errorf("%s 문화센터 강좌군 검증 실패: %v", h.name, err)
	}
	if !validLG {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, 강좌군코드 불일치)", h.name)
	}

	var wait sync.WaitGroup

	c := make(chan *domain.Lecture, 100)

	var totalExtractionLectureCount int64 = 0
	for storeCode, storeName := range h.storeCodeMap {
		// 불러올 전체 강좌 갯수를 구한다.
		_, doc, err := h.cultureLecturePageDocument(1, storeCode, storeName)
		if err != nil {
			return fmt.Errorf("%s 문화센터 전체 강좌 갯수 확인용 페이지 로드 실패(점포:%s): %v", h.name, storeName, err)
		}
		value := doc.Find("#divTotalCnt").Text()
		if len(value) == 0 {
			return fmt.Errorf("%s 문화센터 강좌를 수집하는 중에 전체 강좌 갯수 추출이 실패하였습니다.", h.name)
		}
		totalLectureCount, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("전체 강좌 갯수 파싱 오류: %v", err)
		}

		// 불러올 전체 페이지 갯수를 구한다.
		totalPageCount := int(math.Ceil(float64(totalLectureCount) / homeplusLectureSearchPageSize))

		// 강좌 데이터를 수집한다.
		for pageNo := 1; pageNo <= totalPageCount; pageNo++ {
			wait.Add(1)
			go func(storeCode string, storeName string, pageNo int) {
				defer wait.Done()

				clPageUrl, doc, err := h.cultureLecturePageDocument(pageNo, storeCode, storeName)
				if err != nil {
					log.Fatalf("%s 문화센터(%s) 페이지(pageNo:%d) 로드 실패로 종료합니다: %v", h.name, storeName, pageNo, err)
				}

				clSelection := doc.Find("li > div.result_info_wrap")
				clSelection.Each(func(i int, s *goquery.Selection) {
					atomic.AddInt64(&totalExtractionLectureCount, 1)
					go func(s *goquery.Selection) {
						err := h.extractCultureLecture(clPageUrl, storeName, s, c)
						if err != nil {
							log.Fatalf("%s 문화센터 강좌 추출 오류로 종료합니다: %v", h.name, err)
						}
					}(s)
				})
			}(storeCode, storeName, pageNo)
		}
	}

	wait.Wait()

	var lectureList []domain.Lecture
	for i := int64(0); i < totalExtractionLectureCount; i++ {
		lecture := <-c
		if len(lecture.Title) > 0 {
			lectureList = append(lectureList, *lecture)
		}
	}

	log.Printf("%s 문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", h.name, len(lectureList))

	mainC <- lectureList

	return nil
}

func (h *Homeplus) cultureLecturePageDocument(pageNo int, storeCode, storeName string) (string, *goquery.Document, error) {
	clPageUrl := fmt.Sprintf("%s/Lecture/GetSearchResult", h.cultureBaseUrl)

	var paramIdx = 0
	reqBodyString := fmt.Sprintf("page=%d", pageNo)
	reqBodyString += fmt.Sprintf("&pageSize=%d", homeplusLectureSearchPageSize)
	reqBodyString += h.generateLectureSearchParamString(paramIdx, "", storeName, storeCode, "")
	for lectureGroupCode, lectureGroupName := range h.lectureGroupCodeMap {
		paramIdx++
		reqBodyString += h.generateLectureSearchParamString(paramIdx, "", lectureGroupName, "", lectureGroupCode)
	}
	reqBodyString += "&word="
	reqBodyString += "&sort=1"

	reqBody := bytes.NewBufferString(reqBodyString)
	res, err := http.Post(clPageUrl, "application/x-www-form-urlencoded; charset=UTF-8", reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("http.Post failed: %v", err)
	}
	if res.StatusCode != 200 {
		return "", nil, fmt.Errorf("Request failed with Status: %d", res.StatusCode)
	}

	//goland:noinspection GoUnhandledErrorResult
	defer res.Body.Close()

	resBodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return "", nil, fmt.Errorf("io.ReadAll failed: %v", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(resBodyBytes)))
	if err != nil {
		return "", nil, fmt.Errorf("goquery.NewDocumentFromReader failed: %v", err)
	}

	return clPageUrl, doc, nil
}

func (h *Homeplus) generateLectureSearchParamString(paramIdx int, id, txt, storeCode, lectureGroupCode string) string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("&prm[%d][Id]=%s", paramIdx, id))
	b.WriteString(fmt.Sprintf("&prm[%d][Txt]=%s", paramIdx, txt))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][StoreCode]=%s", paramIdx, storeCode))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][LectureTarget]=%s", paramIdx, lectureGroupCode))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][LectureGroup]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][LectureType]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][LectureWeek]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][ClassCount]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][LectureTime]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][LectureStatusSearch]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][LectureStartMonth]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][DeadLine]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][Confirmed]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][Discount]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][LectureTimeGroup]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][LectureAge]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][LectureOnly]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][WebTheme]=", paramIdx))
	b.WriteString(fmt.Sprintf("&prm[%d][Data][Description]=", paramIdx))
	return b.String()
}

func (h *Homeplus) extractCultureLecture(clPageUrl string, storeName string, s *goquery.Selection, c chan<- *domain.Lecture) error {
	// 강좌 그룹
	title1 := strutil.NormalizeSpace(s.Find("div.title_1").Text())
	// 강좌명
	title2 := strutil.NormalizeSpace(s.Find("div.title_2").Text())
	// 요일/시간, 형식 : 일 14:20 ~ 15:00
	info4 := strutil.NormalizeSpace(s.Find("div.info_4").Text())

	ls := s.Find("div.info_5")
	if ls.Length() != 3 {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 컬럼 개수 불일치:%d, URL:%s)", h.name, ls.Length(), clPageUrl)
	}
	// 강좌횟수/수강료, 형식 : 1회 6,000원
	info5Idx0 := strutil.NormalizeSpace(ls.Eq(0).Text())
	// 개강일, 형식 : 2023.08.20 ~ 2023.08.20
	info5Idx1 := strutil.NormalizeSpace(ls.Eq(1).Text())
	// 강사명, 형식 : 신혜정 강사
	info5Idx2 := strutil.NormalizeSpace(ls.Eq(2).Text())

	// 강좌그룹
	if len(title1) == 0 {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 그룹명이 빈 문자열입니다, URL:%s)", h.name, clPageUrl)
	}
	group := title1

	// 강좌명
	if len(title2) == 0 {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌명이 빈 문자열입니다, URL:%s)", h.name, clPageUrl)
	}
	title := title2

	// 강사
	teacher := strutil.NormalizeSpace(regexp.MustCompile("^(.)*강사").FindString(info5Idx2))
	if len(teacher) == 0 {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5Idx2, clPageUrl)
	}

	// 개강일
	startDate := strutil.NormalizeSpace(regexp.MustCompile("[0-9]{4}.[0-9]{2}.[0-9]{2} ~").FindString(info5Idx1))
	if len(startDate) == 0 {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5Idx1, clPageUrl)
	}
	startDate = strings.ReplaceAll(startDate[:len(startDate)-2], ".", "-")

	// 시작시간, 종료시간
	startTime := regexp.MustCompile("[0-9]{2}:[0-9]{2} ~").FindString(info4)
	endTime := regexp.MustCompile("~ [0-9]{2}:[0-9]{2}").FindString(info4)
	if len(startTime) == 0 || len(endTime) == 0 {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info4, clPageUrl)
	}
	startTime = strutil.NormalizeSpace(startTime[:len(startTime)-1])
	endTime = strutil.NormalizeSpace(endTime[1:])

	// 요일
	dayOfTheWeek := strutil.NormalizeSpace(regexp.MustCompile("^[월화수목금토일] ").FindString(info4))
	if len(dayOfTheWeek) == 0 {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info4, clPageUrl)
	}

	// 수강료

	// '1회 7,000원 (2인 기준)' => '1회 7,000원'
	info5Idx0 = strutil.NormalizeSpace(regexp.MustCompile(`\s*\(\d+인 기준\)`).ReplaceAllString(info5Idx0, ""))

	price := strutil.NormalizeSpace(regexp.MustCompile(" [0-9]{1,3}(,[0-9]{3})*원$").FindString(info5Idx0))
	if len(price) == 0 {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5Idx0, clPageUrl)
	}

	// 강좌횟수
	count := strutil.NormalizeSpace(regexp.MustCompile("^[0-9]{1,3}회").FindString(info5Idx0))
	if len(count) == 0 {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5Idx0, clPageUrl)
	}

	// 접수상태
	classCartImgUrl, exists := s.Find("button.btn_class_cart > img").Attr("src")
	if exists == false {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(접수상태 추출이 실패하였습니다, URL:%s)", h.name, clPageUrl)
	}
	classCartStatus := strutil.NormalizeSpace(s.Find("button.btn_class_cart > span:last-child").Text())

	var status = domain.ReceptionStatusUnknown
	switch classCartImgUrl {
	case "/images/ico/icon_cart_3.png":
		if classCartStatus == "대기" {
			status = domain.ReceptionStatusStnadBy
		} else if classCartStatus == "강의 장바구니 담기" {
			status = domain.ReceptionStatusPossible
		} else {
			return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", h.name, classCartImgUrl, clPageUrl)
		}
	case "/images/ico/icon_cart_4.png":
		if classCartStatus == "마감" {
			status = domain.ReceptionStatusClosed
		} else if classCartStatus == "방문" {
			status = domain.ReceptionStatusVisitConsultation
		} else if classCartStatus == "문의" {
			status = domain.ReceptionStatusVisitInquiry
		} else {
			return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", h.name, classCartImgUrl, clPageUrl)
		}
	default:
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", h.name, classCartImgUrl, clPageUrl)
	}

	// 상세페이지로 이동하기 위한 LectureMasterID를 구한다.
	idSelection := s.Find("input[name=LectureMasterID]")
	lectureMasterId, exists := idSelection.Attr("value")
	if exists == false {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지로 이동하기 위해 필요한 [ LectureMasterID ] 값이 비어 있습니다, URL:%s)", h.name, clPageUrl)
	}

	c <- &domain.Lecture{
		StoreName:      fmt.Sprintf("%s %s", h.name, storeName),
		Group:          group,
		Title:          title,
		Teacher:        teacher,
		StartDate:      startDate,
		StartTime:      startTime,
		EndTime:        endTime,
		DayOfTheWeek:   fmt.Sprintf("%s요일", dayOfTheWeek),
		Price:          price,
		Count:          count,
		Status:         status,
		DetailPageUrl:  fmt.Sprintf("%s/Lecture/Detail?LectureMasterID=%s", h.cultureBaseUrl, strutil.NormalizeSpace(lectureMasterId)),
		ScrapeExcluded: false,
	}

	return nil
}

func (h *Homeplus) validCultureLectureStore() (bool, error) {
	res, err := http.Post(fmt.Sprintf("%s/Store/GetStoreList", h.cultureBaseUrl), "application/json; charset=utf-8", nil)
	if err != nil {
		return false, fmt.Errorf("http.Post failed: %v", err)
	}
	if res.StatusCode != 200 {
		return false, fmt.Errorf("Request failed with Status: %d", res.StatusCode)
	}

	//goland:noinspection GoUnhandledErrorResult
	defer res.Body.Close()

	resBodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return false, fmt.Errorf("io.ReadAll failed: %v", err)
	}

	var storeSearchResult homeplusStoreSearchResult
	err = json.Unmarshal(resBodyBytes, &storeSearchResult)
	if err != nil {
		return false, fmt.Errorf("json.Unmarshal failed: %v", err)
	}

	for storeCode, storeName := range h.storeCodeMap {
		foundStore := false
		for _, elem := range storeSearchResult.Data.StoreList {
			if storeCode == elem.StoreCode && storeName == elem.StoreName {
				foundStore = true
				break
			}
		}
		if foundStore == false {
			return false, nil
		}
	}

	return true, nil
}

func (h *Homeplus) validCultureLectureGroup() (bool, error) {
	res, err := http.Get(fmt.Sprintf("%s/Lecture/Search", h.cultureBaseUrl))
	if err != nil {
		return false, fmt.Errorf("http.Get failed: %v", err)
	}
	if res.StatusCode != 200 {
		return false, fmt.Errorf("Request failed with Status: %d", res.StatusCode)
	}

	//goland:noinspection GoUnhandledErrorResult
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return false, fmt.Errorf("goquery.NewDocumentFromReader failed: %v", err)
	}

	for lectureGroupCode, lectureGroupName := range h.lectureGroupCodeMap {
		lectureGroupSelection := doc.Find(fmt.Sprintf("section.search_body div.menu_depth_2_wrap ul.tree_menu_2 > li.depth_2 > ul.depth_3 > li:first-child > button[data-lecture-target='%s']", lectureGroupCode))
		if lectureGroupSelection.Length() != 1 {
			return false, nil
		}

		val := lectureGroupSelection.Text()
		if strutil.NormalizeSpace(val) != lectureGroupName {
			return false, nil
		}
	}

	return true, nil
}
