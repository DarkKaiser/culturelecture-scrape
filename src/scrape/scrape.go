package scrape

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"helpers"
	"log"
	"math"
	"os"
	"regexp"
	"scrape/culture"
	"strconv"
	"strings"
	"time"
)

/********************************************************************************/
/* 강좌 수집 작업시에 변경되는 값 BEGIN                                              */
/****************************************************************************** */

// 검색년도@@@@@  함수 인자로 받기
var SearchYear = "2020"

// 검색시즌(봄:1, 여름:2, 가을:3, 겨울:4)@@@@@  함수 인자로 받기
var SearchSeasonCode = "2"

// 강좌를 수강하는 아이 개월수@@@@@  함수 인자로 받기
var ChildrenMonths = 51

// 강좌를 수강하는 아이 나이@@@@@  함수 인자로 받기
var ChildrenAge = 5

// 2020년도 공휴일
var Holidays = []string{
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

func init() {
	// @@@@@ 삭제대상
	culture.SearchYear = SearchYear
	culture.SearchSeasonCode = SearchSeasonCode
}

/********************************************************************************/
/* 강좌 수집 작업시에 변경되는 값 END                                                */
/****************************************************************************** */

// 연령제한타입
type AgeLimitType int

// 지원가능한 연령제한타입 값
const (
	AgeLimitTypeUnknwon = iota // 알수없음
	AgeLimitTypeAge            // 나이
	AgeLimitTypeMonths         // 개월수
)

func Scrape() {
	/**
	 * 문화센터 강좌 수집
	 */
	log.Println("문화센터 강좌 수집을 시작합니다.")

	c := make(chan []culture.Lecture, 3)

	var goroutineCount = 0
	go culture.ScrapeEmartCultureLecture(c)
	goroutineCount++
	go culture.ScrapeLottemartCultureLecture(c)
	goroutineCount++
	go culture.ScrapeHomeplusCultureLecture(c)
	goroutineCount++

	var cultureLectures []culture.Lecture
	for i := 0; i < goroutineCount; i++ {
		cultureLecturesScraped := <-c
		cultureLectures = append(cultureLectures, cultureLecturesScraped...)
	}

	log.Printf("문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", len(cultureLectures))

	/**
	 * 수집된 문화센터 강좌 필터링
	 */
	filtering(cultureLectures)

	/**
	 * 최근에 수집된 문화센터 강좌 자료 로드
	 */
	const latestScrapedLecturesFileName = "culturelecture-scrape-latest.csv"

	var latestScrapedCultureLectures [][]string
	f, _ := os.Open(latestScrapedLecturesFileName)
	if f == nil {
		log.Println(fmt.Sprintf("최근에 수집된 문화센터 강좌 자료(%s)가 존재하지 않습니다. 새로 수집된 강좌는 이전에 수집된 강좌와의 변경사항을 추적할 수 없습니다.", latestScrapedLecturesFileName))
	} else {
		defer f.Close()

		r := csv.NewReader(bufio.NewReader(f))
		latestScrapedCultureLectures, _ = r.ReadAll()
		if latestScrapedCultureLectures == nil {
			log.Println(fmt.Sprintf("최근에 수집된 문화센터 강좌 자료(%s)를 로드할 수 없습니다. 새로 수집된 강좌는 이전에 수집된 강좌와의 변경사항을 추적할 수 없습니다.", latestScrapedLecturesFileName))
		} else {
			log.Println(fmt.Sprintf("최근에 수집된 문화센터 강좌 자료(%s)를 로드하였습니다.", latestScrapedLecturesFileName))
		}
	}

	/**
	 * 문화센터 강좌 파일로 저장
	 */
	writeCultureLectures(cultureLectures, latestScrapedCultureLectures)
}

func filtering(cultureLectures []culture.Lecture) {
	// 접수상태가 접수마감인 강좌를 제외한다.
	for i, cultureLecture := range cultureLectures {
		if cultureLecture.Status == culture.ReceptionStatusClosed {
			cultureLectures[i].ScrapeExcluded = true
		}
	}

	// 주말 및 공휴일이 아닌 평일 16시 이전의 강좌를 제외한다.
	weekdays := []string{"월요일", "화요일", "수요일", "목요일", "금요일"}
	for i, cultureLecture := range cultureLectures {
		if helpers.Contains(weekdays, cultureLecture.DayOfTheWeek) == true && helpers.Contains(Holidays, cultureLecture.StartDate) == false {
			h24, err := strconv.Atoi(cultureLecture.StartTime[:2])
			helpers.CheckErr(err)

			if h24 < 16 {
				cultureLectures[i].ScrapeExcluded = true
			}
		}
	}

	// 개월수 및 나이에 포함되지 않는 강좌는 제외한다.
	for i, cultureLecture := range cultureLectures {
		alType, from, to := extractAgeOrMonthsRange(&cultureLecture)

		if alType == AgeLimitTypeMonths {
			if ChildrenMonths < from || ChildrenMonths > to {
				cultureLectures[i].ScrapeExcluded = true
			}
		} else if alType == AgeLimitTypeAge {
			if ChildrenAge < from || ChildrenAge > to {
				cultureLectures[i].ScrapeExcluded = true
			}
		}
	}

	count := 0
	for _, cultureLecture := range cultureLectures {
		if cultureLecture.ScrapeExcluded == true {
			count++
		}
	}

	log.Printf("총 %d건의 문화센터 강좌중에서 %d건이 필터링되어 제외되었습니다.", len(cultureLectures), count)
}

func extractAgeOrMonthsRange(cultureLecture *culture.Lecture) (AgeLimitType, int, int) {
	// 강좌명에 특정 문자열이 포함되어 있는 경우 수집에서 제외한다.
	for _, v := range []string{"키즈발레", "발레리나", "앨리스 스토리텔링 발레", "트윈클 동화발레", "밸리댄스", "[광주국제영어마을"} {
		if strings.Contains(cultureLecture.Title, v) == true {
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
			fs := regexp.MustCompile("[0-9]{1,2}" + v).FindString(cultureLecture.Title)
			if len(fs) > 0 {
				from, err := strconv.Atoi(strings.ReplaceAll(fs, v, ""))
				helpers.CheckErr(err)

				return alType, from, math.MaxInt32
			}
		}

		// a~b세, a-b세, a세~b세, a세-b세
		// a~b개월, a-b개월, a개월~b개월, a개월-b개월
		fs := regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}[%s]?[~-]{1}[0-9]{1,2}%s", alTypeString, alTypeString)).FindString(cultureLecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			helpers.CheckErr(err)
			to, err := strconv.Atoi(split[1])
			helpers.CheckErr(err)

			return alType, from, to
		}

		// n세~초등, n세-초등
		// n개월~초등, n개월-초등
		fs = regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}%s[~-]{1}초등", alTypeString)).FindString(cultureLecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			helpers.CheckErr(err)

			to := 13
			if alType == AgeLimitTypeMonths {
				to *= 12
			}

			return alType, from, to
		}

		// (n세)
		// (n개월)
		fs = regexp.MustCompile(fmt.Sprintf("\\([0-9]{1,2}%s\\)", alTypeString)).FindString(cultureLecture.Title)
		if len(fs) > 0 {
			no, err := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "(", ""), ")", ""))
			helpers.CheckErr(err)

			return alType, no, no
		}
	}

	// 초a~초b, 초a-초b
	fs := regexp.MustCompile("초[1-6][~-]초[1-6]").FindString(cultureLecture.Title)
	if len(fs) > 0 {
		split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, "초", ""), "-", "~"), "~")

		from, err := strconv.Atoi(split[0])
		helpers.CheckErr(err)
		to, err := strconv.Atoi(split[1])
		helpers.CheckErr(err)

		return AgeLimitTypeAge, from + 7, to + 7
	}

	// 강좌명에 특정 문자열이 포함되어 있는 경우, 연령제한타입 및 나이 범위를 임의적으로 반환한다.
	specificTextMap := map[string][3]int{
		"(초등)":  {AgeLimitTypeAge, 8, 13},
		"(초등반)": {AgeLimitTypeAge, 8, 13},
	}
	for k, v := range specificTextMap {
		if strings.Contains(cultureLecture.Title, k) == true {
			return AgeLimitType(v[0]), v[1], v[2]
		}
	}

	return AgeLimitTypeUnknwon, 0, math.MaxInt32
}

