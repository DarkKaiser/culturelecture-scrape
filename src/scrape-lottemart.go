package main

import (
	"bytes"
	"github.com/PuerkitoBio/goquery"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const (
	lottemartCultureBaseURL string = "http://culture.lottemart.com"

	// 검색년도 & 검색시즌
	lottemartSearchTermCode string = "202002"
)

/*
 * 점포
 */
var lottemartStoreCodeMap = map[string]string{
	"705": "롯데마트 여수점",
}

func scrapeLottemartCultureLecture(mainC chan<- []cultureLecture) {
	log.Println("롯데마트 문화센터 강좌 수집을 시작합니다.(검색조건:" + lottemartSearchTermCode + ")")

	var wait sync.WaitGroup

	c := make(chan cultureLecture, 10)

	count := 0
	for storeCode, storeName := range lottemartStoreCodeMap {
		// 불러올 전체 페이지 갯수를 구한다.
		_, doc := getLottemartCultureLecturePageDocument(1, storeCode)
		pi, exists := doc.Find("tr:last-child").Attr("pageinfo")
		if exists == false {
			log.Fatalln("롯데마트 문화센터 강좌를 수집하는 중에 전체 페이지 갯수 추출이 실패하였습니다.")
		}

		// pageinfo 구조
		// --------------------
		// 1|5|85|61|0|24
		//
		// 1  : 현재 페이지 번호
		// 5  : 전체 페이지 번호
		// 85 : 전체 강좌 갯수
		// 61 : 접수가능 갯수
		// 0  : 온라인마감 갯수
		// 24 : 접수마감 갯수
		piArray := strings.Split(pi, "|")
		if len(piArray) != 6 {
			log.Fatalln("롯데마트 문화센터 강좌를 수집하는 중에 전체 페이지 갯수 추출이 실패하였습니다.(pageinfo:" + pi + ")")
		}

		pageCount, err := strconv.Atoi(piArray[1])
		checkErr(err)

		// 강좌 데이터를 수집한다.
		for pageNo := 1; pageNo <= pageCount; pageNo++ {
			wait.Add(1)
			go func(storeCode string, storeName string, pageNo int) {
				defer wait.Done()

				clPageURL, doc := getLottemartCultureLecturePageDocument(pageNo, storeCode)

				clSelection := doc.Find("tr")
				clSelection.Each(func(i int, s *goquery.Selection) {
					count += 1
					go extractLottemartCultureLecture(clPageURL, storeName, s, c)
				})
			}(storeCode, storeName, pageNo)
		}
	}

	wait.Wait()

	var cultureLectures []cultureLecture
	for i := 0; i < count; i++ {
		cultureLecture := <-c
		if len(cultureLecture.title) > 0 {
			cultureLectures = append(cultureLectures, cultureLecture)
		}
	}

	log.Println("롯데마트 문화센터 강좌 수집이 완료되었습니다. 총 " + strconv.Itoa(len(cultureLectures)) + "개의 강좌가 수집되었습니다.")

	mainC <- cultureLectures
}

func extractLottemartCultureLecture(clPageURL string, storeName string, s *goquery.Selection, c chan<- cultureLecture) {
	// @@@@@
	//println("###", s.Text())

	c <- cultureLecture{
		storeName: storeName,
		title:     "1",
		//href:      href,
		//date:      date,
		//time:      time,
		//won:       won,
	}
}

func getLottemartCultureLecturePageDocument(pageNo int, storeCode string) (string, *goquery.Document) {
	clPageURL := lottemartCultureBaseURL + "/cu/gus/course/courseinfo/searchList.do"

	reqBody := bytes.NewBufferString("currPageNo=" + strconv.Itoa(pageNo) + "&search_list_type=&search_str_cd=" + storeCode + "&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&search_term_cd=" + lottemartSearchTermCode + "&search_day_fg=&search_cls_nm=&search_cat_cd=21%2C81%2C22%2C82%2C23%2C83%2C24%2C84%2C25%2C85%2C26%2C86%2C27%2C87%2C31%2C32%2C33%2C34%2C35%2C36%2C37%2C41%2C42%2C43%2C44%2C45%2C46%2C47%2C48&search_opt_cd=&search_tit_cd=&arr_cat_cd=21&arr_cat_cd=81&arr_cat_cd=22&arr_cat_cd=82&arr_cat_cd=23&arr_cat_cd=83&arr_cat_cd=24&arr_cat_cd=84&arr_cat_cd=25&arr_cat_cd=85&arr_cat_cd=26&arr_cat_cd=86&arr_cat_cd=27&arr_cat_cd=87&arr_cat_cd=31&arr_cat_cd=32&arr_cat_cd=33&arr_cat_cd=34&arr_cat_cd=35&arr_cat_cd=36&arr_cat_cd=37&arr_cat_cd=41&arr_cat_cd=42&arr_cat_cd=43&arr_cat_cd=44&arr_cat_cd=45&arr_cat_cd=46&arr_cat_cd=47&arr_cat_cd=48")
	res, err := http.Post(clPageURL, "application/x-www-form-urlencoded; charset=UTF-8", reqBody)
	checkErr(err)
	checkStatusCode(res)

	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	checkErr(err)

	// 실제로 불러온 데이터는 '<table>' 태그가 포함되어 있지 않고 '<tr>', '<td>'만 있는 형태
	// 이 형태에서 goquery.NewDocumentFromReader() 함수를 호출하면 '<tr>', '<td>' 태그가 모두 사라지므로 '<table>' 태그를 강제로 붙여준다.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table>" + string(resBodyBytes) + "</table>"))
	checkErr(err)

	return clPageURL, doc
}
