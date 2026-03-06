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

// ageUnit 강좌의 연령 제한 기준 단위를 나타냅니다.
// 강좌명에서 추출한 연령이 나이(세) 기준인지, 개월 수 기준인지를 구분합니다.
type ageUnit int

const (
	ageUnitUnknown ageUnit = iota // 단위를 판별할 수 없음 (연령 정보 미포함 강좌 등)
	ageUnitYear                   // 나이(세) 기준 (예: 7세 이상, 5~10세)
	ageUnitMonth                  // 개월 수 기준 (예: 24개월 이상, 6~36개월)
)

// ageParsePattern 강좌명 문자열에서 연령 범위를 추출하기 위한 정규식 파싱 패턴입니다.
// 각 패턴은 특정 표기 형식(예: "7세 이상", "24개월~36개월")을 인식하고, 불필요한 문자를 제거하여 순수한 숫자만 남기는 역할을 합니다.
type ageParsePattern struct {
	unit      ageUnit        // 이 패턴이 인식하는 연령의 단위 (나이 또는 개월)
	regex     *regexp.Regexp // 강좌명에서 연령 표기 부분을 찾아내는 정규표현식
	stripText string         // 숫자만 추출하기 위해 제거할 대상 문자열 (예: "세이상", "개월", "세")
}

var (
	// ------------------------------------------------------------------
	// init()에서 동적으로 생성되는 파싱 패턴 슬라이스
	// ------------------------------------------------------------------
	// 나이(세)와 개월(개월) 두 가지 단위에 대해 각각 패턴이 추가됩니다.

	ageOverPatterns          []ageParsePattern // N세/개월 이상, N세/개월~성인 등 하한만 있는 표기 (예: "7세 이상", "24개월~누구나")
	ageRangePatterns         []ageParsePattern // A세/개월~B세/개월 형태의 범위 표기 (예: "5~10세", "6~24개월")
	ageToElemSchoolPatterns  []ageParsePattern // N세/개월~초등 형태의 표기 (예: "5세~초등")
	ageToElemGradePatterns   []ageParsePattern // N세/개월~초a학년 형태의 표기 (예: "5세~초3")
	ageInParenthesesPatterns []ageParsePattern // 괄호 안에 표기된 연령 (예: "(7세)", "(24개월)")

	// ------------------------------------------------------------------
	// 고정 정규식 패턴 (출생연도 또는 초등학년 기반 표기)
	// ------------------------------------------------------------------
	// 아래 패턴들은 나이/개월 단위 구분 없이 항상 ageUnitYear로 처리됩니다.

	elemGradeRangePattern = regexp.MustCompile(`초[1-6][~-]초[1-6]`)      // 초등학년 범위 (예: "초1~초3", "초2-초6")
	year4ToYear4Pattern   = regexp.MustCompile(`[0-9]{4}년?~[0-9]{4}년생`) // 4자리 출생연도 범위 (예: "2010~2015년생")
	year4ToYear2Pattern   = regexp.MustCompile(`[0-9]{4}년?~[0-9]{2}년생`) // 4자리~2자리 출생연도 범위 (예: "2010~15년생")
	year2ToYear2Pattern   = regexp.MustCompile(`[0-9]{2}~[0-9]{2}년생?`)  // 2자리 출생연도 범위 (예: "10~15년생")
	year4OverPattern      = regexp.MustCompile(`[0-9]{4}년생 이상`)         // 4자리 출생연도 이상 (예: "2010년생 이상")
	year2OverPattern      = regexp.MustCompile(`[0-9]{2}년생 이상`)         // 2자리 출생연도 이상 (예: "10년생 이상")
	adultToYear4Pattern   = regexp.MustCompile(`성인~[0-9]{4}년생?`)        // 성인부터 특정 출생연도까지 (예: "성인~2015년생")
)

