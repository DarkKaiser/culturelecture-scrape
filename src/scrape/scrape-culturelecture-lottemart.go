package scrape

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"helpers"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	lottemart = "롯데마트"

	lottemartCultureBaseURL = "http://culture.lottemart.com"

	// 검색년도 & 검색시즌 코드
	lottemartSearchTermCode = SearchYear + "0" + SearchSeasonCode
)

/*
 * 점포
 */
var lottemartStoreCodeMap = map[string]string{
	"705": "여수점",
}

func scrapeLottemartCultureLecture(mainC chan<- []cultureLecture) {
	log.Printf("%s 문화센터 강좌 수집을 시작합니다.(검색조건:%s)", lottemart, lottemartSearchTermCode)

	var wait sync.WaitGroup

	c := make(chan *cultureLecture, 10)

	count := 0
	for storeCode, storeName := range lottemartStoreCodeMap {
		// 불러올 전체 페이지 갯수를 구한다.
		_, doc := lottemartCultureLecturePageDocument(1, storeCode)
		pi, exists := doc.Find("tr:last-child").Attr("pageinfo")
		if exists == false {
			log.Fatalf("%s 문화센터 강좌를 수집하는 중에 전체 페이지 갯수 추출이 실패하였습니다.", lottemart)
		}

		// --------------------
		// pageinfo 값 형식
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
			log.Fatalf("%s 문화센터 강좌를 수집하는 중에 전체 페이지 갯수 추출이 실패하였습니다.(pageinfo:%s)", lottemart, pi)
		}

		totalPageCount, err := strconv.Atoi(piArray[1])
		helpers.CheckErr(err)

		// 강좌 데이터를 수집한다.
		for pageNo := 1; pageNo <= totalPageCount; pageNo++ {
			wait.Add(1)
			go func(storeCode string, storeName string, pageNo int) {
				defer wait.Done()

				clPageURL, doc := lottemartCultureLecturePageDocument(pageNo, storeCode)

				clSelection := doc.Find("tr")
				clSelection.Each(func(i int, s *goquery.Selection) {
					count += 1
					go extractLottemartCultureLecture(clPageURL, storeCode, storeName, s, c)
				})
			}(storeCode, storeName, pageNo)
		}
	}

	wait.Wait()

	var cultureLectures []cultureLecture
	for i := 0; i < count; i++ {
		cultureLecture := <-c
		if len(cultureLecture.title) > 0 {
			cultureLectures = append(cultureLectures, *cultureLecture)
		}
	}

	log.Printf("%s 문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", lottemart, len(cultureLectures))

	mainC <- cultureLectures
}

