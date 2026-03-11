package filter

import "github.com/darkkaiser/culturelecture-scrape/internal/domain"

// ClosedReceptionRule 접수 상태가 이미 마감된 강좌를 필터링 대상으로 표시하는 규칙입니다.
type ClosedReceptionRule struct{}

// 컴파일 타임에 인터페이스 구현 여부를 검증합니다.
var _ Rule = (*ClosedReceptionRule)(nil)

// NewClosedReceptionRule ClosedReceptionRule의 인스턴스를 생성합니다.
func NewClosedReceptionRule() *ClosedReceptionRule {
	return &ClosedReceptionRule{}
}

// IsExcluded 강좌의 접수 상태가 '접수마감'이면 true를 반환합니다.
// 마감된 강좌는 ScrapeExcluded 플래그가 true로 설정되어 CSV 저장 단계에서 건너뜁니다.
func (r *ClosedReceptionRule) IsExcluded(lecture *domain.Lecture) bool {
	return lecture.Status == domain.ReceptionStatusClosed
}
