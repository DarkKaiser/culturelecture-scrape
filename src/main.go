package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

const (
	/********************************************************************************/
	/* 강좌 수집 작업시에 변경되는 값                                                    */
	/****************************************************************************** */
	// 검색년도
	SearchYear = "2020"

	// 검색시즌(봄:1, 여름:2, 가을:3, 겨울:4)
	SearchSeasonCode = "2"
	/********************************************************************************/
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

	c := make(chan []cultureLecture, 3)

	var goRoutineCount = 0
	go scrapeEmartCultureLecture(c)
	goRoutineCount++
	go scrapeLottemartCultureLecture(c)
	goRoutineCount++
	//go scrapeHomeplusCultureLecture(c)
	//goroutineCnt++

	var cultureLectures []cultureLecture
	for i := 0; i < goRoutineCount; i++ {
		cultureLecturesScraped := <-c
		cultureLectures = append(cultureLectures, cultureLecturesScraped...)
	}

	log.Println("문화센터 강좌 수집이 완료되었습니다. 총 " + strconv.Itoa(len(cultureLectures)) + "개의 강좌가 수집되었습니다.")

	// 필터추가
	// 평일 4시 이전 강좌는 모두 제외
	// 개월수/나이에 포함되지 않으면 제외
	// 접수상태
	// @@@@@
	//for _, ddd := range cultureLectures {
	//	println(ddd.detailPageUrl)
	//}

	writeCultureLectures(cultureLectures)
}

func writeCultureLectures(cultureLectures []cultureLecture) {
	log.Println("수집된 문화센터 강좌 자료를 파일로 저장합니다.")

	now := time.Now()
	fName := fmt.Sprintf("cultureLecture-%d%02d%02d%02d%02d%02d.csv", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())

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