// init 강좌명에서 연령 정보를 분석해내기 위한 다양한 정규식 패턴 세트들을 미리 구성해 둡니다.
func init() {
	// 연령을 파악하는 두 가지 기준 단위(나이, 개월 수)와 이를 강좌명에서 추출하기 위해 사용되는 텍스트(예: "세", "개월")를 매핑합니다.
	unitSuffixes := map[ageUnit]string{
		ageUnitYear:  "세",
		ageUnitMonth: "개월",
	}

	for unit, suffix := range unitSuffixes {
		// "N세/개월 이상", "N세/개월~성인" 등 하한만 있는 표기 형식의 패턴을 추가합니다.
		for _, postfix := range []string{suffix + "이상", suffix + " 이상", suffix + "~성인", suffix + "~ 성인", suffix + "~누구나", suffix + "~ 누구나"} {
			ageOverPatterns = append(ageOverPatterns, ageParsePattern{
				unit:      unit,
				regex:     regexp.MustCompile("[0-9]{1,2}" + postfix),
				stripText: postfix,
			})
		}

		// "A세~B세", "A개월-B개월" 등 양쪽 경계값이 있는 범위 표기 형식의 패턴을 추가합니다.
		ageRangePatterns = append(ageRangePatterns, ageParsePattern{
			unit:      unit,
			regex:     regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}[%s]?[~-]{1}[0-9]{1,2}%s", suffix, suffix)),
			stripText: suffix,
		})

		// "N세~초등", "N개월~초등" 등 초등학교 입학 연령까지의 범위 표기 형식의 패턴을 추가합니다.
		ageToElemSchoolPatterns = append(ageToElemSchoolPatterns, ageParsePattern{
			unit:      unit,
			regex:     regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}%s[~-]{1}초등", suffix)),
			stripText: suffix,
		})

		// "N세~초a", "N개월~초a" 등 특정 초등학년까지의 범위 표기 형식의 패턴을 추가합니다.
		ageToElemGradePatterns = append(ageToElemGradePatterns, ageParsePattern{
			unit:      unit,
			regex:     regexp.MustCompile(fmt.Sprintf("[0-9]{1,2}%s[~-]{1}초[1-6]{1}", suffix)),
			stripText: suffix,
		})

		// "(N세)", "(N개월)" 등 괄호 안에 단일 연령이 표기된 형식의 패턴을 추가합니다.
		ageInParenthesesPatterns = append(ageInParenthesesPatterns, ageParsePattern{
			unit:      unit,
			regex:     regexp.MustCompile(fmt.Sprintf("\\([0-9]{1,2}%s\\)", suffix)),
			stripText: suffix,
		})
	}
}

// ageLimitRange 강좌명에서 파싱된 연령 제한 범위를 담는 구조체입니다.
type ageLimitRange struct {
	unit   ageUnit // 연령 단위 (나이 또는 개월)
	minAge int     // 수강 가능한 최소 연령 (포함)
	maxAge int     // 수강 가능한 최대 연령 (포함, math.MaxInt32 이면 상한 없음)
}

// AgeLimitRule 수강생의 연령(또는 개월 수)이 강좌의 수강 가능 범위를 벗어나면 해당 강좌를 제외합니다.
type AgeLimitRule struct {
	studentMonths int // 수강생의 나이를 개월 수로 환산한 값
	studentAge    int // 수강생의 나이(세)
}

// 컴파일 타임에 AgeLimitRule이 Rule 인터페이스를 올바르게 구현하는지 검증합니다.
var _ Rule = (*AgeLimitRule)(nil)

// NewAgeLimitRule AgeLimitRule을 생성합니다.
func NewAgeLimitRule(months, age int) *AgeLimitRule {
	return &AgeLimitRule{
		studentMonths: months,
		studentAge:    age,
	}
}

// IsExcluded 수강생의 연령이 강좌의 수강 가능 범위를 벗어나면 true를 반환합니다.
func (r *AgeLimitRule) IsExcluded(lecture *domain.Lecture) bool {
	unit, minAge, maxAge, err := extractAgeRange(lecture)
	if err != nil {
		// 연령 파싱 중 예기치 못한 오류가 발생한 경우, 해당 강좌를 제외 대상으로 처리하지 않습니다.
		// (오류로 인해 정상적인 강좌가 누락되는 것을 방지하기 위한 안전 정책)
		log.Printf("[경고] 강좌명 연령 제한 추출 실패 (강좌명: %s) - 상세: %v", lecture.Title, err)

		return false
	}

	switch unit {
	case ageUnitMonth:
		// 강좌가 개월 수 기준일 때: 수강생의 개월 수가 허용 범위 밖이면 제외합니다.
		if r.studentMonths < minAge || r.studentMonths > maxAge {
			return true
		}

	case ageUnitYear:
		// 강좌가 나이(세) 기준일 때: 수강생의 나이가 허용 범위 밖이면 제외합니다.
		if r.studentAge < minAge || r.studentAge > maxAge {
			return true
		}
	}

	// 연령 정보가 없거나 수강 가능 범위 안에 있으면 제외하지 않습니다.
	return false
}

