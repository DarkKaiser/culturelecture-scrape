package culture

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/darkkaiser/culturelecture-scrape/scrape/lectures"
	"github.com/darkkaiser/culturelecture-scrape/utils"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
)

type emart struct {
	name           string
	cultureBaseUrl string

	searchYearCode string // 검색년도
	searchSmstCode string // 검색시즌 코드(S1 ~ S4)

	storeCodeMap        map[string]string // 점포
	lectureGroupCodeMap map[string]string // 강좌군
}

func NewEmart(searchYear string, searchSeasonCode string) *emart {
	searchYear = utils.CleanString(searchYear)
	searchSeasonCode = utils.CleanString(searchSeasonCode)

	if searchYear == "" || searchSeasonCode == "" {
		log.Fatalf("검색년도 및 검색시즌코드는 빈 문자열을 허용하지 않습니다(검색년도:%s, 검색시즌코드:%s)", searchYear, searchSeasonCode)
	}

	return &emart{
		name: "이마트",

		cultureBaseUrl: "http://culture.emart.com",

		searchYearCode: searchYear,

		searchSmstCode: "S" + searchSeasonCode,

		storeCodeMap: map[string]string{
			"560": "여수점",
			"900": "순천점",
		},

		lectureGroupCodeMap: map[string]string{
			"10": "엄마랑 아기랑(0~4세)인지/표현",
			"11": "엄마랑 아기랑(0~4세)예능/신체",
			"12": "엄마랑 아기랑(0~4세)주말프로그램",
			"13": "유아(5~7세)인지/표현",
			"14": "유아(5~7세)예능/신체",
			"15": "유아(5~7세)주말프로그램",
			"16": "어린이 인지/표현",
			"17": "어린이 예능/신체",
			"18": "어린이 주말프로그램",
			"20": "체험/이벤트",
			"21": "외부제휴프로그램",
			"50": "8주 단기 강좌",
		},
	}
}

func (e *emart) ScrapeCultureLectures(mainC chan<- []lectures.Lecture) {
	log.Printf("%s 문화센터 강좌 수집을 시작합니다.(검색조건:%s년도 %s)", e.name, e.searchYearCode, e.searchSmstCode)

	var wait sync.WaitGroup

	c := make(chan *lectures.Lecture, 100)

	var count int64 = 0
	for storeCode, storeName := range e.storeCodeMap {
		for lectureGroupCode, lectureGroupName := range e.lectureGroupCodeMap {
			wait.Add(1)
			go func(storeCode string, storeName string, lectureGroupCode string, lectureGroupName string) {
				defer wait.Done()

				clPageUrl := fmt.Sprintf("%s/lecture/lecture/list?year_code=%s&smst_code=%s&order_by=0&flag=&default_display_cnt=999&page_index=1&store_code=%s&group_code=%s&lect_name=", e.cultureBaseUrl, e.searchYearCode, e.searchSmstCode, storeCode, lectureGroupCode)

				res, err := http.Get(clPageUrl)
				utils.CheckErr(err)
				utils.CheckStatusCode(res)

				defer res.Body.Close()

				doc, err := goquery.NewDocumentFromReader(res.Body)
				utils.CheckErr(err)

				// 점포가 유효한지 확인한다.
				vSelection := doc.Find(fmt.Sprintf("#d-storelist a[data-code='%s']", storeCode))
				if vSelection.Length() != 1 || utils.CleanString(vSelection.Text()) != storeName {
					log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, 점포코드 불일치:%s, URL:%s)", e.name, storeCode, clPageUrl)
				}
				// 강좌군이 유효한지 확인한다.
				vSelection = doc.Find(fmt.Sprintf("#d-lectlist > ul.lecture_list > li input[name='group_code'][value='%s']", lectureGroupCode))
				if vSelection.Length() != 1 || utils.CleanString(vSelection.Parent().Text()) != lectureGroupName {
					log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, 강좌군코드 불일치:%s, URL:%s)", e.name, lectureGroupCode, clPageUrl)
				}

				clSelection := doc.Find("div.board_list > table > tbody > tr")
				if clSelection.Length() <= 0 {
					log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, URL:%s)", e.name, clPageUrl)
				}

				clSelection.Each(func(i int, s *goquery.Selection) {
					atomic.AddInt64(&count, 1)
					go e.extractCultureLecture(clPageUrl, storeName, s, c)
				})
			}(storeCode, storeName, lectureGroupCode, lectureGroupName)
		}
	}

	wait.Wait()

	var lectureList []lectures.Lecture
	for i := int64(0); i < count; i++ {
		lecture := <-c
		if len(lecture.Title) > 0 {
			lectureList = append(lectureList, *lecture)
		}
	}

	log.Printf("%s 문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", e.name, len(lectureList))

	mainC <- lectureList
}

