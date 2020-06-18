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

	// 개월수@@@@@
	filterMonths = 50

	// 나이@@@@@
	filterAge = 5
	/********************************************************************************/
)

// 접수상태
type ReceptionStatus uint

// 접수상태 문자열
var ReceptionStatusString = []string{"알수없음", "접수가능", "접수마감", "대기신청", "방문상담", "방문선착순", "당일참여"}

// 지원가능한 접수상태 값
const (
	ReceptionStatusUnknown                   = iota //
	ReceptionStatusPossible                         // 접수가능
	ReceptionStatusClosed                           // 접수마감
	ReceptionStatusStnadBy                          // 대기신청
	ReceptionStatusVisitConsultation                // 방문상담
	ReceptionStatusVisitFirstComeFirstServed        // 방문선착순
	ReceptionStatusDayParticipation                 // 당일참여
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

	var goRoutineCount = 0
	go scrapeEmartCultureLecture(c)
	goRoutineCount++
	go scrapeLottemartCultureLecture(c)
	goRoutineCount++
	go scrapeHomeplusCultureLecture(c)
	goRoutineCount++

	var cultureLectures []cultureLecture
	for i := 0; i < goRoutineCount; i++ {
		cultureLecturesScraped := <-c
		cultureLectures = append(cultureLectures, cultureLecturesScraped...)
	}

	log.Println("문화센터 강좌 수집이 완료되었습니다. 총 " + strconv.Itoa(len(cultureLectures)) + "개의 강좌가 수집되었습니다.")

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
			// @@@@@ 공휴일

			h24, err := strconv.Atoi(cultureLecture.startTime[:2])
			checkErr(err)

			if h24 < 16 {
				cultureLectures[i].scrapeExcluded = true
			}
		}
	}

	// @@@@@
	// 개월수 및 나이에 포함되지 않는 강좌는 제외한다.
	for i, cultureLecture := range cultureLectures {
		ageOrMonths, from, to := extractAgeOrMonthsRange(cultureLecture)
		println(ageOrMonths, from, to)

		if ageOrMonths == 1 /* 개월수 */ {
			if from < filterMonths || to > filterMonths {
				cultureLectures[i].scrapeExcluded = true
			}
		} else if ageOrMonths == 2 /* 나이 */ {
			if from < filterAge || to > filterAge {
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

	log.Println("총 " + strconv.Itoa(len(cultureLectures)) + "건의 강좌중에서 " + strconv.Itoa(count) + "건이 필터링되어 제외되었습니다.")
}

func extractAgeOrMonthsRange(cultureLecture cultureLecture) (int, from int, to int) {
	// @@@@@
	return 0, from, to
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

	log.Println("수집된 문화센터 강좌 자료(" + strconv.Itoa(count) + "건)를 파일(" + fName + ")로 저장하였습니다.")
}
