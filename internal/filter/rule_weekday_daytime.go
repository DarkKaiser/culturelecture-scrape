package filter

import (
	"log"
	"slices"
	"strconv"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// WeekdayDaytimeRule 평일(월~금) 중 낮 시간대(16시 이전)에 개설된 강좌를 필터링 대상으로 표시하는 규칙입니다.
// 직장인이나 오후 이후에만 수강이 가능한 사용자를 위해, 참여가 사실상 어려운 평일 낮 강좌를 결과에서 제외합니다.
// 단, 공휴일로 등록된 날짜의 강좌는 직장인도 참여 가능하므로 이 규칙의 제외 대상에서 조건부 제외합니다.
type WeekdayDaytimeRule struct {
	weekdays []string // 평일 요일 목록 (월요일 ~ 금요일). 여기에 해당하는 요일의 강좌만 시간 조건을 추가로 검사합니다.
	holidays []string // 필터에서 제외할 공휴일 날짜 목록 (YYYY-MM-DD 형식). 공휴일은 평일이더라도 강좌를 제외하지 않습니다.
}

// 컴파일 타임에 인터페이스 구현 여부를 검증합니다.
var _ Rule = (*WeekdayDaytimeRule)(nil)

// NewWeekdayDaytimeRule WeekdayDaytimeRule의 인스턴스를 생성합니다.
func NewWeekdayDaytimeRule(holidays []string) *WeekdayDaytimeRule {
	return &WeekdayDaytimeRule{
		weekdays: []string{"월요일", "화요일", "수요일", "목요일", "금요일"},
		holidays: holidays,
	}
}

// IsExcluded 다음 두 조건을 모두 만족하는 강좌에 대해 true를 반환합니다.
//  1. 강좌 요일이 평일(월~금)에 해당하고
//  2. 강좌 개강일이 공휴일 목록에 포함되지 않으며
//  3. 강좌 시작 시각이 16시 미만(오전~오후 4시 이전)인 경우
//
// 시작 시간("hh:mm" 형식) 파싱에 실패한 강좌는 데이터 품질 오류로 판단하여 제외하지 않고 오류를 로그로 남깁니다.
func (r *WeekdayDaytimeRule) IsExcluded(lecture *domain.Lecture) bool {
	if slices.Contains(r.weekdays, lecture.Weekday) && !slices.Contains(r.holidays, lecture.StartDate) {
		if len(lecture.StartTime) < 2 {
			log.Printf("데이터 형식 오류: 강좌 시작 시각 길이가 너무 짧습니다. (강좌명: %s, 입력된 시각: %s)", lecture.Title, lecture.StartTime)
			return false
		}

		hour, err := strconv.Atoi(lecture.StartTime[:2])
		if err != nil {
			log.Printf("데이터 형식 오류: 강좌 시작 시각을 분석할 수 없습니다. (강좌명: %s, 입력된 시각: %s, 상세 원인: %v)", lecture.Title, lecture.StartTime, err)
			return false
		}

		if hour < 16 {
			return true
		}
	}

	return false
}
