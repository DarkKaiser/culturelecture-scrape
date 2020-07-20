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

	searchTermCode string            // 검색년도 & 검색시즌 코드
	storeCodeMap   map[string]string // 점포
}

func NewLottemart(searchYear string, searchSeasonCode string) *lottemart {
	searchYear = utils.CleanString(searchYear)
	searchSeasonCode = utils.CleanString(searchSeasonCode)

	if len(searchYear) == 0 || len(searchSeasonCode) == 0 {
		log.Fatalf("검색년도 및 검색시즌코드는 빈 문자열을 허용하지 않습니다(검색년도:%s, 검색시즌코드:%s)", searchYear, searchSeasonCode)
	}

	return &lottemart{
		name: "롯데마트",

		cultureBaseUrl: "http://culture.lottemart.com",

		searchTermCode: fmt.Sprintf("%s0%s", searchYear, searchSeasonCode),

		storeCodeMap: map[string]string{
			"705": "여수점",
		},
	}
}

func (l *lottemart) ScrapeCultureLectures(mainC chan<- []lectures.Lecture) {
	log.Printf("%s 문화센터 강좌 수집을 시작합니다.(검색조건:%s)", l.name, l.searchTermCode)

	var wait sync.WaitGroup

	c := make(chan *lectures.Lecture, 100)

	count := 0
	for storeCode, storeName := range l.storeCodeMap {
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

	lectureCol2 := utils.CleanString(ls.Eq(1 /* 강사명 */).Text())
	lectureCol3 := utils.CleanString(ls.Eq(2 /* 요일/시간/개강일 */).Text())
	lectureCol4 := utils.CleanString(ls.Eq(3 /* 수강료 */).Text())
	lectureCol5 := utils.CleanString(ls.Eq(4 /* 접수상태/수강신청 */).Find("div > div > a.btn-status:last-child").Text())

	// 강좌명
	lts := ls.Eq(0 /* 강좌명 */).Find("div.info-txt > a")
	if lts.Length() == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌명 <a> 태그를 찾을 수 없습니다, URL:%s)", l.name, clPageUrl)
	}
	title := utils.CleanString(lts.Text())

	// 개강일
	startDate := regexp.MustCompile("[0-9]{4}\\.[0-9]{2}\\.[0-9]{2}$").FindString(lectureCol3)
	if len(startDate) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol3, clPageUrl)
	}
	startDate = strings.ReplaceAll(startDate, ".", "-")

	// 시작시간, 종료시간
	startTime := strings.TrimSpace(regexp.MustCompile(" [0-9]{2}:[0-9]{2}").FindString(lectureCol3))
	endTime := strings.TrimSpace(regexp.MustCompile("[0-9]{2}:[0-9]{2} ").FindString(lectureCol3))
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
	clPageUrl := l.cultureBaseUrl + "/cu/gus/course/courseinfo/searchList.do"

	reqBody := bytes.NewBufferString(fmt.Sprintf("currPageNo=%d&search_list_type=&search_str_cd=%s&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&search_term_cd=%s&search_day_fg=&search_cls_nm=&search_cat_cd=21,81,22,82,23,83,24,84,25,85,26,86,27,87,31,32,33,34,35,36,37,41,42,43,44,45,46,47,48&search_opt_cd=&search_tit_cd=&arr_cat_cd=21&arr_cat_cd=81&arr_cat_cd=22&arr_cat_cd=82&arr_cat_cd=23&arr_cat_cd=83&arr_cat_cd=24&arr_cat_cd=84&arr_cat_cd=25&arr_cat_cd=85&arr_cat_cd=26&arr_cat_cd=86&arr_cat_cd=27&arr_cat_cd=87&arr_cat_cd=31&arr_cat_cd=32&arr_cat_cd=33&arr_cat_cd=34&arr_cat_cd=35&arr_cat_cd=36&arr_cat_cd=37&arr_cat_cd=41&arr_cat_cd=42&arr_cat_cd=43&arr_cat_cd=44&arr_cat_cd=45&arr_cat_cd=46&arr_cat_cd=47&arr_cat_cd=48", pageNo, storeCode, l.searchTermCode))
	res, err := http.Post(clPageUrl, "application/x-www-form-urlencoded; charset=UTF-8", reqBody)
	utils.CheckErr(err)
	utils.CheckStatusCode(res)

	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	utils.CheckErr(err)

	// 실제로 불러온 데이터는 '<table>' 태그가 포함되어 있지 않고 '<tr>', '<td>'만 있는 형태
	// 이 형태에서 goquery.NewDocumentFromReader() 함수를 호출하면 '<tr>', '<td>' 태그가 모두 사라지므로 '<table>' 태그를 강제로 붙여준다.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table>" + string(resBodyBytes) + "</table>"))
	utils.CheckErr(err)

	return clPageUrl, doc
}
