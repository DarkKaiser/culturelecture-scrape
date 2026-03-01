package scraper

import (
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper/provider"
	"github.com/darkkaiser/notify-server/pkg/strutil"
)

// AgeLimitType 연령제한타입
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

type Scrape struct {
	lectures []domain.Lecture
}

func New() *Scrape {
	return &Scrape{}
}

type Scraper interface {
	ScrapeCultureLectures(mainC chan<- []domain.Lecture) error
}

func (s *Scrape) Scrape(searchYear string, searchSeason string) error {
	searchYear = strutil.NormalizeSpace(searchYear)
	searchSeason = strutil.NormalizeSpace(searchSeason)

	log.Printf("문화센터 강좌 수집을 시작합니다.(검색조건:%s년도 %s)", searchYear, searchSeason)

	if searchYear == "" || searchSeason == "" {
		return fmt.Errorf("검색년도 및 검색시즌은 빈 문자열을 허용하지 않습니다(검색년도:%s, 검색시즌:%s)", searchYear, searchSeason)
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
		return fmt.Errorf("입력된 검색시즌이 올바르지 않습니다(검색시즌:%s)", searchSeason)
	}

	scrapers := []Scraper{
		provider.NewHomeplus(),
		provider.NewLottemart(searchYear, searchSeasonCode),
		provider.NewEmart(searchYear),
	}

	c := make(chan []domain.Lecture, len(scrapers))
	for _, scraper := range scrapers {
		go func(sc Scraper) {
			err := sc.ScrapeCultureLectures(c)
			if err != nil {
				log.Fatalf("스크래핑 작업 중 오류 발생하여 즉시 종료합니다: %v", err)
			}
		}(scraper)
	}

	s.lectures = nil
	for i := 0; i < len(scrapers); i++ {
		scrapedCultureLectures := <-c
		s.lectures = append(s.lectures, scrapedCultureLectures...)
	}

	log.Printf("문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", len(s.lectures))
	return nil
}

