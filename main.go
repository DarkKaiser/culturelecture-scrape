package main

import (
	"fmt"
	"github.com/darkkaiser/culturelecture-scrape/scrape"
	"time"
)

/********************************************************************************/
/* 강좌 수집 작업시에 변경되는 값 BEGIN                                              */
/****************************************************************************** */

// 검색년도
var searchYearCode = "2021"

// 검색시즌(봄:1, 여름:2, 가을:3, 겨울:4)
var searchSeasonCode = "2"

// 강좌를 수강하는 아이 개월수
var childrenMonths = 62

// 강좌를 수강하는 아이 나이
var childrenAge = 6

// 공휴일(2021년도)
var holidays = []string{
	"2021-01-01",
	"2021-02-11", "2021-02-12", "2021-02-13",
	"2021-03-01",
	"2021-05-05",
	"2021-05-19",
	"2021-06-06",
	"2021-08-15",
	"2021-09-20", "2021-09-21", "2021-09-22",
	"2021-10-03",
	"2021-10-09",
	"2021-12-25",
}

/********************************************************************************/
/* 강좌 수집 작업시에 변경되는 값 END                                                */
/****************************************************************************** */

func main() {
	fmt.Println("########################################################")
	fmt.Println("###                                                  ###")
	fmt.Println("###           scrape-culturelecture 0.0.4            ###")
	fmt.Println("###                                                  ###")
	fmt.Println("###                         developed by DarkKaiser  ###")
	fmt.Println("###                                                  ###")
	fmt.Println("########################################################")
	fmt.Println("")

	s := scrape.New()
	s.Scrape(searchYearCode, searchSeasonCode)
	s.Filter(childrenMonths, childrenAge, holidays)

	now := time.Now()
	s.ExportCSV(fmt.Sprintf("culturelecture-scrape-%d%02d%02d%02d%02d%02d.csv", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second()))
}
