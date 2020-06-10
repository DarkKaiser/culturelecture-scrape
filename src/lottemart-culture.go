package main

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io/ioutil"
	"net/http"
	"strings"
)

const (
	lotteCultureBaseURL string = "http://culture.lottemart.com"

	// 검색 년도&시즌
	lotteSearchTermCode string = "202002"
)

// 점포
var lotteStoreCodeMap = map[string]string{
	"705": "롯데마트 여수점",
}

func scrapeLotteCultureLecture(mainC chan<- []cultureLecture) {
	// c := make(chan extractedJob)

	for storeCode, _ := range lotteStoreCodeMap {
		/*
		 * 불러올 전체 페이지수를 구한다.
		 */
		clPageURL := lotteCultureBaseURL + "/cu/gus/course/courseinfo/searchList.do"

		reqBody := bytes.NewBufferString("currPageNo=1&search_list_type=&search_str_cd=" + storeCode + "&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&search_term_cd=" + lotteSearchTermCode + "&search_day_fg=&search_cls_nm=&search_cat_cd=21%2C81%2C22%2C82%2C23%2C83%2C24%2C84%2C25%2C85%2C26%2C86%2C27%2C87%2C31%2C32%2C33%2C34%2C35%2C36%2C37%2C41%2C42%2C43%2C44%2C45%2C46%2C47%2C48&search_opt_cd=&search_tit_cd=&arr_cat_cd=21&arr_cat_cd=81&arr_cat_cd=22&arr_cat_cd=82&arr_cat_cd=23&arr_cat_cd=83&arr_cat_cd=24&arr_cat_cd=84&arr_cat_cd=25&arr_cat_cd=85&arr_cat_cd=26&arr_cat_cd=86&arr_cat_cd=27&arr_cat_cd=87&arr_cat_cd=31&arr_cat_cd=32&arr_cat_cd=33&arr_cat_cd=34&arr_cat_cd=35&arr_cat_cd=36&arr_cat_cd=37&arr_cat_cd=41&arr_cat_cd=42&arr_cat_cd=43&arr_cat_cd=44&arr_cat_cd=45&arr_cat_cd=46&arr_cat_cd=47&arr_cat_cd=48")

		res, err := http.Post(clPageURL, "application/x-www-form-urlencoded; charset=UTF-8", reqBody)
		checkErr(err)
		checkStatusCode(res)

		defer res.Body.Close()

		// tr, td가 모두 사라짐
		htmlData, err := ioutil.ReadAll(res.Body)
		checkErr(err)

		doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table>" + string(htmlData) + "</table>"))
		checkErr(err)

		//clSelection, _ := doc.Find("table > tbody > tr:nth-last-child(1)").Attr("pageInfo")
		searchCards := doc.Find("tr:last-child").Nodes
		println(searchCards[0].Attr[0].Val)

		//// currPageNo=1&search_list_type=&search_str_cd=705&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&lotteSearchTermCode=202002&search_day_fg=&search_cls_nm=&search_cat_cd=21%2C81%2C22%2C82%2C23%2C83%2C24%2C84%2C25%2C85%2C26%2C86%2C27%2C87%2C31%2C32%2C33%2C34%2C35%2C36%2C37&search_opt_cd=&search_tit_cd=&arr_cat_cd=21&arr_cat_cd=81&arr_cat_cd=22&arr_cat_cd=82&arr_cat_cd=23&arr_cat_cd=83&arr_cat_cd=24&arr_cat_cd=84&arr_cat_cd=25&arr_cat_cd=85&arr_cat_cd=26&arr_cat_cd=86&arr_cat_cd=27&arr_cat_cd=87&arr_cat_cd=31&arr_cat_cd=32&arr_cat_cd=33&arr_cat_cd=34&arr_cat_cd=35&arr_cat_cd=36&arr_cat_cd=37
		//var pageURL string = "http://culture.lottemart.com/cu/gus/course/courseinfo/searchList.do"
		//fmt.Println("Requesting;", pageURL)
		//
		////res, err := http.Get(pageURL)
		//reqBody := bytes.NewBufferString("currPageNo=1&search_list_type=&search_str_cd=" + storeCode + "&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&lotteSearchTermCode=" + lotteSearchTermCode + "&search_day_fg=&search_cls_nm=&search_cat_cd=21%2C81%2C22%2C82%2C23%2C83%2C24%2C84%2C25%2C85%2C26%2C86%2C27%2C87%2C31%2C32%2C33%2C34%2C35%2C36%2C37&search_opt_cd=&search_tit_cd=&arr_cat_cd=21&arr_cat_cd=81&arr_cat_cd=22&arr_cat_cd=82&arr_cat_cd=23&arr_cat_cd=83&arr_cat_cd=24&arr_cat_cd=84&arr_cat_cd=25&arr_cat_cd=85&arr_cat_cd=26&arr_cat_cd=86&arr_cat_cd=27&arr_cat_cd=87&arr_cat_cd=31&arr_cat_cd=32&arr_cat_cd=33&arr_cat_cd=34&arr_cat_cd=35&arr_cat_cd=36&arr_cat_cd=37")
		//res, err := http.Post(pageURL, "application/x-www-form-urlencoded; charset=UTF-8", reqBody)
		//checkErr(err)
		//checkStatusCode(res)
		//
		//defer res.Body.Close()
		//
		//htmlData, err := ioutil.ReadAll(res.Body)
		//ht := "<table>" + string(htmlData) + "</table>"
		////println(ht)
		//rHtml := strings.NewReader(ht)
		//
		//// tr, td가 모두 사라짐
		//doc, err := goquery.NewDocumentFromReader(rHtml)
		////doc, err := goquery.NewDocumentFromReader(res.Body)
		//checkErr(err)
		//
		//searchCards := doc.Find("tr")
		//searchCards.Each(func(i int, card *goquery.Selection) {
		//	//	go extractJob(card, c)
		//	extractJob22(card, storeName, "", c)
		//})

	}
}

func extractJob22(card *goquery.Selection, name string, groupName string, c chan<- extractedJob) {
	ss := card.Find("td")
	if ss.Length() == 1 {
		return
	}

	title := cleanString(card.Find("td > a").Text())
	val, _ := card.Find("td > a").Attr("href")
	href := cleanString(val)
	date := cleanString(ss.Eq(1).Text())
	time := cleanString(ss.Eq(2).Text())
	won := cleanString(ss.Eq(3).Text())

	job := extractedJob{
		storeName: name,
		groupName: groupName,
		title:     title,
		href:      href,
		date:      date,
		time:      time,
		won:       won,
	}
	fmt.Println(job)

	//c <- extractedJob{
	//	//id:       id,
	//	title:    title,
	//	//location: location,
	//	//salary:   salary,
	//	//summary:  summary,
	//}
}
