package culture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/darkkaiser/culturelecture-scrape/scrape/lectures"
	"github.com/darkkaiser/culturelecture-scrape/utils"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const homeplusLectureSearchPageSize = 20

type homeplus struct {
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

func NewHomeplus() *homeplus {
	return &homeplus{
		name: "홈플러스",

		cultureBaseUrl: "https://mschool.homeplus.co.kr",

		storeCodeMap: map[string]string{
			"0035": "광양점",
			"1009": "순천풍덕점",
			"0030": "순천점",
		},

		lectureGroupCodeMap: map[string]string{
			"MH|EL|IF": "Kids 전체",
			"BB":       "Baby 전체",
		},
	}
}

func (h *homeplus) ScrapeCultureLectures(mainC chan<- []lectures.Lecture) {
	log.Printf("%s 문화센터 강좌 수집을 시작합니다.", h.name)

	// 점포가 유효한지 확인한다.
	if h.validCultureLectureStore() == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(점포코드 불일치)", h.name)
	}
	// 강좌군이 유효한지 확인한다.
	if h.validCultureLectureGroup() == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, 강좌군코드 불일치)", h.name)
	}

	var wait sync.WaitGroup

	c := make(chan *lectures.Lecture, 100)

	var totalExtractionLectureCount int64 = 0
	for storeCode, storeName := range h.storeCodeMap {
		// 불러올 전체 강좌 갯수를 구한다.
		_, doc := h.cultureLecturePageDocument(1, storeCode, storeName)
		value := doc.Find("#divTotalCnt").Text()
		if len(value) == 0 {
			log.Fatalf("%s 문화센터 강좌를 수집하는 중에 전체 강좌 갯수 추출이 실패하였습니다.", h.name)
		}
		totalLectureCount, err := strconv.Atoi(value)
		utils.CheckErr(err)

		// 불러올 전체 페이지 갯수를 구한다.
		totalPageCount := int(math.Ceil(float64(totalLectureCount) / homeplusLectureSearchPageSize))

		// 강좌 데이터를 수집한다.
		for pageNo := 1; pageNo <= totalPageCount; pageNo++ {
			wait.Add(1)
			go func(storeCode string, storeName string, pageNo int) {
				defer wait.Done()

				clPageUrl, doc := h.cultureLecturePageDocument(pageNo, storeCode, storeName)

				clSelection := doc.Find("li > div.result_info_wrap")
				clSelection.Each(func(i int, s *goquery.Selection) {
					atomic.AddInt64(&totalExtractionLectureCount, 1)
					go h.extractCultureLecture(clPageUrl, storeCode, storeName, s, c)
				})
			}(storeCode, storeName, pageNo)
		}
	}

	wait.Wait()

	var lectureList []lectures.Lecture
	for i := int64(0); i < totalExtractionLectureCount; i++ {
		lecture := <-c
		if len(lecture.Title) > 0 {
			lectureList = append(lectureList, *lecture)
		}
	}

	log.Printf("%s 문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", h.name, len(lectureList))

	mainC <- lectureList
}

func (h *homeplus) cultureLecturePageDocument(pageNo int, storeCode, storeName string) (string, *goquery.Document) {
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
	utils.CheckErr(err)
	utils.CheckStatusCode(res)

	//goland:noinspection GoUnhandledErrorResult
	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	utils.CheckErr(err)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(resBodyBytes)))
	utils.CheckErr(err)

	return clPageUrl, doc
}

