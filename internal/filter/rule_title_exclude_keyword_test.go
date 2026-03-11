package filter

import (
	"testing"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

func TestTitleExcludeKeywordRule_IsExcluded(t *testing.T) {
	rule := NewTitleExcludeKeywordRule()

	tests := []struct {
		name         string
		title        string
		wantExcluded bool
	}{
		// ------------------------------------------------------------------
		// 제외 조건 (필터링되어야 하는 경우: 키워드가 제목 어딘가에 포함된 경우)
		// ------------------------------------------------------------------
		// 정확히 키워드만 있는 경우
		{"ExactMatch_1", "키즈발레", true},
		{"ExactMatch_2", "밸리댄스", true},
		
		// 띄어쓰기 등 공백이 포함된 하드코딩 키워드에 대한 정확 매치
		{"ExactMatch_Space", "체형교정 발레", true},
		
		// 키워드가 제목 앞, 뒤, 중간에 포함된 경우 (Substring 매칭)
		{"Substring_Front", "엔젤 발레 초급반 (오전)", true},
		{"Substring_Back", "여름방학 특별 쁘띠발레", true},
		{"Substring_Middle", "신나는 트윈클 동화발레 교실", true},
		{"Substring_With_Bracket", "[광주국제영어마을] 스토리텔링 시간", true}, // `[` 광주국제영어마을 로 시작되는 하드코딩 키워드 매칭

		// ------------------------------------------------------------------
		// 통과 조건 (필터링되지 않아야 하는 경우: 키워드가 포함되지 않은 경우)
		// ------------------------------------------------------------------
		{"NoMatch_Normal_1", "어린이 요가 교실", false},
		{"NoMatch_Normal_2", "성인 수채화 반", false},
		
		// 비슷한 단어지만 정확한 키워드가 아닌 경우 (TitleExcludeKeywordRule 은 "발레" 단독 키워드는 없음)
		{"NoMatch_Similar", "성인 성악 발레곡 부르기", false}, // "발레"라는 단어만으로는 필터링 대상이 아님에 유의

		// 빈 문자열인 경우
		{"NoMatch_Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lecture := &domain.Lecture{
				Title: tt.title,
			}

			// 메서드 호출 결과(got)와 예상 결과(wantExcluded) 비교
			if got := rule.IsExcluded(lecture); got != tt.wantExcluded {
				t.Errorf("TitleExcludeKeywordRule.IsExcluded() = %v, want %v (Title: %q)", got, tt.wantExcluded, tt.title)
			}
		})
	}
}
