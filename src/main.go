package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"math"
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

	// 강좌를 수강하는 아이 개월수
	childrenMonths = 51

	// 강좌를 수강하는 아이 나이
	childrenAge = 5
	/********************************************************************************/
)

// 접수상태
type ReceptionStatus uint

// 지원가능한 접수상태 값
const (
	ReceptionStatusUnknown                   = iota // 알수없음
	ReceptionStatusPossible                         // 접수가능
	ReceptionStatusClosed                           // 접수마감
	ReceptionStatusStnadBy                          // 대기신청
	ReceptionStatusVisitConsultation                // 방문상담
	ReceptionStatusVisitFirstComeFirstServed        // 방문선착순
	ReceptionStatusDayParticipation                 // 당일참여
)

// 지원가능한 접수상태 문자열
var ReceptionStatusString = []string{"알수없음", "접수가능", "접수마감", "대기신청", "방문상담", "방문선착순", "당일참여"}

// 연령제한타입
type AgeLimitType uint

// 지원가능한 연령제한타입 값
const (
	AgeLimitTypeUnknwon = iota // 알수없음
	AgeLimitTypeAge            // 나이
	AgeLimitTypeMonths         // 개월수
)

type cultureLecture struct {
	storeName      string          // 점포
	group          string          // 강좌그룹
	title          string          // 강좌명
	teacher        string          // 강사명
	startDate      string          // 개강일(YYYY-MM-DD)
	startTime      string          // 시작시간(hh:mm) : 24시간 형식
	endTime        string          // 종료시간(hh:mm) : 24시간 형식
	dayOfTheWeek   string          // 요일
	price          string          // 수강료
	count          string          // 강좌횟수
	status         ReceptionStatus // 접수상태
	detailPageUrl  string          // 상세페이지
	scrapeExcluded bool            // 필터링에 걸려서 파일 저장시 제외되는지의 여부(csv 파일에 포함되지 않는다)
}

func main() {
	log.Println("문화센터 강좌 수집을 시작합니다.")

	c := make(chan []cultureLecture, 3)

	var goroutineCount = 0
	go scrapeEmartCultureLecture(c)
	goroutineCount++
	go scrapeLottemartCultureLecture(c)
	goroutineCount++
	go scrapeHomeplusCultureLecture(c)
	goroutineCount++

	var cultureLectures []cultureLecture
	for i := 0; i < goroutineCount; i++ {
		cultureLecturesScraped := <-c
		cultureLectures = append(cultureLectures, cultureLecturesScraped...)
	}

	log.Printf("문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", len(cultureLectures))

	filtering(cultureLectures)

	writeCultureLectures(cultureLectures)
}

func filtering(cultureLectures []cultureLecture) {
	// 접수상태가 접수마감인 강좌를 제외한다.
	for i, cultureLecture := range cultureLectures {
		if cultureLecture.status == ReceptionStatusClosed {
			cultureLectures[i].scrapeExcluded = true
		}
	}

	// 주말 및 공휴일이 아닌 평일 16시 이전의 강좌를 제외한다.
	weekday := []string{"월요일", "화요일", "수요일", "목요일", "금요일"}
	for i, cultureLecture := range cultureLectures {
		if contains(weekday, cultureLecture.dayOfTheWeek) == true {
			// @@@@@ 공휴일 체크

			h24, err := strconv.Atoi(cultureLecture.startTime[:2])
			checkErr(err)

			if h24 < 16 {
				cultureLectures[i].scrapeExcluded = true
			}
		}
	}

	// 개월수 및 나이에 포함되지 않는 강좌는 제외한다.
	for i, cultureLecture := range cultureLectures {
		alType, from, to := extractAgeOrMonthsRange(&cultureLecture)

		if alType == AgeLimitTypeMonths {
			if childrenMonths < from || childrenMonths > to {
				cultureLectures[i].scrapeExcluded = true
			}
		} else if alType == AgeLimitTypeAge {
			if childrenAge < from || childrenAge > to {
				cultureLectures[i].scrapeExcluded = true
			}
		}
	}

	count := 0
	for _, cultureLecture := range cultureLectures {
		if cultureLecture.scrapeExcluded == true {
			count++
		}
	}

	log.Printf("총 %d건의 강좌중에서 %d건이 필터링되어 제외되었습니다.", len(cultureLectures), count)
}

