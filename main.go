package main

import (
	"fmt"
	"github.com/darkkaiser/culturelecture-scrape/scrape"
	"time"
)

// 문화센터 강좌 수강자
var cultureLecturer = struct {
	YearOfBirth  int
	MonthOfBirth int
	DayOfBirth   int
}{2016, 3, 18}

/********************************************************************************/
/* 강좌 수집 작업시에 변경되는 값 BEGIN                                              */
/****************************************************************************** */

// 검색년도
var searchYearCode = "2021"

// 검색시즌(봄:1, 여름:2, 가을:3, 겨울:4)
var searchSeasonCode = "2"

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
	now := time.Now()

	// 강좌 수강자의 나이 및 개월수를 계산한다.
	cultureLecturerAge := now.Year() - cultureLecturer.YearOfBirth + 1

	cultureLecturerMonths := 0
	cultureLecturerBirthday := time.Date(cultureLecturer.YearOfBirth, time.Month(cultureLecturer.MonthOfBirth), cultureLecturer.DayOfBirth, 0, 0, 0, 0, time.Local)
	for {
		cultureLecturerBirthday = cultureLecturerBirthday.AddDate(0, 1, 0)
		if cultureLecturerBirthday.Unix() > now.Unix() {
			break
		}

		cultureLecturerMonths += 1
	}

	fmt.Println("########################################################")
	fmt.Println("###                                                  ###")
	fmt.Println("###           culturelecture-scrape 0.0.6            ###")
	fmt.Println("###                                                  ###")
	fmt.Println("###                         developed by DarkKaiser  ###")
	fmt.Println("###                                                  ###")
	fmt.Println("########################################################")
	fmt.Println("")
	fmt.Println(fmt.Sprintf("문화센터 강좌 수강자 정보는 %d세(%d개월) 아이입니다.\n", cultureLecturerAge, cultureLecturerMonths))

	s := scrape.New()
	s.Scrape(searchYearCode, searchSeasonCode)
	s.Filter(cultureLecturerMonths, cultureLecturerAge, holidays)

	s.ExportCSV(fmt.Sprintf("culturelecture-scrape-%d%02d%02d%02d%02d%02d.csv", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second()))
}
