package mart

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"helpers"
	"log"
	"net/http"
	"regexp"
	"scrape/culturelecture"
	"strings"
	"sync"
)

type emart struct {
	name           string
	cultureBaseUrl string

	searchYearCode string // 검색년도
	searchSmstCode string // 검색시즌 코드(S1 ~ S4)

	storeCodeMap map[string]string // 점포
	groupCodeMap map[string]string // 강좌군
}

func NewEmart(searchYear string, searchSeasonCode string) *emart {
	searchYear = helpers.CleanString(searchYear)
	searchSeasonCode = helpers.CleanString(searchSeasonCode)

	if len(searchYear) == 0 || len(searchSeasonCode) == 0 {
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

		groupCodeMap: map[string]string{
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

func (e *emart) ScrapeCultureLectures(mainC chan<- []culturelecture.Lecture) {
	log.Printf("%s 문화센터 강좌 수집을 시작합니다.(검색조건:%s년도 %s)", e.name, e.searchYearCode, e.searchSmstCode)

	var wait sync.WaitGroup

	c := make(chan *culturelecture.Lecture, 100)

	count := 0
	for storeCode, storeName := range e.storeCodeMap {
		for groupCode := range e.groupCodeMap {
			wait.Add(1)
			go func(storeCode string, storeName string, groupCode string) {
				defer wait.Done()

				clPageUrl := fmt.Sprintf("%s/lecture/lecture/list?year_code=%s&smst_code=%s&order_by=0&flag=&default_display_cnt=999&page_index=1&store_code=%s&group_code=%s&lect_name=", e.cultureBaseUrl, e.searchYearCode, e.searchSmstCode, storeCode, groupCode)

				res, err := http.Get(clPageUrl)
				helpers.CheckErr(err)
				helpers.CheckStatusCode(res)

				defer res.Body.Close()

				doc, err := goquery.NewDocumentFromReader(res.Body)
				helpers.CheckErr(err)

				clSelection := doc.Find("div.board_list > table > tbody > tr")
				clSelection.Each(func(i int, s *goquery.Selection) {
					count += 1
					go e.extractCultureLecture(clPageUrl, storeName, s, c)
				})
			}(storeCode, storeName, groupCode)
		}
	}

	wait.Wait()

	var lectures []culturelecture.Lecture
	for i := 0; i < count; i++ {
		lecture := <-c
		if len(lecture.Title) > 0 {
			lectures = append(lectures, *lecture)
		}
	}

	log.Printf("%s 문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", e.name, len(lectures))

	mainC <- lectures
}

func (e *emart) extractCultureLecture(clPageUrl string, storeName string, s *goquery.Selection, c chan<- *culturelecture.Lecture) {
	if helpers.CleanString(s.Text()) == "검색된 강좌가 없습니다." {
		c <- &culturelecture.Lecture{}
	} else {
		// 강좌의 컬럼 개수를 확인한다.
		ls := s.Find("td")
		if ls.Length() != 5 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 컬럼 개수 불일치:%d, URL:%s)", e.name, ls.Length(), clPageUrl)
		}

		lectureCol1 := helpers.CleanString(ls.Eq(0 /* 강좌명 */).Text())
		lectureCol2 := helpers.CleanString(ls.Eq(1 /* 강좌시작일(횟수) */).Text())
		lectureCol3 := helpers.CleanString(ls.Eq(2 /* 강좌시간/요일 */).Text())
		lectureCol4 := helpers.CleanString(ls.Eq(3 /* 수강료 */).Text())
		lectureCol5 := helpers.CleanString(ls.Eq(4 /* 접수상태 */).Text())

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
		var status culturelecture.ReceptionStatus = culturelecture.ReceptionStatusUnknown
		switch lectureCol5 {
		case "접수가능":
			status = culturelecture.ReceptionStatusPossible
		case "접수 마감":
			status = culturelecture.ReceptionStatusClosed
		case "대기신청":
			status = culturelecture.ReceptionStatusStnadBy
		default:
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", e.name, lectureCol5, clPageUrl)
		}

		// 상세페이지
		detailPageUrl, exists := ls.Eq(0).Find("a").Attr("href")
		if exists == false {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지 주소를 찾을 수 없습니다, URL:%s)", e.name, clPageUrl)
		}

		c <- &culturelecture.Lecture{
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
			DetailPageUrl:  e.cultureBaseUrl + helpers.CleanString(detailPageUrl),
			ScrapeExcluded: false,
		}
	}
}