func writeCultureLectures(cultureLectures []culture.Lecture, latestScrapedCultureLectures [][]string) {
	log.Println("수집된 문화센터 강좌 자료를 파일로 저장합니다.")

	now := time.Now()
	fname := fmt.Sprintf("culturelecture-scrape-%d%02d%02d%02d%02d%02d.csv", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())

	f, err := os.Create(fname)
	helpers.CheckErr(err)

	defer f.Close()

	// 파일 첫 부분에 UTF-8 BOM을 추가한다.
	_, err = f.WriteString("\xEF\xBB\xBF")
	helpers.CheckErr(err)

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{"점포", "강좌그룹", "강좌명", "강사명", "개강일", "시작시간", "종료시간", "요일", "수강료", "강좌횟수", "접수상태", "상세페이지", "최근에 수집된 강좌와 비교"}
	helpers.CheckErr(w.Write(headers))

	count := 0
	for _, cultureLecture := range cultureLectures {
		if cultureLecture.ScrapeExcluded == true {
			continue
		}

		r := []string{
			cultureLecture.StoreName,
			cultureLecture.Group,
			cultureLecture.Title,
			cultureLecture.Teacher,
			cultureLecture.StartDate,
			cultureLecture.StartTime,
			cultureLecture.EndTime,
			cultureLecture.DayOfTheWeek,
			cultureLecture.Price,
			cultureLecture.Count,
			culture.ReceptionStatusString[cultureLecture.Status],
			cultureLecture.DetailPageUrl,
			compareLatestScrapedCultureLecture(&cultureLecture, latestScrapedCultureLectures),
		}
		helpers.CheckErr(w.Write(r))
		count++
	}

	log.Printf("수집된 문화센터 강좌 자료(%d건)를 파일(%s)로 저장하였습니다.", count, fname)
}