func (e *emart) extractCultureLecture(clPageUrl string, storeName string, s *goquery.Selection, c chan<- *lectures.Lecture) {
	if utils.CleanString(s.Text()) == "검색된 강좌가 없습니다." {
		c <- &lectures.Lecture{}
	} else {
		// 강좌의 컬럼 개수를 확인한다.
		ls := s.Find("td")
		if ls.Length() != 5 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 컬럼 개수 불일치:%d, URL:%s)", e.name, ls.Length(), clPageUrl)
		}

		// 강좌명, 형식 : [가든5점] (월)13:10 댄스스포츠 (자이브,룸바,왈츠,탱고) (성인/자녀동반불가)
		lectureCol1 := utils.CleanString(ls.Eq(0).Text())
		// 강좌시작일(횟수), 형식 : 2020-12-07 (12회)
		lectureCol2 := utils.CleanString(ls.Eq(1).Text())
		// 강좌시간/요일, 형식 : 13:10 ~ 14:20 / 월
		lectureCol3 := utils.CleanString(ls.Eq(2).Text())
		// 수강료, 형식 : 70,000원
		lectureCol4 := utils.CleanString(ls.Eq(3).Text())
		// 접수상태, 형식 : 접수가능
		lectureCol5 := utils.CleanString(ls.Eq(4).Text())

		// 개강일
		startDate := regexp.MustCompile("[0-9]{4}-[0-9]{2}-[0-9]{2}").FindString(lectureCol2)
		if len(startDate) == 0 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", e.name, lectureCol2, clPageUrl)
		}

		// 시작시간, 종료시간
		startTime := regexp.MustCompile("^[0-9]{2}:[0-9]{2}").FindString(lectureCol3)
		endTime := strings.TrimSpace(regexp.MustCompile(" [0-9]{2}:[0-9]{2} ").FindString(lectureCol3))
		if len(startDate) == 0 || len(endTime) == 0 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", e.name, lectureCol3, clPageUrl)
		}

		// 요일
		dayOfTheWeek := regexp.MustCompile("[월화수목금토일]$").FindString(lectureCol3)
		if len(dayOfTheWeek) == 0 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", e.name, lectureCol3, clPageUrl)
		}

		// 수강료
		if strings.Contains(lectureCol4, "원") == false {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", e.name, lectureCol4, clPageUrl)
		}

		// 강좌횟수
		count := regexp.MustCompile("[0-9]{1,3}회").FindString(lectureCol2)
		if len(count) == 0 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", e.name, lectureCol2, clPageUrl)
		}

		// 접수상태
		var status = lectures.ReceptionStatusUnknown
		switch lectureCol5 {
		case "접수 예정":
			status = lectures.ReceptionStatusPlanned
		case "접수가능":
			status = lectures.ReceptionStatusPossible
		case "접수 마감":
			status = lectures.ReceptionStatusClosed
		case "대기신청":
			status = lectures.ReceptionStatusStnadBy
		default:
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", e.name, lectureCol5, clPageUrl)
		}

		// 상세페이지
		detailPageUrl, exists := ls.Eq(0).Find("a").Attr("href")
		if exists == false {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지 주소를 찾을 수 없습니다, URL:%s)", e.name, clPageUrl)
		}

		c <- &lectures.Lecture{
			StoreName:      fmt.Sprintf("%s %s", e.name, storeName),
			Group:          "",
			Title:          lectureCol1,
			Teacher:        "",
			StartDate:      startDate,
			StartTime:      startTime,
			EndTime:        endTime,
			DayOfTheWeek:   dayOfTheWeek + "요일",
			Price:          lectureCol4,
			Count:          count,
			Status:         status,
			DetailPageUrl:  e.cultureBaseUrl + utils.CleanString(detailPageUrl),
			ScrapeExcluded: false,
		}
	}
}
