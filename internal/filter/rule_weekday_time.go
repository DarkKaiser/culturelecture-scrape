package filter

import (
	"log"
	"slices"
	"strconv"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// WeekdayTimeRule 주말 및 공휴일이 아닌 평일 16시 이전의 강좌를 제외합니다.
type WeekdayTimeRule struct {
	holidays []string
	weekdays []string
}

func NewWeekdayTimeRule(holidays []string) *WeekdayTimeRule {
	return &WeekdayTimeRule{
		holidays: holidays,
		weekdays: []string{"월요일", "화요일", "수요일", "목요일", "금요일"},
	}
}

func (r *WeekdayTimeRule) IsExcluded(lecture *domain.Lecture) bool {
	if slices.Contains(r.weekdays, lecture.DayOfTheWeek) && !slices.Contains(r.holidays, lecture.StartDate) {
		h24, err := strconv.Atoi(lecture.StartTime[:2])
		if err != nil {
			log.Printf("강좌 시작시간 파싱 오류 (강좌명: %s, 시간: %s): %v", lecture.Title, lecture.StartTime, err)
			return false
		}

		if h24 < 16 {
			return true
		}
	}
	return false
}
