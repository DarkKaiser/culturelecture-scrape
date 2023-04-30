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
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
)

type homeplus struct {
	name           string
	cultureBaseUrl string

	storeCodeMap        map[string]string // 점포
	lectureGroupCodeMap map[string]string // 강좌군
}

/*
 * 강좌 검색 POST 데이터
 */
type homeplusLectureSearchPostData struct {
	Param [15]string `json:"prm"`
}

/*
 * Convert JSON to Go struct : https://mholt.github.io/json-to-go/
 */
type homeplusLectureSearchResult struct {
	Table  []homeplusLectureSearchResultData `json:"Table"`
	Table1 []struct {
		Query      interface{} `json:"query"`
		TotalCount string      `json:"totalCount"`
		Collection string      `json:"collection"`
		Sort       string      `json:"sort"`
		StartCount string      `json:"startCount"`
		ViewCount  string      `json:"viewCount"`
	} `json:"Table1"`
}

type homeplusLectureSearchResultData struct {
	LectureMasterID         string      `json:"LectureMasterID"`
	StoreCode               string      `json:"StoreCode"`
	YYYY                    int         `json:"YYYY"`
	Season                  int         `json:"Season"`
	LectureName             string      `json:"LectureName"`
	ComLectureName          string      `json:"ComLectureName"`
	LectureTypeSearch       interface{} `json:"LectureTypeSearch"`
	LectureTypeView         interface{} `json:"LectureTypeView"`
	LectureTagetSearch      interface{} `json:"LectureTagetSearch"`
	LectureTargetView       string      `json:"LectureTargetView"`
	LectureGroupSearch      interface{} `json:"LectureGroupSearch"`
	LectureGroupView        interface{} `json:"LectureGroupView"`
	LectureImage            string      `json:"LectureImage"`
	SubLectureName1         string      `json:"SubLectureName1"`
	SubLectureName2         string      `json:"SubLectureName2"`
	TeacherName             string      `json:"TeacherName"`
	AgeLectureFr            string      `json:"AgeLectureFr"`
	AgeLectureTo            string      `json:"AgeLectureTo"`
	DateLectureFr           string      `json:"DateLectureFr"`
	DateLectureTo           string      `json:"DateLectureTo"`
	ClassCount              string      `json:"ClassCount"`
	LectureDC               interface{} `json:"LectureDC"`
	LectureWeek             string      `json:"LectureWeek"`
	LectureTime             string      `json:"LectureTime"`
	LectureWebIcon          string      `json:"LectureWebIcon"`
	LectureMobileIcon       string      `json:"LectureMobileIcon"`
	LectureWeekOrder        string      `json:"LectureWeekOrder"`
	LectureWeekName         string      `json:"LectureWeekName"`
	LectureStatus           string      `json:"LectureStatus"`
	LectureStatusName       string      `json:"LectureStatusName"`
	LectureStatusDecription string      `json:"LectureStatusDecription"`
	TuitionFeeDC            string      `json:"TuitionFeeDC"`
	TuitionFeeDCFormating   string      `json:"TuitionFeeDCFormating"`
	MinMember               string      `json:"MinMember"`
	AdmitLimitType          string      `json:"AdmitLimitType"`
	AdmitDateFrom           string      `json:"AdmitDateFrom"`
	AdmitDateTo             string      `json:"AdmitDateTo"`
	AdmitValid              string      `json:"AdmitValid"`
	IsOnlyLecture           string      `json:"IsOnlyLecture"`
	DeadLine                string      `json:"DeadLine"`
	Temp1                   string      `json:"Temp1"`
}

type homeplusStoreSearchResult struct {
	Table []struct {
		RowSeq    int    `json:"RowSeq"`
		StoreCode string `json:"StoreCode"`
		StoreName string `json:"StoreName"`
		IsMyStore string `json:"IsMyStore"`
		Latitude  string `json:"Latitude"`
		Longitude string `json:"Longitude"`
	} `json:"Table"`
	Table1 []struct {
		RowSeq        int    `json:"RowSeq"`
		StoreAreaName string `json:"StoreAreaName"`
		StoreInfos    string `json:"StoreInfos"`
	} `json:"Table1"`
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
			"IF": "유아",
			"BB": "영아",
		},
	}
}

