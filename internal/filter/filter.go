package filter

import (
	"log"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// Filter 수집된 전체 강좌 목록을 순회하며, 사전에 정의된 여러 필터링 규칙(Rule)을 차례대로 적용합니다.
// 조건에 부합하여 제외되어야 할 강좌는 ScrapeExcluded 플래그가 true 로 설정됩니다.
func Filter(lectures []domain.Lecture, studentMonths, studentAge int, holidays []string) {
	// 1. 필터링 파이프라인 구성
	rules := []Rule{
		NewClosedReceptionRule(),                   // 접수 마감된 강좌 제외
		NewWeekdayDaytimeRule(holidays),            // 평일 주간 강좌 제외 (공휴일은 제외하지 않음)
		NewTitleExcludeKeywordRule(),               // 원치 않는 키워드가 포함된 강좌 제외
		NewAgeLimitRule(studentMonths, studentAge), // 대상 연령이 맞지 않는 강좌 제외
	}

	// 2. 강좌별 규칙 적용
	for i := range lectures {
		// 이전 단계(다른 처리기)에서 이미 제외 처리된 강좌라면, 추가 필터링 없이 건너뜁니다.
		if lectures[i].ScrapeExcluded {
			continue
		}

		// 등록된 규칙들을 순차적으로 적용합니다.
		for _, rule := range rules {
			if rule.IsExcluded(&lectures[i]) {
				// 단 하나의 규칙이라도 '제외 대상'이라고 판단하면, 즉시 플래그를 설정하고 다음 강좌로 넘어갑니다.
				lectures[i].ScrapeExcluded = true
				break
			}
		}
	}

	// 3. 필터링 결과 집계 및 로깅
	// 전체 강좌 중 최종적으로 제외 처리된 강좌의 수를 카운트하여 로그로 남깁니다.
	excludedCount := 0
	for _, lecture := range lectures {
		if lecture.ScrapeExcluded {
			excludedCount++
		}
	}

	log.Printf("[필터링 완료] 전체 강좌 수: %d건 | 필터링 제외 대상: %d건", len(lectures), excludedCount)
}
