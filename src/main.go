package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

/********************************************************************************/
/* 강좌 수집 작업시에 변경되는 값                                                    */
/****************************************************************************** */
const (
	// 검색년도
	SearchYear = "2020"

	// 검색시즌(봄:1, 여름:2, 가을:3, 겨울:4)
	SearchSeasonCode = "2"

	// 강좌를 수강하는 아이 개월수
	childrenMonths = 51

	// 강좌를 수강하는 아이 나이
	childrenAge = 5
)

// 2020년도 공휴일
var holidays = []string{
	"2020-01-01",
	"2020-01-24", "2020-01-25", "2020-01-26", "2020-01-27",
	"2020-03-01",
	"2020-04-30",
	"2020-05-05",
	"2020-06-06",
	"2020-08-15",
	"2020-09-30", "2020-10-01", "2020-10-02",
	"2020-10-03",
	"2020-10-09",
	"2020-12-25",
}

/********************************************************************************/

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
type AgeLimitType int

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
	weekdays := []string{"월요일", "화요일", "수요일", "목요일", "금요일"}
	for i, cultureLecture := range cultureLectures {
		if contains(weekdays, cultureLecture.dayOfTheWeek) == true && contains(holidays, cultureLecture.startDate) == false {
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
	// 강좌명에 특정 문자열이 포함되어 있는 경우 수집에서 제외한다.
	for _, v := range []string{"키즈발레", "발레리나", "앨리스 스토리텔링 발레", "트윈클 동화발레", "밸리댄스", "[광주국제영어마을"} {
		if strings.Contains(cultureLecture.title, v) == true {
			return AgeLimitTypeAge, 99, 99
		}
	}

	alTypesMap := map[AgeLimitType]string{
		AgeLimitTypeAge:    "세",
		AgeLimitTypeMonths: "개월",
	}
	for alType, alTypeString := range alTypesMap {
		// n세이상, n세 이상, n세~성인, n세~ 성인
		// n개월이상, n개월 이상, n개월~성인, n개월~ 성인
		for _, v := range []string{alTypeString + "이상", alTypeString + " 이상", alTypeString + "~성인", alTypeString + "~ 성인"} {
			fs := regexp.MustCompile("[0-9]{1,2}" + v).FindString(cultureLecture.title)
			if len(fs) > 0 {
				from, err := strconv.Atoi(strings.ReplaceAll(fs, v, ""))
				checkErr(err)

				return alType, from, math.MaxInt32
			}
		}

		// a~b세, a-b세, a세~b세, a세-b세
		// a~b개월, a-b개월, a개월~b개월, a개월-b개월
		fs := regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}[%s]?[~-]{1}[0-9]{1,2}%s", alTypeString, alTypeString)).FindString(cultureLecture.title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			checkErr(err)
			to, err := strconv.Atoi(split[1])
			checkErr(err)

			return alType, from, to
		}

		// n세~초등, n세-초등
		// n개월~초등, n개월-초등
		fs = regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}%s[~-]{1}초등", alTypeString)).FindString(cultureLecture.title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			checkErr(err)

			to := 13
			if alType == AgeLimitTypeMonths {
				to *= 12
			}

			return alType, from, to
		}

		// (n세)
		// (n개월)
		fs = regexp.MustCompile(fmt.Sprintf("\\([0-9]{1,2}%s\\)", alTypeString)).FindString(cultureLecture.title)
		if len(fs) > 0 {
			no, err := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "(", ""), ")", ""))
			checkErr(err)

			return alType, no, no
		}
	}

	// 초a~초b, 초a-초b
	fs := regexp.MustCompile("초[1-6][~-]초[1-6]").FindString(cultureLecture.title)
	if len(fs) > 0 {
		split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, "초", ""), "-", "~"), "~")

		from, err := strconv.Atoi(split[0])
		checkErr(err)
		to, err := strconv.Atoi(split[1])
		checkErr(err)

		return AgeLimitTypeAge, from + 7, to + 7
	}

	// 강좌명에 특정 문자열이 포함되어 있는 경우 연령제한타입 및 나이 범위를 임의적으로 반환한다.
	specificTextMap := map[string][3]int{
		"(초등)":  {AgeLimitTypeAge, 8, 13},
		"(초등반)": {AgeLimitTypeAge, 8, 13},
	}
	for k, v := range specificTextMap {
		if strings.Contains(cultureLecture.title, k) == true {
			return AgeLimitType(v[0]), v[1], v[2]
		}
	}

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
