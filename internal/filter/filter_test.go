package filter

import (
	"testing"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

func TestFilter(t *testing.T) {
	// 필터링에 사용할 사용자 환경 설정 Mock 세팅
	studentMonths := 40 // 약 3~4세
	studentAge := 4     // 4세
	holidays := []string{"2024-05-05", "2024-12-25"}

	// ------------------------------------------------------------------
	// 테스트 데이터 셋 (여러 강좌가 한데 묶여 들어오는 파이프라인 상황 시뮬레이션)
	// ------------------------------------------------------------------
	lectures := []domain.Lecture{
		{
			Title:          "이미 제외된 강좌",
			ScrapeExcluded: true, // 이전 파이프라인에서 이미 제외됨. 필터 규칙 검사 스킵 테스트용 (continue 도달 확인용)
		},
		{
			Title:  "접수 마감된 강좌",
			Status: domain.ReceptionStatusClosed, // ClosedReceptionRule 테스트용 (break 도달 확인용)
		},
		{
			Title: "재미있는 키즈발레 교실", // TitleExcludeKeywordRule 테스트용 (발레 키워드 필터링 작동 확인용)
		},
		{
			Title:     "평일 낮 주부 요리",
			Weekday:   "수요일",
			StartDate: "2024-04-10",
			StartTime: "10:00", // WeekdayDaytimeRule 테스트용 (평일 오전 강좌 필터링 확인용)
		},
		{
			Title:     "어린이 대상 미달 강좌 (7세 이상)", // AgeLimitRule 테스트용 (4세 아이가 듣지 못하는 연령 조건 확인용)
		},
		{
			Title:     "주말 아빠와 함께 (4세 적합)",
			Status:    domain.ReceptionStatusPossible,
			Weekday:   "일요일",
			StartDate: "2024-04-14",
			StartTime: "11:00", // 모든 필터를 무사히 통과해야 하는 정상(통과) 강좌 조건 충족 테스트용
		},
	}

	// 필터 함수 수행
	Filter(lectures, studentMonths, studentAge, holidays)

	// ------------------------------------------------------------------
	// 결과 어설션(검증)
	// ------------------------------------------------------------------
	tests := []struct {
		name         string
		index        int
		wantExcluded bool
	}{
		{"AlreadyExcluded_Skip", 0, true},             // 기존 값 유지
		{"ClosedReceptionRule_Catch", 1, true},        // 필터링되어야 함
		{"TitleExcludeKeywordRule_Catch", 2, true},    // 필터링되어야 함
		{"WeekdayDaytimeRule_Catch", 3, true},         // 필터링되어야 함
		{"AgeLimitRule_Catch", 4, true},               // 필터링되어야 함
		{"ValidLecture_Unchanged_Pass", 5, false},     // 필터링되지 않아야 함 (위의 모든 루프 통과)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lectures[tt.index].ScrapeExcluded; got != tt.wantExcluded {
				t.Errorf("Filter() result for '%s': got %v, want %v", lectureTypeDesc(tt.index), got, tt.wantExcluded)
			}
		})
	}
}

// 테스트 결과 출력 시 강좌의 식별을 돕기 위한 헬퍼 함수
func lectureTypeDesc(index int) string {
	descriptions := []string{
		"이미 제외된 강좌",
		"접수 마감된 강좌",
		"키워드 제외 강좌",
		"평일 주간 제외 강좌",
		"연령 미달/초과 강좌",
		"모든 필터 통과 정상 강좌",
	}
	if index >= 0 && index < len(descriptions) {
		return descriptions[index]
	}
	return "알 수 없는 강좌 타입"
}
