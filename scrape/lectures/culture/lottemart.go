package culture

import (
	"bytes"
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
	"sync"
)

type lottemart struct {
	name           string
	cultureBaseUrl string

	searchTermCode string // 검색년도 & 검색시즌 코드

	storeCodeMap        map[string]string            // 점포
	lectureGroupCodeMap map[string]map[string]string // 강좌군
}

func NewLottemart(searchYear string, searchSeasonCode string) *lottemart {
	searchYear = utils.CleanString(searchYear)
	searchSeasonCode = utils.CleanString(searchSeasonCode)

	if searchYear == "" || searchSeasonCode == "" {
		log.Fatalf("검색년도 및 검색시즌코드는 빈 문자열을 허용하지 않습니다(검색년도:%s, 검색시즌코드:%s)", searchYear, searchSeasonCode)
	}

	return &lottemart{
		name: "롯데마트",

		cultureBaseUrl: "https://culture.lottemart.com",

		searchTermCode: fmt.Sprintf("%s0%s", searchYear, searchSeasonCode),

		storeCodeMap: map[string]string{
			"705": "여수점",
		},

		lectureGroupCodeMap: map[string]map[string]string{
			"baby-tit": { // 영아강좌(0~5세)
				"21": "음악감성",
				"81": "",
				"22": "미술표현",
				"82": "",
				"23": "언어인지",
				"83": "",
				"24": "통합놀이",
				"84": "",
				"25": "신체발달",
				"85": "",
				"26": "조기영재",
				"86": "",
				"27": "창의적체험활동",
				"87": "",
			},
			"toddler-tit": { // 유아 강좌(5~7세)
				"31": "음악 감성",
				"32": "미술표현",
				"33": "창의인지",
				"34": "언어인지",
				"35": "신체발달",
				"36": "키즈쿠킹",
				"37": "창의적체험활동",
			},
			"child-tit": { // 어린이청소년
				"41": "음악감성",
				"42": "미술표현",
				"43": "창의인지",
				"44": "진로/직업체험",
				"45": "언어인지",
				"46": "신체발달",
				"47": "키즈쿠킹",
				"48": "창의적체험활동",
			},
		},
	}
}

func (l *lottemart) ScrapeCultureLectures(mainC chan<- []lectures.Lecture) {
	log.Printf("%s 문화센터 강좌 수집을 시작합니다.(검색조건:%s)", l.name, l.searchTermCode)

	// 강좌군이 유효한지 확인한다.
	if l.validCultureLectureGroup() == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, 강좌군코드 불일치)", l.name)
	}

	var wait sync.WaitGroup

	c := make(chan *lectures.Lecture, 100)

	count := 0
	for storeCode, storeName := range l.storeCodeMap {
		// 점포가 유효한지 확인한다.
		if l.validCultureLectureStore(storeCode, storeName) == false {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, 점포코드 불일치:%s)", l.name, storeCode)
		}

		// 불러올 전체 페이지 갯수를 구한다.
		_, doc := l.cultureLecturePageDocument(1, storeCode)
		pi, exists := doc.Find("tr:last-child").Attr("pageinfo")
		if exists == false {
			log.Fatalf("%s 문화센터 강좌를 수집하는 중에 전체 페이지 갯수 추출이 실패하였습니다.", l.name)
		}

		// ---------------------------------
		// pageinfo 값 형식 : 1|5|85|61|0|24
		// ---------------------------------
		// 1  : 현재 페이지 번호
		// 5  : 전체 페이지 번호
		// 85 : 전체 강좌 갯수
		// 61 : 접수가능 갯수
		// 0  : 온라인마감 갯수
		// 24 : 접수마감 갯수
		piSplit := strings.Split(pi, "|")
		if len(piSplit) != 6 {
			log.Fatalf("%s 문화센터 강좌를 수집하는 중에 전체 페이지 갯수 추출이 실패하였습니다.(pageinfo:%s)", l.name, pi)
		}

		totalPageCount, err := strconv.Atoi(piSplit[1])
		utils.CheckErr(err)

		// 강좌 데이터를 수집한다.
		for pageNo := 1; pageNo <= totalPageCount; pageNo++ {
			wait.Add(1)
			go func(storeCode string, storeName string, pageNo int) {
				defer wait.Done()

				clPageUrl, doc := l.cultureLecturePageDocument(pageNo, storeCode)

				clSelection := doc.Find("tr")
				clSelection.Each(func(i int, s *goquery.Selection) {
					count += 1
					go l.extractCultureLecture(clPageUrl, storeCode, storeName, s, c)
				})
			}(storeCode, storeName, pageNo)
		}
	}

	wait.Wait()

	var lectureList []lectures.Lecture
	for i := 0; i < count; i++ {
		lecture := <-c
		if len(lecture.Title) > 0 {
			lectureList = append(lectureList, *lecture)
		}
	}

	log.Printf("%s 문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", l.name, len(lectureList))

	mainC <- lectureList
}

