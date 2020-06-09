package main

import (
	"encoding/csv"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"os"
)

type CultureLecture struct {
	storeName     string
	title         string // 강좌명
	teacher       string // 강사명
	startDate     string // 개강일
	time          string // 시간
	dayOfTheWeek  string // 요일
	price         int    // 수강료
	count         int    // 강좌횟수
	status        int    // 접수상태
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
	c := make(chan []CultureLecture)

	go scrapeEmartCultureLecture(c)
	go scrapeLottemartCultureLecture(c)

	var cultureLectures []CultureLecture
	for i := 0; i < 1; i++ {
		cultureLecture := <-c
		cultureLectures = append(cultureLectures, cultureLecture...)
	}

	fmt.Println("수집 작업이 완료되었습니다.")
}

//
//func getPage(page int, mainC chan<- []extractedJob) {
//	//var jobs []extractedJob
//	c := make(chan extractedJob)
//
//	pageURL := baseURL
//	fmt.Println("Requesting;", pageURL)
//
//	res, err := http.Get(pageURL)
//	checkErr(err)
//	checkStatusCode(res)
//
//	defer res.Body.Close()
//
//	doc, err := goquery.NewDocumentFromReader(res.Body)
//	checkErr(err)
//
//	searchCards := doc.Find("div.board_list > table > tbody > tr")
//	searchCards.Each(func(i int, card *goquery.Selection) {
//	//	go extractJob(card, c)
//		extractJob(card, c)
//	})
//
//	//for i := 0; i < searchCards.Length(); i++ {
//	//	job := <-c
//	//	jobs = append(jobs, job)
//	//}
//	//
//	//mainC <- jobs
//}

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

//func getPages() int {
//	pages := 0
//
//	res, err := http.Get(baseURL)
//	checkErr(err)
//	checkStatusCode(res)
//
//	defer res.Body.Close()
//
//	doc, err := goquery.NewDocumentFromReader(res.Body)
//	checkErr(err)
//
//	doc.Find(".pagination").Each(func(i int, s *goquery.Selection) {
//		pages = s.Find("a").Length()
//	})
//
//	return pages
//}

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
