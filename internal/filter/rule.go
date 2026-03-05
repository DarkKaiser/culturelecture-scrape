package filter

import "github.com/darkkaiser/culturelecture-scrape/internal/domain"

// Rule 인터페이스는 특정 강좌가 수집 대상에서 제외되어야 하는지를 평가합니다.
type Rule interface {
	IsExcluded(lecture *domain.Lecture) bool
}
