package scrape

import (
	"encoding/csv"
	"fmt"
	"github.com/darkkaiser/culturelecture-scrape/scrape/lectures"
	"github.com/darkkaiser/culturelecture-scrape/scrape/lectures/culture"
	"github.com/darkkaiser/culturelecture-scrape/utils"
	"log"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 연령제한타입
type AgeLimitType int

// 지원가능한 연령제한타입 값
const (
	AgeLimitUnknwon AgeLimitType = iota // 알수없음
	AgeLimitAge                         // 나이
	AgeLimitMonths                      // 개월수
)

type AgeLimitRange struct {
	alType AgeLimitType
	from   int
	to     int
}

type scrape struct {
	lectures []lectures.Lecture
}

func New() *scrape {
	return &scrape{}
}

type Scraper interface {
	ScrapeCultureLectures(mainC chan<- []lectures.Lecture)
}

func (s *scrape) Scrape(searchYear string, searchSeason string) {
	log.Println("문화센터 강좌 수집을 시작합니다.")

	searchYear = utils.CleanString(searchYear)
	searchSeason = utils.CleanString(searchSeason)

	if searchYear == "" || searchSeason == "" {
		log.Fatalf("검색년도 및 검색시즌은 빈 문자열을 허용하지 않습니다(검색년도:%s, 검색시즌:%s)", searchYear, searchSeason)
	}

	// 검색시즌코드(봄:1, 여름:2, 가을:3, 겨울:4)
	var searchSeasonCode string
	switch searchSeason {
	case "봄":
		searchSeasonCode = "1"
	case "여름":
		searchSeasonCode = "2"
	case "가을":
		searchSeasonCode = "3"
	case "겨울":
		searchSeasonCode = "4"
	default:
		log.Fatalf("입력된 검색시즌이 올바르지 않습니다(검색시즌:%s)", searchSeason)
	}

	scrapers := []Scraper{
		culture.NewHomeplus(),
		culture.NewLottemart(searchYear, searchSeasonCode),
		culture.NewEmart(searchYear),
	}

	c := make(chan []lectures.Lecture, len(scrapers))
	for _, scraper := range scrapers {
		go scraper.ScrapeCultureLectures(c)
	}

	s.lectures = nil
	for i := 0; i < len(scrapers); i++ {
		scrapedCultureLectures := <-c
		s.lectures = append(s.lectures, scrapedCultureLectures...)
	}

	log.Printf("문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", len(s.lectures))
}

func (s *scrape) Filter(cultureLecturerMonths int, cultureLecturerAge int, holidays []string) {
	// 접수상태가 접수마감인 강좌를 제외한다.
	for i, lecture := range s.lectures {
		if lecture.Status == lectures.ReceptionStatusClosed {
			s.lectures[i].ScrapeExcluded = true
		}
	}

	// 주말 및 공휴일이 아닌 평일 16시 이전의 강좌를 제외한다.
	weekdays := []string{"월요일", "화요일", "수요일", "목요일", "금요일"}
	for i, lecture := range s.lectures {
		if utils.Contains(weekdays, lecture.DayOfTheWeek) == true && utils.Contains(holidays, lecture.StartDate) == false {
			h24, err := strconv.Atoi(lecture.StartTime[:2])
			utils.CheckErr(err)

			if h24 < 16 {
				s.lectures[i].ScrapeExcluded = true
			}
		}
	}

	// 강좌명에 특정 문자열이 포함되어 있는 경우 수집에서 제외한다.
	for i, lecture := range s.lectures {
		for _, v := range []string{"키즈발레", "영어발레", "엔젤발레", "엔젤 발레", "체형교정발레", "체형교정 발레", "YSM발레", "YSM 발레", "쁘띠발레", "발레리나", "앨리스 스토리텔링 발레", "트윈클 동화발레", "밸리댄스", "[광주국제영어마을"} {
			if strings.Contains(lecture.Title, v) == true {
				s.lectures[i].ScrapeExcluded = true
				break
			}
		}
	}

	// 개월수 및 나이에 포함되지 않는 강좌는 제외한다.
	for i, lecture := range s.lectures {
		alType, from, to := s.extractMonthsOrAgeRange(&lecture)

		if alType == AgeLimitMonths {
			if cultureLecturerMonths < from || cultureLecturerMonths > to {
				s.lectures[i].ScrapeExcluded = true
			}
		} else if alType == AgeLimitAge {
			if cultureLecturerAge < from || cultureLecturerAge > to {
				s.lectures[i].ScrapeExcluded = true
			}
		}
	}

	excludedLectureCount := 0
	for _, lecture := range s.lectures {
		if lecture.ScrapeExcluded == true {
			excludedLectureCount++
		}
	}

	log.Printf("총 %d건의 문화센터 강좌중에서 %d건이 필터링되어 제외되었습니다.", len(s.lectures), excludedLectureCount)
}

func (s *scrape) extractMonthsOrAgeRange(lecture *lectures.Lecture) (AgeLimitType, int, int) {
	alTypesMap := map[AgeLimitType]string{
		AgeLimitAge:    "세",
		AgeLimitMonths: "개월",
	}
	for alType, alTypeString := range alTypesMap {
		// n세이상, n세 이상, n세~성인, n세~ 성인, n세~누구나, n세~ 누구나
		// n개월이상, n개월 이상, n개월~성인, n개월~ 성인, n개월~누구나, n개월~ 누구나
		for _, v := range []string{alTypeString + "이상", alTypeString + " 이상", alTypeString + "~성인", alTypeString + "~ 성인", alTypeString + "~누구나", alTypeString + "~ 누구나"} {
			fs := regexp.MustCompile("[0-9]{1,2}" + v).FindString(lecture.Title)
			if len(fs) > 0 {
				from, err := strconv.Atoi(strings.ReplaceAll(fs, v, ""))
				utils.CheckErr(err)

				return alType, from, math.MaxInt32
			}
		}

		// a~b세, a-b세, a세~b세, a세-b세
		// a~b개월, a-b개월, a개월~b개월, a개월-b개월
		fs := regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}[%s]?[~-]{1}[0-9]{1,2}%s", alTypeString, alTypeString)).FindString(lecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			utils.CheckErr(err)
			to, err := strconv.Atoi(split[1])
			utils.CheckErr(err)

			return alType, from, to
		}

		// n세~초등, n세-초등
		// n개월~초등, n개월-초등
		fs = regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}%s[~-]{1}초등", alTypeString)).FindString(lecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			utils.CheckErr(err)

			to := 13
			if alType == AgeLimitMonths {
				to *= 12
			}

			return alType, from, to
		}

		// n세~초n, n세-초n
		// n개월~초n, n개월-초n
		fs = regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}%s[~-]{1}초[1-6]{1}", alTypeString)).FindString(lecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			utils.CheckErr(err)

			to, err := strconv.Atoi(strings.ReplaceAll(split[1], "초", ""))
			utils.CheckErr(err)

			to += 7
			if alType == AgeLimitMonths {
				to *= 12
			}

			return alType, from, to
		}

		// (n세)
		// (n개월)
		fs = regexp.MustCompile(fmt.Sprintf("\\([0-9]{1,2}%s\\)", alTypeString)).FindString(lecture.Title)
		if len(fs) > 0 {
			no, err := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "(", ""), ")", ""))
			utils.CheckErr(err)

			return alType, no, no
		}
	}

	// 초a~초b, 초a-초b
	fs := regexp.MustCompile("초[1-6][~-]초[1-6]").FindString(lecture.Title)
	if len(fs) > 0 {
		split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, "초", ""), "-", "~"), "~")

		from, err := strconv.Atoi(split[0])
		utils.CheckErr(err)
		to, err := strconv.Atoi(split[1])
		utils.CheckErr(err)

		return AgeLimitAge, from + 7, to + 7
	}

	now := time.Now()

	// nn~nn년생
	fs = regexp.MustCompile("[0-9]{2}[~][0-9]{2}년생").FindString(lecture.Title)
	if len(fs) > 0 {
		split := strings.Split(strings.ReplaceAll(fs, "년생", ""), "~")

		from, err := strconv.Atoi(split[0])
		utils.CheckErr(err)
		to, err := strconv.Atoi(split[1])
		utils.CheckErr(err)

		return AgeLimitAge, now.Year() - (2000 + from) + 1, now.Year() - (2000 + to) + 1
	}

	// nnnn년~nnnn년생
	fs = regexp.MustCompile("[0-9]{4}년[~][0-9]{4}년생").FindString(lecture.Title)
	if len(fs) > 0 {
		split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, "년생", ""), "년", ""), "~")

		from, err := strconv.Atoi(split[0])
		utils.CheckErr(err)
		to, err := strconv.Atoi(split[1])
		utils.CheckErr(err)

		return AgeLimitAge, now.Year() - from + 1, now.Year() - to + 1
	}

	// 강좌명에 특정 문자열이 포함되어 있는 경우, 연령제한타입 및 나이 범위를 임의적으로 반환한다.
	specificTextMap := map[string]AgeLimitRange{
		"(초등)": {
			alType: AgeLimitAge,
			from:   8,
			to:     13,
		},
		"(초등반)": {
			alType: AgeLimitAge,
			from:   8,
			to:     13,
		},
		"(모든연령": {
			alType: AgeLimitAge,
			from:   0,
			to:     math.MaxInt32,
		},
		"(모든 연령)": {
			alType: AgeLimitAge,
			from:   0,
			to:     math.MaxInt32,
		},
		"(초등~성인)": {
			alType: AgeLimitAge,
			from:   8,
			to:     math.MaxInt32,
		},
	}
	for k, v := range specificTextMap {
		if strings.Contains(lecture.Title, k) == true {
			return v.alType, v.from, v.to
		}
	}

	if lecture.ScrapeExcluded == false {
		log.Printf(" >> 수집된 강좌의 연령(나이, 개월수) 추출 실패, 필터링 대상에서 제외됩니다.(%s : %s)", lecture.StoreName, lecture.Title)
	}

	return AgeLimitUnknwon, 0, math.MaxInt32
}

