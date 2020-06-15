package main

import (
	"github.com/PuerkitoBio/goquery"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const (
	emartCultureBaseURL = "http://culture.emart.com"

	// 검색년도
	emartSearchYearCode = "2020"

	// 검색시즌(S1 ~ S4)
	emartSearchSmstCode = "S2"
)

/*
 * 점포
 */
var emartStoreCodeMap = map[string]string{
	"560": "이마트 여수점",
	"900": "이마트 순천점",
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
	log.Println("이마트 문화센터 강좌 수집을 시작합니다.(검색조건:" + emartSearchYearCode + "년도 " + emartSearchSmstCode + ")")

	var wait sync.WaitGroup

	c := make(chan cultureLecture, 10)

	count := 0
	for storeCode, storeName := range emartStoreCodeMap {
		for groupCode, _ := range emartGroupCodeMap {
			wait.Add(1)
			go func(storeCode string, storeName string, groupCode string) {
				defer wait.Done()

				clPageURL := emartCultureBaseURL + "/lecture/lecture/list?year_code=" + emartSearchYearCode + "&smst_code=" + emartSearchSmstCode + "&order_by=0&flag=&default_display_cnt=999&page_index=1&store_code=" + storeCode + "&group_code=" + groupCode + "&lect_name="

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
			cultureLectures = append(cultureLectures, cultureLecture)
		}
	}

	log.Println("이마트 문화센터 강좌 수집이 완료되었습니다. 총 " + strconv.Itoa(len(cultureLectures)) + "개의 강좌가 수집되었습니다.")

	mainC <- cultureLectures
}

func extractEmartCultureLecture(clPageURL string, storeName string, s *goquery.Selection, c chan<- cultureLecture) {
	if cleanString(s.Text()) == "검색된 강좌가 없습니다." {
		c <- cultureLecture{}
	} else {
		// @@@@@
		// 강좌 목록에서 열의 갯수가 5개가 아니라면 파싱 에러
		if s.Find("td").Length() != 5 {
			log.Fatalln("강좌 파싱 에러", clPageURL)
		}

		columns := s.Find("td")

		val, _ := columns.Find("a").Attr("href")
		val = emartCultureBaseURL + cleanString(val)

		dateTime := cleanString(columns.Eq(2).Text())
		split := strings.Split(dateTime, "/")

		sd := cleanString(columns.Eq(1).Text())
		pos1 := strings.Index(sd, "(")
		pos2 := strings.Index(sd, ")")

		c <- cultureLecture{
			storeName:     storeName,
			title:         cleanString(columns.Find("a").Text()),
			teacher:       "",
			startDate:     cleanString(string(sd[0:pos1])),
			time:          cleanString(split[0]),
			dayOfTheWeek:  cleanString(split[1]) + "요일",
			price:         cleanString(columns.Eq(3).Text()),
			count:         cleanString(string(sd[pos1+1 : pos2])),
			status:        cleanString(columns.Eq(4).Text()),
			detailPageUrl: val,
		}
	}
}
