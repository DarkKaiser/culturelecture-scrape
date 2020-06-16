package main

import (
	"log"
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
	var goroutineCnt = 0

	log.Println("문화센터 강좌 수집을 시작합니다.")

	c := make(chan []cultureLecture)

	go scrapeEmartCultureLecture(c)
	goroutineCnt++
	go scrapeLottemartCultureLecture(c)
	goroutineCnt++
	//go scrapeHomeplusCultureLecture(c)
	//goroutineCnt++

	var cultureLectures []cultureLecture
	for i := 0; i < goroutineCnt; i++ {
		cultureLecturesScraped := <-c
		cultureLectures = append(cultureLectures, cultureLecturesScraped...)
	}

	log.Println("문화센터 강좌 수집이 완료되었습니다.")

	// @@@@@
	//for _, ddd := range cultureLectures {
	//	println(ddd.detailPageUrl)
	//}

	writeJobs(cultureLectures)
}

func writeJobs(jobs []cultureLecture) {
	// @@@@@
	//file, err := os.Create("jobs.csv")
	//checkErr(err)
	//
	//w := csv.NewWriter(file)
	//defer w.Flush()
	//
	//headers := []string{"Link", "Title", "Location", "Salary", "Summary"}
	//
	//wErr := w.Write(headers)
	//checkErr(wErr)
	//
	//for _, job := range jobs {
	//	jobSlice := []string{"https://kr.indeed.com/viewjob?jk=" + job.id, job.title, job.location, job.salary, job.summary}
	//	jwErr := w.Write(jobSlice)
	//	checkErr(jwErr)
	//}
}