func (h *homeplus) ScrapeCultureLectures(mainC chan<- []lectures.Lecture) {
	log.Printf("%s 문화센터 강좌 수집을 시작합니다.", h.name)

	// 점포가 유효한지 확인한다.
	if h.validCultureLectureStore() == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, 점포코드 불일치)", h.name)
	}
	// 강좌군이 유효한지 확인한다.
	if h.validCultureLectureGroup() == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, 강좌군코드 불일치)", h.name)
	}

	c := make(chan *lectures.Lecture, 100)

	clPageUrl := h.cultureBaseUrl + "/Lecture/GetSearchLecture"

	// 각 점포 및 강좌군의 검색까지 병렬화(goroutine)하면, 검색 결과의 데이터 갯수가 매번 다르게 반환되어 오류가 발생하므로 병렬화를 하지 않음!!!
	// 홈플러스 서버에 병렬화하여 동시에 요청을 많이 보낸 경우, 제대로 응답이 들어오는 경우도 있지만 그렇지 못하는 경우가 발생함!!!
	var totalExtractionLectureCount int64 = 0
	for storeCode, storeName := range h.storeCodeMap {
		for lectureGroupCode, lectureGroupName := range h.lectureGroupCodeMap {
			idx := 1
			extractionLectureCount := 0
			for {
				lspd := h.newLectureSearchPostData(storeCode, lectureGroupCode, idx)
				lspdJSONBytes, err := json.Marshal(lspd)
				utils.CheckErr(err)

				reqBody := bytes.NewBuffer(lspdJSONBytes)
				res, err := http.Post(clPageUrl, "application/json; charset=utf-8", reqBody)
				utils.CheckErr(err)
				utils.CheckStatusCode(res)

				resBodyBytes, err := ioutil.ReadAll(res.Body)
				utils.CheckErr(err)

				utils.CheckErr(res.Body.Close())

				var lectureSearchResult homeplusLectureSearchResult
				err = json.Unmarshal(resBodyBytes, &lectureSearchResult)
				utils.CheckErr(err)

				for i := range lectureSearchResult.Table {
					extractionLectureCount += 1
					atomic.AddInt64(&totalExtractionLectureCount, 1)
					go h.extractCultureLecture(clPageUrl, storeName, &lectureSearchResult.Table[i], c)
				}

				totalLectureCount, err := strconv.Atoi(lectureSearchResult.Table1[0].TotalCount)
				utils.CheckErr(err)

				if len(lectureSearchResult.Table) == 0 {
					log.Fatalf("%s 문화센터 강좌를 수집하는 중에 0건의 강좌 데이터가 수신되어 강좌 수집이 실패하였습니다.(점포:%s, 강좌군:%s, IDX:%d)", h.name, storeName, lectureGroupName, idx)
				}
				if extractionLectureCount > totalLectureCount {
					log.Fatalf("%s 문화센터 강좌를 수집하는 중에 전체 강좌수보다 수집된 강좌수가 더 많아 강좌 수집이 실패하였습니다.(점포:%s, 강좌군:%s, IDX:%d, 전체강좌수:%d, 수집된강좌수:%d)", h.name, storeName, lectureGroupName, idx, totalLectureCount, extractionLectureCount)
				}

				// 전체 강좌를 모두 수집하였는지 확인한다.
				if extractionLectureCount == totalLectureCount {
					break
				}

				idx += 1
			}
		}
	}

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

func (h *homeplus) newLectureSearchPostData(storeCode string, lectureGroupCode string, idx int) *homeplusLectureSearchPostData {
	lspd := homeplusLectureSearchPostData{}

	lspd.Param[0] = strconv.Itoa(idx) // 페이지 번호
	lspd.Param[1] = "20"              // 페이지 크기
	lspd.Param[2] = storeCode         // 점포
	lspd.Param[3] = ""                //
	lspd.Param[4] = ""                //
	lspd.Param[5] = ""                //
	lspd.Param[6] = ""                //
	lspd.Param[7] = lectureGroupCode  //
	lspd.Param[8] = "N"               //
	lspd.Param[9] = "0"               //
	lspd.Param[10] = "20"             // 페이지당 보여줄 강좌 갯수 => 보여줄 강좌의 갯수가 너무 많은 경우 500 에러가 발생함
	lspd.Param[11] = ""               // 강좌상태('':전체, '1':접수중, '0':마감/대기등록)
	lspd.Param[12] = "1"              // 정렬('1':강좌명순, '2':요일/시간순, '3':수강료순, '4':개강임박순, '5':마감임박순)
	lspd.Param[13] = ""               //
	lspd.Param[14] = ""               //

	return &lspd
}