func extractLottemartCultureLecture(clPageURL string, storeCode string, storeName string, s *goquery.Selection, c chan<- *cultureLecture) {
	// 강좌의 컬럼 개수를 확인한다.
	ls := s.Find("td")
	if ls.Length() != 5 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 컬럼 개수 불일치:%d, URL:%s)", lottemart, ls.Length(), clPageURL)
	}

	lectureCol2 := helpers.CleanString(ls.Eq(1 /* 강사명 */).Text())
	lectureCol3 := helpers.CleanString(ls.Eq(2 /* 요일/시간/개강일 */).Text())
	lectureCol4 := helpers.CleanString(ls.Eq(3 /* 수강료 */).Text())
	lectureCol5 := helpers.CleanString(ls.Eq(4 /* 접수상태/수강신청 */).Find("div > div > a.btn-status:last-child").Text())

	// 강좌명
	lts := ls.Eq(0 /* 강좌명 */).Find("div.info-txt > a")
	if lts.Length() == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌명 <a> 태그를 찾을 수 없습니다, URL:%s)", lottemart, clPageURL)
	}
	title := helpers.CleanString(lts.Text())

	// 개강일
	startDate := regexp.MustCompile("[0-9]{4}\\.[0-9]{2}\\.[0-9]{2}$").FindString(lectureCol3)
	if len(startDate) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", lottemart, lectureCol3, clPageURL)
	}
	startDate = strings.ReplaceAll(startDate, ".", "-")

	// 시작시간, 종료시간
	startTime := strings.TrimSpace(regexp.MustCompile(" [0-9]{2}:[0-9]{2}").FindString(lectureCol3))
	endTime := strings.TrimSpace(regexp.MustCompile("[0-9]{2}:[0-9]{2} ").FindString(lectureCol3))
	if len(startDate) == 0 || len(endTime) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", lottemart, lectureCol3, clPageURL)
	}

	// 요일
	dayOfTheWeek := regexp.MustCompile("\\([월화수목금토일]{1}").FindString(lectureCol3)
	if len(dayOfTheWeek) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", lottemart, lectureCol3, clPageURL)
	}
	dayOfTheWeek = string([]rune(dayOfTheWeek)[1:])

	// 수강료
	price := regexp.MustCompile("[0-9,]{1,8}원$").FindString(lectureCol4)
	if strings.Contains(price, "원") == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", lottemart, lectureCol4, clPageURL)
	}

	// 강좌횟수
	count := regexp.MustCompile("[0-9]{1,3}회").FindString(lectureCol4)
	if len(count) == 0 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", lottemart, lectureCol4, clPageURL)
	}

	// 접수상태
	var status ReceptionStatus = ReceptionStatusUnknown
	switch lectureCol5 {
	case "바로신청":
		status = ReceptionStatusPossible
	case "접수마감":
		status = ReceptionStatusClosed
	case "대기자 신청":
		status = ReceptionStatusStnadBy
	default:
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", lottemart, lectureCol5, clPageURL)
	}

	// 상세페이지
	classCode, exists := lts.Attr("onclick")
	if exists == false {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지 주소를 찾을 수 없습니다, URL:%s)", lottemart, clPageURL)
	}
	pos1 := strings.Index(classCode, "'")
	pos2 := strings.LastIndex(classCode, "'")
	if pos1 == -1 || pos2 == -1 || pos1 == pos2 {
		log.Fatalf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지 주소를 찾을 수 없습니다, URL:%s)", lottemart, clPageURL)
	}
	classCode = classCode[pos1+1 : pos2]

	c <- &cultureLecture{
		storeName:      fmt.Sprintf("%s %s", lottemart, storeName),
		group:          "",
		title:          title,
		teacher:        lectureCol2,
		startDate:      startDate,
		startTime:      startTime,
		endTime:        endTime,
		dayOfTheWeek:   dayOfTheWeek + "요일",
		price:          price,
		count:          count,
		status:         status,
		detailPageUrl:  fmt.Sprintf("%s/cu/gus/course/courseinfo/courseview.do?cls_cd=%s&is_category_open=N&search_term_cd=%s&search_str_cd=%s", lottemartCultureBaseURL, classCode, lottemartSearchTermCode, storeCode),
		scrapeExcluded: false,
	}
}

func lottemartCultureLecturePageDocument(pageNo int, storeCode string) (string, *goquery.Document) {
	clPageURL := lottemartCultureBaseURL + "/cu/gus/course/courseinfo/searchList.do"

	reqBody := bytes.NewBufferString(fmt.Sprintf("currPageNo=%d&search_list_type=&search_str_cd=%s&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&search_term_cd=%s&search_day_fg=&search_cls_nm=&search_cat_cd=21%2C81%2C22%2C82%2C23%2C83%2C24%2C84%2C25%2C85%2C26%2C86%2C27%2C87%2C31%2C32%2C33%2C34%2C35%2C36%2C37%2C41%2C42%2C43%2C44%2C45%2C46%2C47%2C48&search_opt_cd=&search_tit_cd=&arr_cat_cd=21&arr_cat_cd=81&arr_cat_cd=22&arr_cat_cd=82&arr_cat_cd=23&arr_cat_cd=83&arr_cat_cd=24&arr_cat_cd=84&arr_cat_cd=25&arr_cat_cd=85&arr_cat_cd=26&arr_cat_cd=86&arr_cat_cd=27&arr_cat_cd=87&arr_cat_cd=31&arr_cat_cd=32&arr_cat_cd=33&arr_cat_cd=34&arr_cat_cd=35&arr_cat_cd=36&arr_cat_cd=37&arr_cat_cd=41&arr_cat_cd=42&arr_cat_cd=43&arr_cat_cd=44&arr_cat_cd=45&arr_cat_cd=46&arr_cat_cd=47&arr_cat_cd=48", pageNo, storeCode, lottemartSearchTermCode))
	res, err := http.Post(clPageURL, "application/x-www-form-urlencoded; charset=UTF-8", reqBody)
	helpers.CheckErr(err)
	helpers.CheckStatusCode(res)

	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	helpers.CheckErr(err)

	// 실제로 불러온 데이터는 '<table>' 태그가 포함되어 있지 않고 '<tr>', '<td>'만 있는 형태
	// 이 형태에서 goquery.NewDocumentFromReader() 함수를 호출하면 '<tr>', '<td>' 태그가 모두 사라지므로 '<table>' 태그를 강제로 붙여준다.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table>" + string(resBodyBytes) + "</table>"))
	helpers.CheckErr(err)

	return clPageURL, doc
}
