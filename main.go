package main

import (
	"fmt"
	"github.com/darkkaiser/scrape-culturelecture/scrape"
	"time"
)

/********************************************************************************/
/* 강좌 수집 작업시에 변경되는 값 BEGIN                                              */
/****************************************************************************** */

// 검색년도
var searchYearCode = "2020"

// 검색시즌(봄:1, 여름:2, 가을:3, 겨울:4)
var searchSeasonCode = "2"

// 강좌를 수강하는 아이 개월수
var childrenMonths = 51

// 강좌를 수강하는 아이 나이
var childrenAge = 5

// 공휴일(2020년도)
var holidays = []string{
	"2020-01-01",
	"2020-01-24", "2020-01-25", "2020-01-26", "2020-01-27",
	"2020-03-01",
	"2020-04-30",
	"2020-05-05",
	"2020-06-06",
	"2020-08-15",
	"2020-09-30", "2020-10-01", "2020-10-02",
	"2020-10-03",
	"2020-10-09",
	"2020-12-25",
}

/********************************************************************************/
/* 강좌 수집 작업시에 변경되는 값 END                                                */
/****************************************************************************** */

func main() {
	fmt.Println("########################################################")
	fmt.Println("###                                                  ###")
	fmt.Println("###           scrape-culturelecture 0.0.2            ###")
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
