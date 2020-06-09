package main

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io/ioutil"
	"net/http"
	"strings"
)

// currPageNo=1&search_list_type=&search_str_cd=705&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&lottemartSearchTermCode=202002&search_day_fg=&search_cls_nm=&search_cat_cd=10&search_opt_cd=&search_tit_cd=&arr_cat_cd=10
//	search_day_fg=&search_cls_nm=&search_cat_cd=21%2C81%2C22%2C82%2C23%2C83%2C24%2C84%2C25%2C85%2C26%2C86%2C27%2C87%2C31%2C32%2C33%2C34%2C35%2C36%2C37&search_opt_cd=&search_tit_cd=&arr_cat_cd=21&arr_cat_cd=81&arr_cat_cd=22&arr_cat_cd=82&arr_cat_cd=23&arr_cat_cd=83&arr_cat_cd=24&arr_cat_cd=84&arr_cat_cd=25&arr_cat_cd=85&arr_cat_cd=26&arr_cat_cd=86&arr_cat_cd=27&arr_cat_cd=87&arr_cat_cd=31&arr_cat_cd=32&arr_cat_cd=33&arr_cat_cd=34&arr_cat_cd=35&arr_cat_cd=36&arr_cat_cd=37
//	search_day_fg=&search_cls_nm=&search_cat_cd=21%2C81%2C22%2C82%2C23%2C83%2C24%2C84%2C25%2C85%2C26%2C86%2C27%2C87%2C31%2C32%2C33%2C34%2C35%2C36%2C37&search_opt_cd=&search_tit_cd=&arr_cat_cd=21&arr_cat_cd=81&arr_cat_cd=22&arr_cat_cd=82&arr_cat_cd=23&arr_cat_cd=83&arr_cat_cd=24&arr_cat_cd=84&arr_cat_cd=25&arr_cat_cd=85&arr_cat_cd=26&arr_cat_cd=86&arr_cat_cd=27&arr_cat_cd=87&arr_cat_cd=31&arr_cat_cd=32&arr_cat_cd=33&arr_cat_cd=34&arr_cat_cd=35&arr_cat_cd=36&arr_cat_cd=37

const (
	// 검색 년도&시즌
	lottemartSearchTermCode string = "202002"
)

// 점포
var lottemartStoreCodeMap = map[string]string{
	"705": "롯데마트 여수점",
}

func scrapeLottemartCultureLecture(mainC chan<- []CultureLecture) {
	c := make(chan extractedJob)

	for storeCode, storeName := range lottemartStoreCodeMap {
		// currPageNo=1&search_list_type=&search_str_cd=705&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&lottemartSearchTermCode=202002&search_day_fg=&search_cls_nm=&search_cat_cd=21%2C81%2C22%2C82%2C23%2C83%2C24%2C84%2C25%2C85%2C26%2C86%2C27%2C87%2C31%2C32%2C33%2C34%2C35%2C36%2C37&search_opt_cd=&search_tit_cd=&arr_cat_cd=21&arr_cat_cd=81&arr_cat_cd=22&arr_cat_cd=82&arr_cat_cd=23&arr_cat_cd=83&arr_cat_cd=24&arr_cat_cd=84&arr_cat_cd=25&arr_cat_cd=85&arr_cat_cd=26&arr_cat_cd=86&arr_cat_cd=27&arr_cat_cd=87&arr_cat_cd=31&arr_cat_cd=32&arr_cat_cd=33&arr_cat_cd=34&arr_cat_cd=35&arr_cat_cd=36&arr_cat_cd=37
		var pageURL string = "http://culture.lottemart.com/cu/gus/course/courseinfo/searchList.do"
		fmt.Println("Requesting;", pageURL)

		//res, err := http.Get(pageURL)
		reqBody := bytes.NewBufferString("currPageNo=1&search_list_type=&search_str_cd=" + storeCode + "&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&lottemartSearchTermCode=" + lottemartSearchTermCode + "&search_day_fg=&search_cls_nm=&search_cat_cd=21%2C81%2C22%2C82%2C23%2C83%2C24%2C84%2C25%2C85%2C26%2C86%2C27%2C87%2C31%2C32%2C33%2C34%2C35%2C36%2C37&search_opt_cd=&search_tit_cd=&arr_cat_cd=21&arr_cat_cd=81&arr_cat_cd=22&arr_cat_cd=82&arr_cat_cd=23&arr_cat_cd=83&arr_cat_cd=24&arr_cat_cd=84&arr_cat_cd=25&arr_cat_cd=85&arr_cat_cd=26&arr_cat_cd=86&arr_cat_cd=27&arr_cat_cd=87&arr_cat_cd=31&arr_cat_cd=32&arr_cat_cd=33&arr_cat_cd=34&arr_cat_cd=35&arr_cat_cd=36&arr_cat_cd=37")
		res, err := http.Post(pageURL, "application/x-www-form-urlencoded; charset=UTF-8", reqBody)
		checkErr(err)
		checkStatusCode(res)

		defer res.Body.Close()

		htmlData, err := ioutil.ReadAll(res.Body)
		ht := "<table>" + string(htmlData) + "</table>"
		//println(ht)
		rHtml := strings.NewReader(ht)

		// tr, td가 모두 사라짐
		doc, err := goquery.NewDocumentFromReader(rHtml)
		//doc, err := goquery.NewDocumentFromReader(res.Body)
		checkErr(err)

		val, _ := doc.Html()
		println(val)

		searchCards := doc.Find("tr")
		searchCards.Each(func(i int, card *goquery.Selection) {
			//	go extractJob(card, c)
			extractJob22(card, storeName, "", c)
		})

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