func (l *lottemart) extractCultureLecture(clPageUrl string, storeCode string, storeName string, s *goquery.Selection, c chan<- *lectures.Lecture) {
	// 강좌의 컬럼 개수를 확인한다.
	ls := s.Find("td")
	if ls.Length() != 5 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 컬럼 개수 불일치:%d, URL:%s)", l.name, ls.Length(), clPageUrl)
	}

	// 강사명, 형식 : 김준희
	lectureCol2 := utils.CleanString(ls.Eq(1).Text())
	// 개강일/요일/시간, 형식 : 2020.12.05(토) 15:20~16:00
	lectureCol3 := utils.CleanString(ls.Eq(2).Text())
	// 수강료, 형식 : 12회 80,000원 60,000원
	lectureCol4 := utils.CleanString(ls.Eq(3).Text())
	// 접수상태/수강신청, 형식 : 바로신청
	lectureCol5 := utils.CleanString(ls.Eq(4).Find("div > div > a.btn-status:last-child").Text())

	// 강좌명
	lts := ls.Eq(0).Find("div.info-txt > a")
	if lts.Length() == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌명 <a> 태그를 찾을 수 없습니다, URL:%s)", l.name, clPageUrl)
	}
	title := utils.CleanString(lts.Text())

	// 개강일
	startDate := regexp.MustCompile("^[0-9]{4}\\.[0-9]{2}\\.[0-9]{2}").FindString(lectureCol3)
	if len(startDate) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol3, clPageUrl)
	}
	startDate = strings.ReplaceAll(startDate, ".", "-")

	// 시작시간, 종료시간
	startTime := strings.TrimSpace(regexp.MustCompile(" [0-9]{2}:[0-9]{2}").FindString(lectureCol3))
	endTime := strings.TrimSpace(regexp.MustCompile("[0-9]{2}:[0-9]{2}$").FindString(lectureCol3))
	if len(startDate) == 0 || len(endTime) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol3, clPageUrl)
	}

	// 요일
	dayOfTheWeek := regexp.MustCompile("\\([월화수목금토일]").FindString(lectureCol3)
	if len(dayOfTheWeek) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol3, clPageUrl)
	}
	dayOfTheWeek = string([]rune(dayOfTheWeek)[1:])

	// 수강료
	price := regexp.MustCompile("[0-9,]{1,8}원$").FindString(lectureCol4)
	if strings.Contains(price, "원") == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol4, clPageUrl)
	}

	// 강좌횟수
	count := regexp.MustCompile("[0-9]{1,3}회").FindString(lectureCol4)
	if len(count) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol4, clPageUrl)
	}

	// 접수상태
	var status = lectures.ReceptionStatusUnknown
	switch lectureCol5 {
	case "바로신청":
		status = lectures.ReceptionStatusPossible
	case "접수마감":
		status = lectures.ReceptionStatusClosed
	case "대기자 신청":
		status = lectures.ReceptionStatusStnadBy
	case "현장문의":
		status = lectures.ReceptionStatusVisitInquiry
	default:
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", l.name, lectureCol5, clPageUrl)
	}

	// 상세페이지
	classCode, exists := lts.Attr("onclick")
	if exists == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지 주소를 찾을 수 없습니다, URL:%s)", l.name, clPageUrl)
	}
	pos1 := strings.Index(classCode, "'")
	pos2 := strings.LastIndex(classCode, "'")
	if pos1 == -1 || pos2 == -1 || pos1 == pos2 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지 주소를 찾을 수 없습니다, URL:%s)", l.name, clPageUrl)
	}
	classCode = classCode[pos1+1 : pos2]

	c <- &lectures.Lecture{
		StoreName:      fmt.Sprintf("%s %s", l.name, storeName),
		Group:          "",
		Title:          title,
		Teacher:        lectureCol2,
		StartDate:      startDate,
		StartTime:      startTime,
		EndTime:        endTime,
		DayOfTheWeek:   dayOfTheWeek + "요일",
		Price:          price,
		Count:          count,
		Status:         status,
		DetailPageUrl:  fmt.Sprintf("%s/cu/gus/course/courseinfo/courseview.do?cls_cd=%s&is_category_open=N&search_term_cd=%s&search_str_cd=%s", l.cultureBaseUrl, classCode, l.searchTermCode, storeCode),
		ScrapeExcluded: false,
	}
}