// extractAgeRange 강좌명(lecture.Title)에서 수강 가능 연령 범위를 파싱하여 반환합니다.
func extractAgeRange(lecture *domain.Lecture) (ageUnit, int, int, error) {
	// ------------------------------------------------------------------
	// [패턴 그룹 1] N세/개월 이상, N세/개월~성인 등 하한만 있는 표기
	// ------------------------------------------------------------------

	// 매칭 예: "7세 이상", "24개월~누구나", "5세~성인"
	for _, pattern := range ageOverPatterns {
		match := pattern.regex.FindString(lecture.Title)
		if len(match) > 0 {
			// 접미어(예: "세 이상")를 제거하면 순수한 숫자(하한 연령)만 남습니다.
			minAge, err := strconv.Atoi(strings.ReplaceAll(match, pattern.stripText, ""))
			if err != nil {
				return ageUnitUnknown, 0, 0, err
			}
			return pattern.unit, minAge, math.MaxInt32, nil
		}
	}

	// ------------------------------------------------------------------
	// [패턴 그룹 2] A세~B세, A개월-B개월 등 양쪽 경계값이 있는 범위 표기
	// ------------------------------------------------------------------

	// 매칭 예: "5~10세", "6~24개월", "3세~7세"
	for _, pattern := range ageRangePatterns {
		match := pattern.regex.FindString(lecture.Title)
		if len(match) > 0 {
			// 접미어 제거 후 "-"를 "~"로 통일하고, "~"로 분리하여 age1/age2 추출
			parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(match, pattern.stripText, ""), "-", "~"), "~")

			age1, err := strconv.Atoi(parts[0])
			if err != nil {
				return ageUnitUnknown, 0, 0, err
			}
			age2, err := strconv.Atoi(parts[1])
			if err != nil {
				return ageUnitUnknown, 0, 0, err
			}

			// 강좌명에 따라 순서가 뒤바뀔 수 있으므로, 항상 작은 값이 minAge(최솟값)가 되도록 정렬합니다.
			if age1 < age2 {
				return pattern.unit, age1, age2, nil
			} else {
				return pattern.unit, age2, age1, nil
			}
		}
	}

	// ------------------------------------------------------------------
	// [패턴 그룹 3] N세/개월~초등 형태 — 초등학생 최고 학년 연령(만 13세)을 상한으로 사용
	// ------------------------------------------------------------------

	// 매칭 예: "5세~초등", "24개월~초등"
	for _, pattern := range ageToElemSchoolPatterns {
		match := pattern.regex.FindString(lecture.Title)
		if len(match) > 0 {
			parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(match, pattern.stripText, ""), "-", "~"), "~")

			minAge, err := strconv.Atoi(parts[0])
			if err != nil {
				return ageUnitUnknown, 0, 0, err
			}

			// 초등학교 최고 학년(6학년)의 나이는 만 13세로 봅니다.
			// 단위가 개월인 경우 13세 × 12개월 = 156개월로 환산합니다.
			maxAge := 13
			if pattern.unit == ageUnitMonth {
				maxAge *= 12
			}

			return pattern.unit, minAge, maxAge, nil
		}
	}

	// ------------------------------------------------------------------
	// [패턴 그룹 4] N세/개월~초a학년 형태 — 특정 초등 학년을 상한으로 사용
	// ------------------------------------------------------------------

	// 매칭 예: "5세~초3", "5세~초6"
	for _, pattern := range ageToElemGradePatterns {
		match := pattern.regex.FindString(lecture.Title)
		if len(match) > 0 {
			parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(match, pattern.stripText, ""), "-", "~"), "~")

			minAge, err := strconv.Atoi(parts[0])
			if err != nil {
				return ageUnitUnknown, 0, 0, err
			}

			// parts[1]에 "초a" 형태로 학년이 남아 있으므로 "초"를 제거하여 숫자만 추출합니다.
			maxAge, err := strconv.Atoi(strings.ReplaceAll(parts[1], "초", ""))
			if err != nil {
				return ageUnitUnknown, 0, 0, err
			}

			// 초등학교 입학 나이(만 7세)를 기준으로 학년을 나이로 환산합니다. (예: 초3 → 3 + 7 = 10세, 초6 → 6 + 7 = 13세)
			// 단위가 개월이면 환산된 나이에 12를 곱해 개월 수로 변환합니다.
			maxAge += 7
			if pattern.unit == ageUnitMonth {
				maxAge *= 12
			}

			return pattern.unit, minAge, maxAge, nil
		}
	}

	// ------------------------------------------------------------------
	// [패턴 그룹 5] (N세), (N개월) 형태 — 괄호 안에 단일 연령 표기
	// ------------------------------------------------------------------

	// 매칭 예: "(7세)", "(24개월)"
	for _, pattern := range ageInParenthesesPatterns {
		match := pattern.regex.FindString(lecture.Title)
		if len(match) > 0 {
			// 접미어와 괄호 "(", ")"를 모두 제거하면 숫자만 남습니다.
			exactAge, err := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(match, pattern.stripText, ""), "(", ""), ")", ""))
			if err != nil {
				return ageUnitUnknown, 0, 0, err
			}

			// minAge == maxAge 로 설정하여 해당 연령만 수용합니다.
			return pattern.unit, exactAge, exactAge, nil
		}
	}

	// ------------------------------------------------------------------
	// [고정 패턴 1] 초a~초b, 초a-초b 형태 — 초등 학년 범위
	// ------------------------------------------------------------------

	// 매칭 예: "초1~초3", "초2-초6"
	match := elemGradeRangePattern.FindString(lecture.Title)
	if len(match) > 0 {
		parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(match, "초", ""), "-", "~"), "~")

		age1, err := strconv.Atoi(parts[0])
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}
		age2, err := strconv.Atoi(parts[1])
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}

		// 초등 a학년 = (a + 7)세로 환산하고, 항상 작은 값이 minAge가 되도록 정렬합니다.
		if age1 < age2 {
			return ageUnitYear, age1 + 7, age2 + 7, nil
		} else {
			return ageUnitYear, age2 + 7, age1 + 7, nil
		}
	}

	// ------------------------------------------------------------------
	// [고정 패턴 2] nnnn~nnnn년생, nnnn년~nnnn년생 — 4자리 출생연도 범위
	// ------------------------------------------------------------------

	// 아래 고정 패턴 2~7은 모두 "OO년생" 형태의 출생연도를 나이(세)로 변환하여 처리합니다.
	// 이 프로그램은 연 나이(한국식 나이) 기준이므로 다음 공식을 사용합니다.
	//   나이 = 현재 연도 - 출생 연도 + 1
	now := time.Now()

	// 매칭 예: "2010~2015년생", "2012년~2018년생"
	match = year4ToYear4Pattern.FindString(lecture.Title)
	if len(match) > 0 {
		// "년생", "년" 을 모두 제거한 뒤 "~"로 분리하여 두 출생연도를 추출합니다.
		parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(match, "년생", ""), "년", ""), "~")

		age1, err := strconv.Atoi(parts[0])
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}
		age2, err := strconv.Atoi(parts[1])
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}

		// 두 출생연도를 각각 현재 나이(연 나이)로 변환합니다.
		age1 = now.Year() - age1 + 1
		age2 = now.Year() - age2 + 1

		// 항상 작은 나이가 minAge가 되도록 정렬합니다.
		if age1 < age2 {
			return ageUnitYear, age1, age2, nil
		} else {
			return ageUnitYear, age2, age1, nil
		}
	}

	// ------------------------------------------------------------------
	// [고정 패턴 3] nnnn~nn년생, nnnn년~nn년생 — 4자리+2자리 출생연도 범위
	// ------------------------------------------------------------------

	// 매칭 예: "2010~15년생", "2012년~18년생"
	match = year4ToYear2Pattern.FindString(lecture.Title)
	if len(match) > 0 {
		parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(match, "년생", ""), "년", ""), "~")

		age1, err := strconv.Atoi(parts[0])
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}
		age2, err := strconv.Atoi(parts[1])
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}

		// age1: 4자리 연도 그대로 나이로 역산합니다.
		// age2: 2자리 연도(예: "15")는 2000년대생으로 보고 2000을 더해 보정한 뒤 역산합니다.
		age1 = now.Year() - age1 + 1
		age2 = now.Year() - (2000 + age2) + 1

		if age1 < age2 {
			return ageUnitYear, age1, age2, nil
		} else {
			return ageUnitYear, age2, age1, nil
		}
	}

	// ------------------------------------------------------------------
	// [고정 패턴 4] nn~nn년, nn~nn년생 — 2자리 출생연도 범위
	// ------------------------------------------------------------------

	// 매칭 예: "10~15년생", "18~22년"
	match = year2ToYear2Pattern.FindString(lecture.Title)
	if len(match) > 0 {
		parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(match, "년생", ""), "년", ""), "~")

		age1, err := strconv.Atoi(parts[0])
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}
		age2, err := strconv.Atoi(parts[1])
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}

		// age1, age2 모두 2자리 연도(예: "10" → 2010년생)이므로 2000을 더해 보정한 뒤 나이로 역산합니다.
		age1 = now.Year() - (2000 + age1) + 1
		age2 = now.Year() - (2000 + age2) + 1

		if age1 < age2 {
			return ageUnitYear, age1, age2, nil
		} else {
			return ageUnitYear, age2, age1, nil
		}
	}

	// ------------------------------------------------------------------
	// [고정 패턴 5] nnnn년생 이상 — 4자리 출생연도 기준 하한
	// ------------------------------------------------------------------

	// 매칭 예: "2010년생 이상"
	match = year4OverPattern.FindString(lecture.Title)
	if len(match) > 0 {
		birthYear, err := strconv.Atoi(strings.ReplaceAll(match, "년생 이상", ""))
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}

		return ageUnitYear, now.Year() - birthYear + 1, math.MaxInt32, nil
	}

	// ------------------------------------------------------------------
	// [고정 패턴 6] nn년생 이상 — 2자리 출생연도 기준 하한
	// ------------------------------------------------------------------

	// 매칭 예: "10년생 이상" → 2010년생 이상
	match = year2OverPattern.FindString(lecture.Title)
	if len(match) > 0 {
		birthYear, err := strconv.Atoi(strings.ReplaceAll(match, "년생 이상", ""))
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}

		return ageUnitYear, now.Year() - (2000 + birthYear) + 1, math.MaxInt32, nil
	}

	// ------------------------------------------------------------------
	// [고정 패턴 7] 성인~nnnn년, 성인~nnnn년생 — 성인부터 특정 출생연도까지
	// ------------------------------------------------------------------

	// 매칭 예: "성인~2015년생", "성인~2010년"
	match = adultToYear4Pattern.FindString(lecture.Title)
	if len(match) > 0 {
		parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(match, "년생", ""), "년", ""), "~")

		// parts[0]은 "성인"(문자열), parts[1]이 출생연도 숫자입니다.
		birthYear, err := strconv.Atoi(parts[1])
		if err != nil {
			return ageUnitUnknown, 0, 0, err
		}

		return ageUnitYear, now.Year() - birthYear + 1, math.MaxInt32, nil
	}

	// ------------------------------------------------------------------
	// [특정 키워드 매핑] 정규식으로 파싱이 어려운 문자열 표기를 직접 처리
	// ------------------------------------------------------------------

	// 강좌명에 아래 키워드가 포함된 경우, 사전에 정의된 데이터 기반의 연령 범위를 반환합니다.
	// 주의: 이 맵은 순서가 보장되지 않으므로, 키워드가 겹치는 강좌에서는 결과가 비결정적일 수 있습니다.
	keywordAgeLimits := map[string]ageLimitRange{
		"(초등)": {
			unit:   ageUnitYear,
			minAge: 8,  // 초등학교 1학년(만 7세, 한국식 8세)
			maxAge: 13, // 초등학교 6학년(만 12세, 한국식 13세)
		},
		"(초등반)": {
			unit:   ageUnitYear,
			minAge: 8,
			maxAge: 13,
		},
		"(모든연령": {
			unit:   ageUnitYear,
			minAge: 0,             // 연령 하한 없음
			maxAge: math.MaxInt32, // 연령 상한 없음
		},
		"(모든 연령)": {
			unit:   ageUnitYear,
			minAge: 0,
			maxAge: math.MaxInt32,
		},
		"(초등~성인)": {
			unit:   ageUnitYear,
			minAge: 8,             // 초등학생부터
			maxAge: math.MaxInt32, // 성인 이상 상한 없음
		},
		"(성인~중학생이상)": {
			unit:   ageUnitYear,
			minAge: 14,            // 중학교 1학년(만 13세, 한국식 14세)
			maxAge: math.MaxInt32, // 상한 없음
		},
		"(성인)": {
			unit:   ageUnitYear,
			minAge: 20,            // 성인(만 19세, 한국식 20세)
			maxAge: math.MaxInt32, // 상한 없음
		},
	}

	for keyword, limit := range keywordAgeLimits {
		if strings.Contains(lecture.Title, keyword) {
			return limit.unit, limit.minAge, limit.maxAge, nil
		}
	}

	// ------------------------------------------------------------------
	// [파싱 실패] 어떤 패턴에도 매칭되지 않은 경우
	// ------------------------------------------------------------------

	// 수집 대상 강좌(ScrapeExcluded == false)인 경우에만 경고 로그를 출력합니다.
	// 이미 수집 제외로 분류된 강좌는 로그를 남기지 않아 불필요한 노이즈를 방지합니다.
	if !lecture.ScrapeExcluded {
		log.Printf("[경고] 강좌명에서 연령 제한 기준을 식별할 수 없어 필터링 대상에서 제외합니다 (지점명: %s, 강좌명: %s)", lecture.StoreName, lecture.Title)
	}

	// 연령 정보를 명확히 특정할 수 없으므로 상한 없음(math.MaxInt32)을 기본값으로 적용하여 수용합니다.
	return ageUnitUnknown, 0, math.MaxInt32, nil
}
