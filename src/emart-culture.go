package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"log"
	"net/http"
)

const (
	emartCultureBaseURL string = "http://culture.emart.com"

	// 검색 년도
	emartSearchYearCode string = "2020"

	// 검색 시즌(S1 ~ S4)
	emartSearchSmstCode string = "S2"
)

// 점포
var emartStoreCodeMap = map[string]string{
	"560": "이마트 여수점",
	"900": "이마트 순천점",
}

// 강좌군
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
	c := make(chan cultureLecture)

	count := 0
	for storeCode, storeName := range emartStoreCodeMap {
		for groupCode, _ := range emartGroupCodeMap {
			// @@@@@ 병렬처리
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
		}
	}

	var cultureLectures []cultureLecture
	for i := 0; i < count; i++ {
		cultureLecture := <-c
		if len(cultureLecture.title) > 0 {
			cultureLectures = append(cultureLectures, cultureLecture)
		}
	}

	mainC <- cultureLectures

	fmt.Println("이마트 문화센터 강좌 수집이 완료되었습니다.")
}

func extractEmartCultureLecture(cultureLecturePageURL string, storeName string, s *goquery.Selection, c chan<- cultureLecture) {
	if cleanString(s.Text()) == "검색된 강좌가 없습니다." {
		c <- cultureLecture{}
	} else {
		// @@@@@
		if s.Find("td").Length() != 5 {
			log.Fatalln("Request failed with Status:", cultureLecturePageURL)
		}

		title := cleanString(s.Find("td > a").Text())
		//val, _ := s.Find("td > a").Attr("href")
		//href := cleanString(val)
		//date := cleanString(ss.Eq(1).Text())
		//time := cleanString(ss.Eq(2).Text())
		//won := cleanString(ss.Eq(3).Text())

		c <- cultureLecture{
			storeName: storeName,
			title:     title,
			//href:      href,
			//date:      date,
			//time:      time,
			//won:       won,
		}
	}
}
