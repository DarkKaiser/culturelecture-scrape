package main

import (
	"encoding/csv"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"os"
)

type cultureLecture struct {
	storeName     string // 점포
	title         string // 강좌명
	teacher       string // 강사명
	startDate     string // 개강일
	time          string // 시간
	dayOfTheWeek  string // 요일
	price         string // 수강료
	count         string // 강좌횟수
	status        string // 접수상태
	detailPageUrl string // 상세페이지
}

type extractedJob struct {
	storeName string
	groupName string

	title string
	href  string
	date  string
	time  string
	won   string

	id       string
	location string
	salary   string
	summary  string
}

func main() {
	c := make(chan []cultureLecture)

	//go scrapeEmartCultureLecture(c)
	//go scrapeLottemartCultureLecture(c)
	go scrapeHomeplusCultureLecture(c)

	var cultureLectures []cultureLecture
	for i := 0; i < 1; i++ {
		cultureLectureList := <-c
		cultureLectures = append(cultureLectures, cultureLectureList...)
	}

	for _, ddd := range cultureLectures {
		println(ddd.teacher)
	}

	fmt.Println("수집 작업이 완료되었습니다.")
}

func extractJob(card *goquery.Selection, c chan<- extractedJob) {
	//id, _ := card.Attr("data-jk")
	title := cleanString(card.Find("td > a").Text())
	fmt.Println(title)
	//location := cleanString(card.Find(".sjcl").Text())
	//salary := cleanString(card.Find(".salaryText").Text())
	//summary := cleanString(card.Find(".summary").Text())

	//c <- extractedJob{
	//	//id:       id,
	//	title:    title,
	//	//location: location,
	//	//salary:   salary,
	//	//summary:  summary,
	//}
}

func writeJobs(jobs []extractedJob) {
	file, err := os.Create("jobs.csv")
	checkErr(err)

	w := csv.NewWriter(file)
	defer w.Flush()

	headers := []string{"Link", "Title", "Location", "Salary", "Summary"}

	wErr := w.Write(headers)
	checkErr(wErr)

	for _, job := range jobs {
		jobSlice := []string{"https://kr.indeed.com/viewjob?jk=" + job.id, job.title, job.location, job.salary, job.summary}
		jwErr := w.Write(jobSlice)
		checkErr(jwErr)
	}
}
