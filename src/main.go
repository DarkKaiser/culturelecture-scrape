package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"
)

type cultureLecture struct {
	storeName     string // 점포
	title         string // 강좌명
	teacher       string // 강사명
	startDate     string // 개강일(YYYY-MM-DD)
	startTime     string // 시작시간(hh:mm)
	endTime       string // 종료시간(hh:mm)
	dayOfTheWeek  string // 요일
	price         string // 수강료
	count         string // 강좌횟수
	status        string // 접수상태
	detailPageUrl string // 상세페이지
}

func main() {
	log.Println("문화센터 강좌 수집을 시작합니다.")

	c := make(chan []cultureLecture)

	var goroutineCnt = 0
	//go scrapeEmartCultureLecture(c)
	//goroutineCnt++
	//go scrapeLottemartCultureLecture(c)
	//goroutineCnt++
	go scrapeHomeplusCultureLecture(c)
	goroutineCnt++

	var cultureLectures []cultureLecture
	for i := 0; i < goroutineCnt; i++ {
		cultureLecturesScraped := <-c
		cultureLectures = append(cultureLectures, cultureLecturesScraped...)
	}

	log.Println("문화센터 강좌 수집이 완료되었습니다. 파일로 저장합니다.")

	// 필터추가
	// @@@@@
	for _, ddd := range cultureLectures {
		println(ddd.detailPageUrl)
	}

	writeCultureLectures(cultureLectures)
}

func writeCultureLectures(cultureLectures []cultureLecture) {
	t := time.Now()
	fName := fmt.Sprintf("cultureLecture-%d%02d%02d%02d%02d%02d.csv", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())

	f, err := os.Create(fName)
	checkErr(err)

	defer f.Close()

	// 파일 첫 부분에 UTF-8 BOM을 추가한다.
	_, err = f.WriteString("\xEF\xBB\xBF")
	checkErr(err)

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{"점포", "강좌명", "강사명", "개강일", "시작시간", "종료시간", "요일", "수강료", "강좌횟수", "접수상태", "상세페이지"}
	err = w.Write(headers)
	checkErr(err)

	for _, cultureLecture := range cultureLectures {
		r := []string{
			cultureLecture.storeName,
			cultureLecture.title,
			cultureLecture.teacher,
			cultureLecture.startDate,
			cultureLecture.startTime,
			cultureLecture.endTime,
			cultureLecture.dayOfTheWeek,
			cultureLecture.price,
			cultureLecture.count,
			cultureLecture.status,
			cultureLecture.detailPageUrl,
		}
		err := w.Write(r)
		checkErr(err)
	}

	log.Println("수집된 문화센터 강좌 자료를 파일(" + fName + ")로 저장하였습니다.")
}
