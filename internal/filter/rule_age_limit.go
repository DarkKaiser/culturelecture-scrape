package filter

import (
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// AgeLimitType 연령제한타입
type AgeLimitType int

// 지원가능한 연령제한타입 값
const (
	AgeLimitUnknwon AgeLimitType = iota // 알수없음
	AgeLimitAge                         // 나이
	AgeLimitMonths                      // 개월수
)

type ageLimitRegexInfo struct {
	alType      AgeLimitType
	regex       *regexp.Regexp
	replaceFrom string
}

var (
	regexAgeMonthsOverOrAdult   []ageLimitRegexInfo
	regexAgeMonthsRange         []ageLimitRegexInfo
	regexAgeMonthsToElemSchool  []ageLimitRegexInfo
	regexAgeMonthsToElemGrade   []ageLimitRegexInfo
	regexAgeMonthsInParentheses []ageLimitRegexInfo

	regexElemGradeRange   = regexp.MustCompile(`초[1-6][~-]초[1-6]`)
	regexYearToYearRange  = regexp.MustCompile(`[0-9]{4}년?~[0-9]{4}년생`)
	regexYearTo2DigitYear = regexp.MustCompile(`[0-9]{4}년?~[0-9]{2}년생`)
	regex2DigitYearRange  = regexp.MustCompile(`[0-9]{2}~[0-9]{2}년생?`)
	regexYearOver         = regexp.MustCompile(`[0-9]{4}년생 이상`)
	regex2DigitYearOver   = regexp.MustCompile(`[0-9]{2}년생 이상`)
	regexAdultToYear      = regexp.MustCompile(`성인~[0-9]{4}년생?`)
)

func init() {
	alTypesMap := map[AgeLimitType]string{
		AgeLimitAge:    "세",
		AgeLimitMonths: "개월",
	}

	for alType, alTypeString := range alTypesMap {
		for _, v := range []string{alTypeString + "이상", alTypeString + " 이상", alTypeString + "~성인", alTypeString + "~ 성인", alTypeString + "~누구나", alTypeString + "~ 누구나"} {
			regexAgeMonthsOverOrAdult = append(regexAgeMonthsOverOrAdult, ageLimitRegexInfo{
				alType:      alType,
				regex:       regexp.MustCompile("[0-9]{1,2}" + v),
				replaceFrom: v,
			})
		}

		regexAgeMonthsRange = append(regexAgeMonthsRange, ageLimitRegexInfo{
			alType:      alType,
			regex:       regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}[%s]?[~-]{1}[0-9]{1,2}%s", alTypeString, alTypeString)),
			replaceFrom: alTypeString,
		})

		regexAgeMonthsToElemSchool = append(regexAgeMonthsToElemSchool, ageLimitRegexInfo{
			alType:      alType,
			regex:       regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}%s[~-]{1}초등", alTypeString)),
			replaceFrom: alTypeString,
		})

		regexAgeMonthsToElemGrade = append(regexAgeMonthsToElemGrade, ageLimitRegexInfo{
			alType:      alType,
			regex:       regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}%s[~-]{1}초[1-6]{1}", alTypeString)),
			replaceFrom: alTypeString,
		})

		regexAgeMonthsInParentheses = append(regexAgeMonthsInParentheses, ageLimitRegexInfo{
			alType:      alType,
			regex:       regexp.MustCompile(fmt.Sprintf("\\([0-9]{1,2}%s\\)", alTypeString)),
			replaceFrom: alTypeString,
		})
	}
}

type AgeLimitRange struct {
	alType AgeLimitType
	from   int
	to     int
}

// AgeLimitRule 수강생의 연령(또는 개월수)에 맞지 않는 강좌를 제외합니다.
type AgeLimitRule struct {
	cultureLecturerMonths int
	cultureLecturerAge    int
}

func NewAgeLimitRule(months, age int) *AgeLimitRule {
	return &AgeLimitRule{
		cultureLecturerMonths: months,
		cultureLecturerAge:    age,
	}
}