func (s *Scrape) Filter(cultureLecturerMonths int, cultureLecturerAge int, holidays []string) {
	// 접수상태가 접수마감인 강좌를 제외한다.
	for i, lecture := range s.lectures {
		if lecture.Status == domain.ReceptionStatusClosed {
			s.lectures[i].ScrapeExcluded = true
		}
	}

	// 주말 및 공휴일이 아닌 평일 16시 이전의 강좌를 제외한다.
	weekdays := []string{"월요일", "화요일", "수요일", "목요일", "금요일"}
	for i, lecture := range s.lectures {
		if slices.Contains(weekdays, lecture.DayOfTheWeek) == true && slices.Contains(holidays, lecture.StartDate) == false {
			h24, err := strconv.Atoi(lecture.StartTime[:2])
			if err != nil {
				log.Printf("강좌 시작시간 파싱 오류 (강좌명: %s, 시간: %s): %v", lecture.Title, lecture.StartTime, err)
				continue
			}

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
		alType, from, to, err := s.extractMonthsOrAgeRange(&lecture)
		if err != nil {
			log.Printf("연령 범위 추출 오류 (강좌명: %s): %v", lecture.Title, err)
			continue
		}

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

func (s *Scrape) extractMonthsOrAgeRange(lecture *domain.Lecture) (AgeLimitType, int, int, error) {
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
				if err != nil {
					return AgeLimitUnknwon, 0, 0, err
				}

				return alType, from, math.MaxInt32, nil
			}
		}

		// a~b세, a-b세, a세~b세, a세-b세
		// a~b개월, a-b개월, a개월~b개월, a개월-b개월
		fs := regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}[%s]?[~-]{1}[0-9]{1,2}%s", alTypeString, alTypeString)).FindString(lecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "-", "~"), "~")

			value1, err := strconv.Atoi(split[0])
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}
			value2, err := strconv.Atoi(split[1])
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}

			if value1 < value2 {
				return alType, value1, value2, nil
			} else {
				return alType, value2, value1, nil
			}
		}

		// n세~초등, n세-초등
		// n개월~초등, n개월-초등
		fs = regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}%s[~-]{1}초등", alTypeString)).FindString(lecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}

			to := 13
			if alType == AgeLimitMonths {
				to *= 12
			}

			return alType, from, to, nil
		}

		// n세~초n, n세-초n
		// n개월~초n, n개월-초n
		fs = regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}%s[~-]{1}초[1-6]{1}", alTypeString)).FindString(lecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}

			to, err := strconv.Atoi(strings.ReplaceAll(split[1], "초", ""))
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}

			to += 7
			if alType == AgeLimitMonths {
				to *= 12
			}

			return alType, from, to, nil
		}

		// (n세)
		// (n개월)
		fs = regexp.MustCompile(fmt.Sprintf("\\([0-9]{1,2}%s\\)", alTypeString)).FindString(lecture.Title)
		if len(fs) > 0 {
			no, err := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(fs, alTypeString, ""), "(", ""), ")", ""))
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}

			return alType, no, no, nil
		}
	}

	// 초a~초b, 초a-초b
	fs := regexp.MustCompile("초[1-6][~-]초[1-6]").FindString(lecture.Title)
	if len(fs) > 0 {
		split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, "초", ""), "-", "~"), "~")

		value1, err := strconv.Atoi(split[0])
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}
		value2, err := strconv.Atoi(split[1])
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}

		if value1 < value2 {
			return AgeLimitAge, value1 + 7, value2 + 7, nil
		} else {
			return AgeLimitAge, value2 + 7, value1 + 7, nil
		}
	}

	now := time.Now()

	// nnnn~nnnn년생, nnnn년~nnnn년생
	fs = regexp.MustCompile("[0-9]{4}년?~[0-9]{4}년생").FindString(lecture.Title)
	if len(fs) > 0 {
		split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, "년생", ""), "년", ""), "~")

		value1, err := strconv.Atoi(split[0])
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}
		value2, err := strconv.Atoi(split[1])
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}

		value1 = now.Year() - value1 + 1
		value2 = now.Year() - value2 + 1

		if value1 < value2 {
			return AgeLimitAge, value1, value2, nil
		} else {
			return AgeLimitAge, value2, value1, nil
		}
	}

	// nnnn~nn년생, nnnn년~nn년생
	fs = regexp.MustCompile("[0-9]{4}년?~[0-9]{2}년생").FindString(lecture.Title)
	if len(fs) > 0 {
		split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, "년생", ""), "년", ""), "~")

		value1, err := strconv.Atoi(split[0])
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}
		value2, err := strconv.Atoi(split[1])
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}

		value1 = now.Year() - value1 + 1
		value2 = now.Year() - (2000 + value2) + 1

		if value1 < value2 {
			return AgeLimitAge, value1, value2, nil
		} else {
			return AgeLimitAge, value2, value1, nil
		}
	}

	// nn~nn년, nn~nn년생
	fs = regexp.MustCompile("[0-9]{2}~[0-9]{2}년생?").FindString(lecture.Title)
	if len(fs) > 0 {
		split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, "년생", ""), "년", ""), "~")

		value1, err := strconv.Atoi(split[0])
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}
		value2, err := strconv.Atoi(split[1])
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}

		value1 = now.Year() - (2000 + value1) + 1
		value2 = now.Year() - (2000 + value2) + 1

		if value1 < value2 {
			return AgeLimitAge, value1, value2, nil
		} else {
			return AgeLimitAge, value2, value1, nil
		}
	}

	// nnnn년생 이상
	fs = regexp.MustCompile("[0-9]{4}년생 이상").FindString(lecture.Title)
	if len(fs) > 0 {
		from, err := strconv.Atoi(strings.ReplaceAll(fs, "년생 이상", ""))
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}

		return AgeLimitAge, now.Year() - from + 1, math.MaxInt32, nil
	}

	// nn년생 이상
	fs = regexp.MustCompile("[0-9]{2}년생 이상").FindString(lecture.Title)
	if len(fs) > 0 {
		from, err := strconv.Atoi(strings.ReplaceAll(fs, "년생 이상", ""))
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}

		return AgeLimitAge, now.Year() - (2000 + from) + 1, math.MaxInt32, nil
	}

	// 성인~nnnn년
	// 성인~nnnn년생
	fs = regexp.MustCompile("성인~[0-9]{4}년생?").FindString(lecture.Title)
	if len(fs) > 0 {
		split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, "년생", ""), "년", ""), "~")

		from, err := strconv.Atoi(split[1])
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}

		return AgeLimitAge, now.Year() - from + 1, math.MaxInt32, nil
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
		"(성인~중학생이상)": {
			alType: AgeLimitAge,
			from:   14,
			to:     math.MaxInt32,
		},
		"(성인)": {
			alType: AgeLimitAge,
			from:   20,
			to:     math.MaxInt32,
		},
	}
	for k, v := range specificTextMap {
		if strings.Contains(lecture.Title, k) == true {
			return v.alType, v.from, v.to, nil
		}
	}

	if lecture.ScrapeExcluded == false {
		log.Printf(" >> 수집된 강좌의 연령(나이, 개월수) 추출 실패, 필터링 대상에서 제외됩니다.(%s : %s)", lecture.StoreName, lecture.Title)
	}

	return AgeLimitUnknwon, 0, math.MaxInt32, nil
}

func (s *Scrape) ExportCSV(fileName string) error {
	/**
	 * CSV 파일저장
	 */
	log.Println("수집된 문화센터 강좌 자료를 CSV 파일로 저장합니다.")

	f, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("CSV 파일 생성 실패: %v", err)
	}

	//goland:noinspection GoUnhandledErrorResult
	defer f.Close()

	// 파일 첫 부분에 UTF-8 BOM을 추가한다.
	_, err = f.WriteString("\xEF\xBB\xBF")
	if err != nil {
		return fmt.Errorf("UTF-8 BOM 쓰기 실패: %v", err)
	}

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{"점포", "강좌그룹", "강좌명", "강사명", "개강일", "시작시간", "종료시간", "요일", "수강료", "강좌횟수", "접수상태", "상세페이지"}
	if err := w.Write(headers); err != nil {
		return fmt.Errorf("CSV 헤더 쓰기 실패: %v", err)
	}

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
			domain.ReceptionStatusString[lecture.Status],
			lecture.DetailPageUrl,
		}
		if err := w.Write(r); err != nil {
			return fmt.Errorf("CSV 레코드 쓰기 실패: %v", err)
		}
		count++
	}

	log.Printf("수집된 문화센터 강좌 자료(%d건)를 CSV 파일(%s)로 저장하였습니다.", count, fileName)

	return nil
}
