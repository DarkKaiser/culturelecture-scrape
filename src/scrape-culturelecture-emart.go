package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

const (
	emart = "이마트"

	emartCultureBaseURL = "http://culture.emart.com"

	// 검색년도
	emartSearchYearCode = SearchYear

	// 검색시즌 코드(S1 ~ S4)
	emartSearchSmstCode = "S" + SearchSeasonCode
)

/*
 * 점포
 */
var emartStoreCodeMap = map[string]string{
	"560": "여수점",
	"900": "순천점",
}

/*
 * 강좌군
 */
var emartGroupCodeMap = map[string]string{
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
}

func scrapeEmartCultureLecture(mainC chan<- []cultureLecture) {
	log.Printf("%s 문화센터 강좌 수집을 시작합니다.(검색조건:%s년도 %s)", emart, emartSearchYearCode, emartSearchSmstCode)

	var wait sync.WaitGroup

	c := make(chan *cultureLecture, 10)

	count := 0
	for storeCode, storeName := range emartStoreCodeMap {
		for groupCode := range emartGroupCodeMap {
			wait.Add(1)
			go func(storeCode string, storeName string, groupCode string) {
				defer wait.Done()

				clPageURL := fmt.Sprintf("%s/lecture/lecture/list?year_code=%s&smst_code=%s&order_by=0&flag=&default_display_cnt=999&page_index=1&store_code=%s&group_code=%s&lect_name=", emartCultureBaseURL, emartSearchYearCode, emartSearchSmstCode, storeCode, groupCode)

				res, err := http.Get(clPageURL)
				checkErr(err)
				checkStatusCode(res)

				defer res.Body.Close()

				doc, err := goquery.NewDocumentFromReader(res.Body)
				checkErr(err)

				clSelection := doc.Find("div.board_list > table > tbody > tr")
				clSelection.Each(func(i int, s *goquery.Selection) {
					count += 1
					go extractEmartCultureLecture(clPageURL, storeName, s, c)
				})
			}(storeCode, storeName, groupCode)
		}
	}

	wait.Wait()

	var cultureLectures []cultureLecture
	for i := 0; i < count; i++ {
		cultureLecture := <-c
		if len(cultureLecture.title) > 0 {
			cultureLectures = append(cultureLectures, *cultureLecture)
		}
	}

	log.Printf("%s 문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", emart, len(cultureLectures))

	mainC <- cultureLectures
}

func extractEmartCultureLecture(clPageURL string, storeName string, s *goquery.Selection, c chan<- *cultureLecture) {
	if cleanString(s.Text()) == "검색된 강좌가 없습니다." {
		c <- &cultureLecture{}
	} else {
		// 강좌의 컬럼 개수를 확인한다.
		ls := s.Find("td")
		if ls.Length() != 5 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 컬럼 개수 불일치:%d, URL:%s)", emart, ls.Length(), clPageURL)
		}

		lectureCol1 := cleanString(ls.Eq(0 /* 강좌명 */).Text())
		lectureCol2 := cleanString(ls.Eq(1 /* 강좌시작일(횟수) */).Text())
		lectureCol3 := cleanString(ls.Eq(2 /* 강좌시간/요일 */).Text())
		lectureCol4 := cleanString(ls.Eq(3 /* 수강료 */).Text())
		lectureCol5 := cleanString(ls.Eq(4 /* 접수상태 */).Text())

		// 개강일
		startDate := regexp.MustCompile("[0-9]{4}-[0-9]{2}-[0-9]{2}").FindString(lectureCol2)
		if len(startDate) == 0 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", emart, lectureCol2, clPageURL)
		}

		// 시작시간, 종료시간
		startTime := regexp.MustCompile("^[0-9]{2}:[0-9]{2}").FindString(lectureCol3)
		endTime := strings.TrimSpace(regexp.MustCompile(" [0-9]{2}:[0-9]{2} ").FindString(lectureCol3))
		if len(startDate) == 0 || len(endTime) == 0 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", emart, lectureCol3, clPageURL)
		}

		// 요일
		dayOfTheWeek := regexp.MustCompile("[월화수목금토일]{1}$").FindString(lectureCol3)
		if len(dayOfTheWeek) == 0 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", emart, lectureCol3, clPageURL)
		}

		// 수강료
		if strings.Contains(lectureCol4, "원") == false {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", emart, lectureCol4, clPageURL)
		}

		// 강좌횟수
		count := regexp.MustCompile("[0-9]{1,3}회").FindString(lectureCol2)
		if len(count) == 0 {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", emart, lectureCol2, clPageURL)
		}

		// 접수상태
		var status ReceptionStatus = ReceptionStatusUnknown
		switch lectureCol5 {
		case "접수가능":
			status = ReceptionStatusPossible
		case "접수 마감":
			status = ReceptionStatusClosed
		case "대기신청":
			status = ReceptionStatusStnadBy
		default:
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", emart, lectureCol5, clPageURL)
		}

		// 상세페이지
		detailPageUrl, exists := ls.Eq(0).Find("a").Attr("href")
		if exists == false {
			log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지 주소를 찾을 수 없습니다, URL:%s)", emart, clPageURL)
		}

		c <- &cultureLecture{
			storeName:      fmt.Sprintf("%s %s", emart, storeName),
			group:          "",
			title:          lectureCol1,
			teacher:        "",
			startDate:      startDate,
			startTime:      startTime,
			endTime:        endTime,
			dayOfTheWeek:   dayOfTheWeek + "요일",
			price:          lectureCol4,
			count:          count,
			status:         status,
			detailPageUrl:  emartCultureBaseURL + cleanString(detailPageUrl),
			scrapeExcluded: false,
		}
	}
}