func (l *lottemart) cultureLecturePageDocument(pageNo int, storeCode string) (string, *goquery.Document) {
	clPageUrl := fmt.Sprintf("%s/cu/gus/course/courseinfo/searchList.do", l.cultureBaseUrl)

	paramArrCatCd := ""
	paramSearchCatCd := ""
	for _, v := range l.lectureGroupCodeMap {
		for lectureGroupCode, _ := range v {
			if paramSearchCatCd != "" {
				paramSearchCatCd += ","
			}
			paramSearchCatCd += lectureGroupCode

			if paramArrCatCd != "" {
				paramArrCatCd += "&"
			}
			paramArrCatCd += fmt.Sprintf("arr_cat_cd=%s", lectureGroupCode)
		}
	}
	reqBody := bytes.NewBufferString(fmt.Sprintf("currPageNo=%d&search_list_type=&search_str_cd=%s&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&search_term_cd=%s&search_day_fg=&search_cls_nm=&search_cat_cd=%s&search_opt_cd=&search_tit_cd=&%s", pageNo, storeCode, l.searchTermCode, paramSearchCatCd, paramArrCatCd))

	res, err := http.Post(clPageUrl, "application/x-www-form-urlencoded; charset=UTF-8", reqBody)
	utils.CheckErr(err)
	utils.CheckStatusCode(res)

	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	utils.CheckErr(err)

	// 실제 불러온 데이터는 '<table>' 태그가 포함되어 있지 않고 '<tr>', '<td>'만 있는 형태!!
	// 이 형태에서 goquery.NewDocumentFromReader() 함수를 호출하면 '<tr>', '<td>' 태그가 모두 사라지므로 '<table>' 태그를 강제로 붙여준다.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table>" + string(resBodyBytes) + "</table>"))
	utils.CheckErr(err)

	return clPageUrl, doc
}

func (l *lottemart) validCultureLectureStore(storeCode, storeName string) bool {
	res, err := http.Get(fmt.Sprintf("%s/cu/branch/main.do?search_str_cd=%s", l.cultureBaseUrl, storeCode))
	utils.CheckErr(err)
	utils.CheckStatusCode(res)

	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	utils.CheckErr(err)

	vSelection := doc.Find("#contents div.branch_main-wrap div.branch_info-area > div.branch_spot-area > h3")
	if vSelection.Length() != 1 || utils.CleanString(vSelection.Text()) != storeName {
		return false
	}

	return true
}

func (l *lottemart) validCultureLectureGroup() bool {
	res, err := http.Get(fmt.Sprintf("%s/cu/gus/course/courseinfo/courselist.do", l.cultureBaseUrl))
	utils.CheckErr(err)
	utils.CheckStatusCode(res)

	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	utils.CheckErr(err)

	for lectureGroupsID, v := range l.lectureGroupCodeMap {
		lectureGroupsIDSelection := doc.Find(fmt.Sprintf("#%s", lectureGroupsID))
		if lectureGroupsIDSelection.Length() != 1 {
			return false
		}

		for lectureGroupCode, lectureGroupName := range v {
			if lectureGroupName == "" {
				continue
			}

			lectureGroupSelection := lectureGroupsIDSelection.Parent().Parent().Parent().Find(fmt.Sprintf("dd > ul > li > div > input[value='%s']", lectureGroupCode))
			if lectureGroupSelection.Length() != 1 || utils.CleanString(lectureGroupSelection.Parent().Text()) != lectureGroupName {
				return false
			}
		}
	}

	return true
}