func (h *homeplus) generateLectureSearchParamString(paramIdx int, id, txt, storeCode, lectureGroupCode string) string {
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

func (h *homeplus) extractCultureLecture(clPageUrl string, storeCode string, storeName string, s *goquery.Selection, c chan<- *lectures.Lecture) {
	// 강좌 그룹
	title1 := utils.CleanString(s.Find("div.title_1").Text())
	// 강좌명
	title2 := utils.CleanString(s.Find("div.title_2").Text())
	// 강좌 수강대상 및 기타 정보
	info3 := utils.CleanString(s.Find("div.info_3").Text())
	// 강사명/요일/시간, 형식 : 신혜정 강사 / 일 14:20 ~ 15:00
	info4 := utils.CleanString(s.Find("div.info_4").Text())
	// 강좌횟수/수강료/개강일, 형식 : 1회 6,000원 (2023-08-20~2023-08-20)
	info5 := utils.CleanString(s.Find("div.info_5").Text())

	// 강좌그룹
	if len(title1) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 그룹명이 빈 문자열입니다, URL:%s)", h.name, clPageUrl)
	}
	group := title1

	// 강좌명
	if len(title2) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌명이 빈 문자열입니다, URL:%s)", h.name, clPageUrl)
	}
	title := utils.CleanString(fmt.Sprintf("%s %s ", title2, info3))

	// 강사
	teacher := utils.CleanString(regexp.MustCompile("^(.)*강사").FindString(info4))
	if len(teacher) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info4, clPageUrl)
	}

	// 개강일
	startDate := utils.CleanString(regexp.MustCompile("[0-9]{4}-[0-9]{2}-[0-9]{2}~").FindString(info5))
	if len(startDate) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5, clPageUrl)
	}
	startDate = startDate[:len(startDate)-1]

	// 시작시간, 종료시간
	startTime := regexp.MustCompile("[0-9]{2}:[0-9]{2} ~").FindString(info4)
	endTime := regexp.MustCompile("~ [0-9]{2}:[0-9]{2}").FindString(info4)
	if len(startTime) == 0 || len(endTime) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info4, clPageUrl)
	}
	startTime = utils.CleanString(startTime[:len(startTime)-1])
	endTime = utils.CleanString(endTime[1:])

	// 요일
	dayOfTheWeek := utils.CleanString(regexp.MustCompile(" [월화수목금토일] ").FindString(info4))
	if len(dayOfTheWeek) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info4, clPageUrl)
	}

	// 수강료
	price := utils.CleanString(regexp.MustCompile(" [0-9]{1,3}(,[0-9]{3})*원").FindString(info5))
	if len(price) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5, clPageUrl)
	}

	// 강좌횟수
	count := utils.CleanString(regexp.MustCompile("^[0-9]{1,3}회").FindString(info5))
	if len(count) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5, clPageUrl)
	}

	// 접수상태
	classCartImgUrl, exists := s.Find("button.btn_class_cart > img").Attr("src")
	if exists == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(접수상태 추출이 실패하였습니다, URL:%s)", h.name, clPageUrl)
	}
	classCartStatus := utils.CleanString(s.Find("button.btn_class_cart > span:last-child").Text())

	var status = lectures.ReceptionStatusUnknown
	switch classCartImgUrl {
	case "/images/ico/icon_cart_3.png":
		if classCartStatus == "대기" {
			status = lectures.ReceptionStatusStnadBy
		} else if classCartStatus == "강의 장바구니 담기" {
			status = lectures.ReceptionStatusPossible
		} else {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", h.name, classCartImgUrl, clPageUrl)
		}
	case "/images/ico/icon_cart_4.png":
		if classCartStatus == "마감" {
			status = lectures.ReceptionStatusClosed
		} else if classCartStatus == "방문" {
			status = lectures.ReceptionStatusVisitConsultation
		} else if classCartStatus == "문의" {
			status = lectures.ReceptionStatusVisitInquiry
		} else {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", h.name, classCartImgUrl, clPageUrl)
		}
	default:
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", h.name, classCartImgUrl, clPageUrl)
	}

	// 상세페이지로 이동하기 위한 LectureMasterID를 구한다.
	idSelection := s.Find("input[name=LectureMasterID]")
	lectureMasterId, exists := idSelection.Attr("value")
	if exists == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지로 이동하기 위해 필요한 [ LectureMasterID ] 값이 비어 있습니다, URL:%s)", h.name, clPageUrl)
	}

	c <- &lectures.Lecture{
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
		DetailPageUrl:  fmt.Sprintf("%s/Lecture/Detail?LectureMasterID=%s", h.cultureBaseUrl, utils.CleanString(lectureMasterId)),
		ScrapeExcluded: false,
	}
}

func (h *homeplus) validCultureLectureStore() bool {
	res, err := http.Post(fmt.Sprintf("%s/Store/GetStoreList", h.cultureBaseUrl), "application/json; charset=utf-8", nil)
	utils.CheckErr(err)
	utils.CheckStatusCode(res)

	//goland:noinspection GoUnhandledErrorResult
	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	utils.CheckErr(err)

	var storeSearchResult homeplusStoreSearchResult
	err = json.Unmarshal(resBodyBytes, &storeSearchResult)
	utils.CheckErr(err)

	for storeCode, storeName := range h.storeCodeMap {
		foundStore := false
		for _, elem := range storeSearchResult.Data.StoreList {
			if storeCode == elem.StoreCode && storeName == elem.StoreName {
				foundStore = true
				break
			}
		}
		if foundStore == false {
			return false
		}
	}

	return true
}

func (h *homeplus) validCultureLectureGroup() bool {
	res, err := http.Get(fmt.Sprintf("%s/Lecture/Search", h.cultureBaseUrl))
	utils.CheckErr(err)
	utils.CheckStatusCode(res)

	//goland:noinspection GoUnhandledErrorResult
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	utils.CheckErr(err)

	for lectureGroupCode, lectureGroupName := range h.lectureGroupCodeMap {
		lectureGroupSelection := doc.Find(fmt.Sprintf("section.search_body div.menu_depth_2_wrap ul.tree_menu_2 > li.depth_2 > ul.depth_3 > li:first-child > button[data-lecture-target='%s']", lectureGroupCode))
		if lectureGroupSelection.Length() != 1 {
			return false
		}

		val := lectureGroupSelection.Text()
		if utils.CleanString(val) != lectureGroupName {
			return false
		}
	}

	return true
}
