package filter

import (
	"strings"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// TitleExcludeKeywordRule 강좌명에 사전에 정의된 제외 키워드가 포함된 경우, 해당 강좌를 필터링 대상으로 표시하는 규칙입니다.
// 사용자의 관심 분야와 무관한 강좌(예: 발레, 밸리댄스 등)를 결과에서 미리 걸러냅니다.
type TitleExcludeKeywordRule struct {
	keywords []string // 제외 키워드 목록. 이 목록 중 하나라도 포함되면 필터링됩니다.
}

// 컴파일 타임에 인터페이스 구현 여부를 검증합니다.
var _ Rule = (*TitleExcludeKeywordRule)(nil)

// NewTitleExcludeKeywordRule TitleExcludeKeywordRule의 인스턴스를 생성합니다.
func NewTitleExcludeKeywordRule() *TitleExcludeKeywordRule {
	return &TitleExcludeKeywordRule{
		keywords: []string{
			"키즈발레", "영어발레", "엔젤발레", "엔젤 발레", "체형교정발레", "체형교정 발레",
			"YSM발레", "YSM 발레", "쁘띠발레", "발레리나", "앨리스 스토리텔링 발레", "트윈클 동화발레",
			"밸리댄스", "[광주국제영어마을",
		},
	}
}

// IsExcluded 강좌명에 제외할 키워드가 포함되어 있는지 검사합니다.
// 등록된 키워드 중 하나라도 제목에 포함되어 있으면 필터링 대상으로 판단합니다.
func (r *TitleExcludeKeywordRule) IsExcluded(lecture *domain.Lecture) bool {
	for _, k := range r.keywords {
		if strings.Contains(lecture.Title, k) {
			return true
		}
	}

	return false
}