func extractAgeOrMonthsRange(cultureLecture *cultureLecture) (AgeLimitType, int, int) {
	//title := cultureLecture.title

	//// @@@@@
	//older := map[AgeLimitType][]string{
	//	AgeLimitTypeAge:    {"세이상", "세 이상"},
	//	AgeLimitTypeMonths: {"개월이상", "개월 이상"},
	//}
	//for k, v := range older {
	//	for _, text := range v {
	//		a := regexp.MustCompile("[0-9]{1,2}" + text).FindString(title)
	//		if len(a) > 0 {
	//			age, err := strconv.Atoi(strings.ReplaceAll(a, text, ""))
	//			checkErr(err)
	//			return k, age, math.MaxInt32
	//		}
	//	}
	//}
	//
	//ageRange := map[AgeLimitType][]string{
	//	AgeLimitTypeAge:    {"세"},
	//	AgeLimitTypeMonths: {"개월"},
	//}
	//for key, val := range ageRange {
	//	for _, text := range val {
	//		a := regexp.MustCompile("[0-9]{1,2}~[0-9]{1,2}" + text).FindString(title)
	//		if len(a) > 0 {
	//			age := strings.ReplaceAll(a, text, "")
	//			split := strings.Split(age, "~")
	//			n1, err1 := strconv.Atoi(split[0])
	//			n2, err2 := strconv.Atoi(split[0])
	//			checkErr(err1)
	//			checkErr(err2)
	//			return key, n1, n2
	//		}
	//	}
	//}

	//exclude := map[string][2]int{
	//	"성인": {20, math.MaxInt32},
	//}
	//for key, val := range exclude {
	//	a := regexp.MustCompile(key).FindString(title)
	//	if len(a) > 0 {
	//		return AgeLimitTypeAge, val[0], val[1]
	//	}
	//}

	return AgeLimitTypeUnknwon, 0, math.MaxInt32
}

func writeCultureLectures(cultureLectures []cultureLecture) {
	log.Println("수집된 문화센터 강좌 자료를 파일로 저장합니다.")

	now := time.Now()
	fname := fmt.Sprintf("cultureLecture-%d%02d%02d%02d%02d%02d.csv", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())

	f, err := os.Create(fname)
	checkErr(err)

	defer f.Close()

	// 파일 첫 부분에 UTF-8 BOM을 추가한다.
	_, err = f.WriteString("\xEF\xBB\xBF")
	checkErr(err)

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{"점포", "강좌그룹", "강좌명", "강사명", "개강일", "시작시간", "종료시간", "요일", "수강료", "강좌횟수", "접수상태", "상세페이지"}
	checkErr(w.Write(headers))

	count := 0
	for _, cultureLecture := range cultureLectures {
		if cultureLecture.scrapeExcluded == true {
			continue
		}

		r := []string{
			cultureLecture.storeName,
			cultureLecture.group,
			cultureLecture.title,
			cultureLecture.teacher,
			cultureLecture.startDate,
			cultureLecture.startTime,
			cultureLecture.endTime,
			cultureLecture.dayOfTheWeek,
			cultureLecture.price,
			cultureLecture.count,
			ReceptionStatusString[cultureLecture.status],
			cultureLecture.detailPageUrl,
		}
		checkErr(w.Write(r))
		count++
	}

	log.Printf("수집된 문화센터 강좌 자료(%d건)를 파일(%s)로 저장하였습니다.", count, fname)
}