func compareLatestScrapedCultureLecture(cultureLecture *culture.Lecture, latestScrapedCultureLectures [][]string) string {
	if latestScrapedCultureLectures == nil || (len(latestScrapedCultureLectures) == 1 && len(latestScrapedCultureLectures[0]) == 1) {
		return "-"
	}

	for _, latestScrapedCultureLecture := range latestScrapedCultureLectures {
		if len(latestScrapedCultureLecture) != 13 {
			continue
		}

		if latestScrapedCultureLecture[0] == cultureLecture.StoreName &&
			latestScrapedCultureLecture[1] == cultureLecture.Group &&
			latestScrapedCultureLecture[2] == cultureLecture.Title &&
			latestScrapedCultureLecture[3] == cultureLecture.Teacher &&
			latestScrapedCultureLecture[4] == cultureLecture.StartDate &&
			latestScrapedCultureLecture[5] == cultureLecture.StartTime &&
			latestScrapedCultureLecture[6] == cultureLecture.EndTime &&
			latestScrapedCultureLecture[8] == cultureLecture.Price &&
			latestScrapedCultureLecture[9] == cultureLecture.Count &&
			latestScrapedCultureLecture[11] == cultureLecture.DetailPageUrl {
			return "변경사항 없음"
		}

		if latestScrapedCultureLecture[11] == cultureLecture.DetailPageUrl {
			return "변경됨"
		}
	}

	return "신규"
}