func (h *homeplus) extractCultureLecture(clPageUrl string, storeName string, lsrd *homeplusLectureSearchResultData, c chan<- *lectures.Lecture) {
	// 강좌그룹
	group := fmt.Sprintf("[%s] %s", utils.CleanString(lsrd.LectureTargetView), utils.CleanString(lsrd.ComLectureName))
	if len(group) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터1:%s, 분석데이터2:%s, URL:%s)", h.name, lsrd.LectureTargetView, lsrd.ComLectureName, clPageUrl)
	}

	// 강좌명
	title := fmt.Sprintf("%s %s %s", utils.CleanString(lsrd.LectureName), utils.CleanString(lsrd.SubLectureName1), utils.CleanString(lsrd.SubLectureName2))
	if len(title) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터1:%s, 분석데이터2:%s, 분석데이터3:%s, URL:%s)", h.name, lsrd.LectureName, lsrd.SubLectureName1, lsrd.SubLectureName2, clPageUrl)
	}

	// 개강일
	startDate := utils.CleanString(regexp.MustCompile("^[0-9]{4}-[0-9]{2}-[0-9]{2}$").FindString(lsrd.DateLectureFr))
	if len(startDate) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, lsrd.DateLectureFr, clPageUrl)
	}

	// 시작시간, 종료시간
	startTime := regexp.MustCompile("[0-9]{2}:[0-9]{2} ~").FindString(lsrd.LectureTime)
	endTime := regexp.MustCompile("~ [0-9]{2}:[0-9]{2}").FindString(lsrd.LectureTime)
	if len(startDate) == 0 || len(endTime) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, lsrd.LectureTime, clPageUrl)
	}
	startTime = utils.CleanString(startTime[:len(startTime)-1])
	endTime = utils.CleanString(endTime[1:])

	// 요일
	dayOfTheWeek := utils.CleanString(regexp.MustCompile("^[월화수목금토일] ").FindString(lsrd.LectureWeekName))
	if len(dayOfTheWeek) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, lsrd.LectureWeekName, clPageUrl)
	}

	// 수강료
	price := "0"
	num, err := strconv.Atoi(lsrd.TuitionFeeDC)
	utils.CheckErr(err)

	price = utils.FormatCommas(num)
	if len(price) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터1:%s, 분석데이터2:%s, URL:%s)", h.name, lsrd.TuitionFeeDC, lsrd.TuitionFeeDC, clPageUrl)
	}

	// 강좌횟수
	count := utils.CleanString(lsrd.ClassCount)
	if len(count) == 0 || regexp.MustCompile(`^[0-9]+$`).MatchString(count) == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, lsrd.ClassCount, clPageUrl)
	}

	// 접수상태
	var status = lectures.ReceptionStatusUnknown
	switch lsrd.LectureStatus {
	case "0":
		status = lectures.ReceptionStatusPossible
	case "1":
		status = lectures.ReceptionStatusPossible
	case "2":
		status = lectures.ReceptionStatusStnadBy
	case "3":
		status = lectures.ReceptionStatusVisitConsultation
	case "4":
		status = lectures.ReceptionStatusClosed
	case "8":
		status = lectures.ReceptionStatusVisitFirstComeFirstServed
	case "9":
		status = lectures.ReceptionStatusDayParticipation
	default:
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", h.name, lsrd.LectureStatus, clPageUrl)
	}

	// 상세페이지
	if len(utils.CleanString(lsrd.LectureMasterID)) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지로 이동하기 위해 필요한 [ LectureMasterID ] 값이 비어 있습니다, URL:%s)", h.name, clPageUrl)
	}

	c <- &lectures.Lecture{
		StoreName:      fmt.Sprintf("%s %s", h.name, storeName),
		Group:          group,
		Title:          title,
		Teacher:        lsrd.TeacherName,
		StartDate:      startDate,
		StartTime:      startTime,
		EndTime:        endTime,
		DayOfTheWeek:   dayOfTheWeek + "요일",
		Price:          price + "원",
		Count:          count + "회",
		Status:         status,
		DetailPageUrl:  fmt.Sprintf("%s/Lecture/Detail?LectureMasterID=%s", h.cultureBaseUrl, lsrd.LectureMasterID),
		ScrapeExcluded: false,
	}
}

func (h *homeplus) validCultureLectureStore() bool {
	reqBody := bytes.NewBuffer([]byte("{\"prm\":[\"\",\"\",\"|||0\"]}"))
	res, err := http.Post(fmt.Sprintf("%s/LectureJsonNoAuth/GetStoreMasterAtMobile", h.cultureBaseUrl), "application/json; charset=utf-8", reqBody)
	utils.CheckErr(err)
	utils.CheckStatusCode(res)

	//goland:noinspection GoUnhandledErrorResult
	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	utils.CheckErr(err)

	// resBodyBytes 앞뒤의 '"' 문자를 삭제하고, 전체 문자열에서 '\"' 문자열을 '"'로 치환한다.
	resBodyBytes = []byte(strings.ReplaceAll(string(resBodyBytes[1:len(resBodyBytes)-1]), "\\\"", "\""))

	var storeSearchResult homeplusStoreSearchResult
	err = json.Unmarshal(resBodyBytes, &storeSearchResult)
	utils.CheckErr(err)

	for storeCode, storeName := range h.storeCodeMap {
		stringToFind := fmt.Sprintf("%s|%s|", storeCode, storeName)

		foundStore := false
		for _, elem := range storeSearchResult.Table1 {
			if strings.Contains(elem.StoreInfos, stringToFind) == true {
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
	res, err := http.Get(fmt.Sprintf("%s/Lecture/SearchByCategory", h.cultureBaseUrl))
	utils.CheckErr(err)
	utils.CheckStatusCode(res)

	//goland:noinspection GoUnhandledErrorResult
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	utils.CheckErr(err)

	for lectureGroupCode, lectureGroupName := range h.lectureGroupCodeMap {
		lectureGroupSelection := doc.Find(fmt.Sprintf("div.target_group_select > ul > li > a[commoncode='%s']", lectureGroupCode))
		if lectureGroupSelection.Length() != 1 {
			return false
		}

		val, _ := lectureGroupSelection.Attr("commonename")
		if utils.CleanString(val) != lectureGroupName {
			return false
		}
	}

	return true
}
