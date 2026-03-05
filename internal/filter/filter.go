package filter

import (
	"log"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// Filter 함수는 수집된 강좌 정보에 필터링 규칙(Rule)을 적용하여 제외할 강좌를 표시합니다.
// 내부적으로 Strategy 패턴 기반의 파이프라인을 사용합니다.
func Filter(lectures []domain.Lecture, cultureLecturerMonths int, cultureLecturerAge int, holidays []string) {
	// 필터링 규칙 목록을 조립합니다.
	rules := []Rule{
		NewClosedReceptionRule(),
		NewWeekdayTimeRule(holidays),
		NewKeywordRule(),
		NewAgeLimitRule(cultureLecturerMonths, cultureLecturerAge),
	}

	for i := range lectures {
		// 이미 수집 제외로 마킹되었다면 건너뜀
		if lectures[i].ScrapeExcluded {
			continue
		}

		// 여러 규칙을 순차적으로 적용합니다 (Chain of Responsibility)
		for _, rule := range rules {
			if rule.IsExcluded(&lectures[i]) {
				lectures[i].ScrapeExcluded = true
				break // 하나의 규칙에라도 걸리면 즉시 제외 처리
			}
		}
	}

	excludedLectureCount := 0
	for _, lecture := range lectures {
		if lecture.ScrapeExcluded {
			excludedLectureCount++
		}
	}

	log.Printf("총 %d건의 문화센터 강좌중에서 %d건이 필터링되어 제외되었습니다.", len(lectures), excludedLectureCount)
}