func (s *scrape) ExportCSV(fileName string) {
	/**
	 * CSV 파일저장
	 */
	log.Println("수집된 문화센터 강좌 자료를 CSV 파일로 저장합니다.")

	f, err := os.Create(fileName)
	utils.CheckErr(err)

	//goland:noinspection GoUnhandledErrorResult
	defer f.Close()

	// 파일 첫 부분에 UTF-8 BOM을 추가한다.
	_, err = f.WriteString("\xEF\xBB\xBF")
	utils.CheckErr(err)

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{"점포", "강좌그룹", "강좌명", "강사명", "개강일", "시작시간", "종료시간", "요일", "수강료", "강좌횟수", "접수상태", "상세페이지"}
	utils.CheckErr(w.Write(headers))

	count := 0
	for _, lecture := range s.lectures {
		if lecture.ScrapeExcluded == true {
			continue
		}

		r := []string{
			lecture.StoreName,
			lecture.Group,
			lecture.Title,
			lecture.Teacher,
			lecture.StartDate,
			lecture.StartTime,
			lecture.EndTime,
			lecture.DayOfTheWeek,
			lecture.Price,
			lecture.Count,
			lectures.ReceptionStatusString[lecture.Status],
			lecture.DetailPageUrl,
		}
		utils.CheckErr(w.Write(r))
		count++
	}

	log.Printf("수집된 문화센터 강좌 자료(%d건)를 CSV 파일(%s)로 저장하였습니다.", count, fileName)
}
