package filter

import (
	"strings"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// KeywordRule 강좌명에 특정 문자열이 포함되어 있는 경우 수집에서 제외합니다.
type KeywordRule struct {
	excludedKeywords []string
}

func NewKeywordRule() *KeywordRule {
	return &KeywordRule{
		excludedKeywords: []string{
			"키즈발레", "영어발레", "엔젤발레", "엔젤 발레", "체형교정발레", "체형교정 발레",
			"YSM발레", "YSM 발레", "쁘띠발레", "발레리나", "앨리스 스토리텔링 발레", "트윈클 동화발레",
			"밸리댄스", "[광주국제영어마을",
		},
	}
}

func (r *KeywordRule) IsExcluded(lecture *domain.Lecture) bool {
	for _, kw := range r.excludedKeywords {
		if strings.Contains(lecture.Title, kw) {
			return true
		}
	}
	return false
}