func (r *AgeLimitRule) IsExcluded(lecture *domain.Lecture) bool {
	alType, from, to, err := extractMonthsOrAgeRange(lecture)
	if err != nil {
		log.Printf("연령 범위 추출 오류 (강좌명: %s): %v", lecture.Title, err)
		return false
	}

	if alType == AgeLimitMonths {
		if r.cultureLecturerMonths < from || r.cultureLecturerMonths > to {
			return true
		}
	} else if alType == AgeLimitAge {
		if r.cultureLecturerAge < from || r.cultureLecturerAge > to {
			return true
		}
	}
	return false
}

func extractMonthsOrAgeRange(lecture *domain.Lecture) (AgeLimitType, int, int, error) {
	for _, info := range regexAgeMonthsOverOrAdult {
		fs := info.regex.FindString(lecture.Title)
		if len(fs) > 0 {
			from, err := strconv.Atoi(strings.ReplaceAll(fs, info.replaceFrom, ""))
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}
			return info.alType, from, math.MaxInt32, nil
		}
	}

	for _, info := range regexAgeMonthsRange {
		fs := info.regex.FindString(lecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, info.replaceFrom, ""), "-", "~"), "~")

			value1, err := strconv.Atoi(split[0])
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}
			value2, err := strconv.Atoi(split[1])
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}

			if value1 < value2 {
				return info.alType, value1, value2, nil
			} else {
				return info.alType, value2, value1, nil
			}
		}
	}

	for _, info := range regexAgeMonthsToElemSchool {
		fs := info.regex.FindString(lecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, info.replaceFrom, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}

			to := 13
			if info.alType == AgeLimitMonths {
				to *= 12
			}

			return info.alType, from, to, nil
		}
	}

	for _, info := range regexAgeMonthsToElemGrade {
		fs := info.regex.FindString(lecture.Title)
		if len(fs) > 0 {
			split := strings.Split(strings.ReplaceAll(strings.ReplaceAll(fs, info.replaceFrom, ""), "-", "~"), "~")

			from, err := strconv.Atoi(split[0])
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}

			to, err := strconv.Atoi(strings.ReplaceAll(split[1], "초", ""))
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}

			to += 7
			if info.alType == AgeLimitMonths {
				to *= 12
			}

			return info.alType, from, to, nil
		}
	}

	for _, info := range regexAgeMonthsInParentheses {
		fs := info.regex.FindString(lecture.Title)
		if len(fs) > 0 {
			no, err := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(fs, info.replaceFrom, ""), "(", ""), ")", ""))
			if err != nil {
				return AgeLimitUnknwon, 0, 0, err
			}

			return info.alType, no, no, nil
		}
	}

	// 초a~초b, 초a-초b
	fs := regexElemGradeRange.FindString(lecture.Title)
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
	fs = regexYearToYearRange.FindString(lecture.Title)
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
	fs = regexYearTo2DigitYear.FindString(lecture.Title)
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
	fs = regex2DigitYearRange.FindString(lecture.Title)
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
	fs = regexYearOver.FindString(lecture.Title)
	if len(fs) > 0 {
		from, err := strconv.Atoi(strings.ReplaceAll(fs, "년생 이상", ""))
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}

		return AgeLimitAge, now.Year() - from + 1, math.MaxInt32, nil
	}

	// nn년생 이상
	fs = regex2DigitYearOver.FindString(lecture.Title)
	if len(fs) > 0 {
		from, err := strconv.Atoi(strings.ReplaceAll(fs, "년생 이상", ""))
		if err != nil {
			return AgeLimitUnknwon, 0, 0, err
		}

		return AgeLimitAge, now.Year() - (2000 + from) + 1, math.MaxInt32, nil
	}

	// 성인~nnnn년
	// 성인~nnnn년생
	fs = regexAdultToYear.FindString(lecture.Title)
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
		if strings.Contains(lecture.Title, k) {
			return v.alType, v.from, v.to, nil
		}
	}

	if !lecture.ScrapeExcluded {
		log.Printf(" >> 수집된 강좌의 연령(나이, 개월수) 추출 실패, 필터링 대상에서 제외됩니다.(%s : %s)", lecture.StoreName, lecture.Title)
	}

	return AgeLimitUnknwon, 0, math.MaxInt32, nil
}
