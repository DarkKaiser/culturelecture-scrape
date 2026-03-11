package filter

import (
	"testing"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

func TestWeekdayDaytimeRule_IsExcluded(t *testing.T) {
	// 주입해줄 테스트용 임의 공휴일 목록
	testHolidays := []string{"2024-03-01", "2024-05-05", "2024-08-15"}
	rule := NewWeekdayDaytimeRule(testHolidays)

	tests := []struct {
		name         string
		startDate    string // 개강일 (YYYY-MM-DD 형식)
		startTime    string // 시작시간 (hh:mm 형식)
		weekday      string // 요일
		wantExcluded bool   // true면 필터링(제외)됨, false면 통과됨
	}{
		// ------------------------------------------------------------------
		// 제외 조건 (필터링되어야 하는 경우: 평일이면서 공휴일이 아니고 16시 미만)
		// ------------------------------------------------------------------
		{"Weekday_Daytime_Morning", "2024-04-01", "10:30", "월요일", true}, // 오전 평일
		{"Weekday_Daytime_Afternoon", "2024-04-03", "15:59", "수요일", true}, // 16시 직전 평일

		// ------------------------------------------------------------------
		// 통과 조건 1: 시간대 조건 불만족 (평일이더라도 16시 이후 시작)
		// ------------------------------------------------------------------
		{"Weekday_Evening_Boundary", "2024-04-04", "16:00", "목요일", false}, // 정각 16시 (수용)
		{"Weekday_Night", "2024-04-05", "19:00", "금요일", false}, // 저녁 시간대

		// ------------------------------------------------------------------
		// 통과 조건 2: 요일 조건 불만족 (주말 강좌)
		// ------------------------------------------------------------------
		{"Weekend_Saturday_Morning", "2024-04-06", "10:00", "토요일", false}, // 주말은 시간 관계없이 수용
		{"Weekend_Sunday_Morning", "2024-04-07", "14:00", "일요일", false},

		// ------------------------------------------------------------------
		// 통과 조건 3: 평일이지만 공휴일 특례 (testHolidays에 포함된 날짜)
		// ------------------------------------------------------------------
		{"Holiday_Weekday_Morning", "2024-03-01", "10:00", "금요일", false}, // 지정된 공휴일은 평일 낮이어도 수용

		// ------------------------------------------------------------------
		// 에러 및 엣지 케이스 (파싱 실패 시 예외 처리로 무조건 통과됨)
		// ------------------------------------------------------------------
		{"Error_Parse_EmptyTime", "2024-04-02", "", "화요일", false}, // 빈 문자열 (안전망 작용 여부)
		{"Error_Parse_InvalidFormat1", "2024-04-02", "오전10시", "화요일", false}, // 숫자 이외의 포맷
		{"Error_Parse_InvalidFormat2", "2024-04-02", "9:30", "화요일", false}, // "hh:mm" 포맷에서 [:2] 슬라이싱 시 "9:" 문자 파싱으로 Atoi 에러 발생 (안전망)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lecture := &domain.Lecture{
				StartDate: tt.startDate,
				StartTime: tt.startTime,
				Weekday:   tt.weekday,
			}

			// 메서드 호출 결과(got)와 예상 결과(wantExcluded) 비교
			if got := rule.IsExcluded(lecture); got != tt.wantExcluded {
				t.Errorf("WeekdayDaytimeRule.IsExcluded() = %v, want %v (Weekday: %v, StartDate: %v, StartTime: %v)",
					got, tt.wantExcluded, tt.weekday, tt.startDate, tt.startTime)
			}
		})
	}
}
