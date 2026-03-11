package filter

import "github.com/darkkaiser/culturelecture-scrape/internal/domain"

// Rule 각 수집기로부터 가져온 강좌 데이터 중에서, 불필요한 강좌를 걸러내기 위한
// '단일 필터링 조건'을 정의하는 인터페이스입니다.
type Rule interface {
	IsExcluded(lecture *domain.Lecture) bool
}
