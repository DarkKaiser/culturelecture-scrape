package filter

import "github.com/darkkaiser/culturelecture-scrape/internal/domain"

// ClosedReceptionRule 접수 상태가 마감인 강좌를 제외합니다.
type ClosedReceptionRule struct{}

func NewClosedReceptionRule() *ClosedReceptionRule {
	return &ClosedReceptionRule{}
}

func (r *ClosedReceptionRule) IsExcluded(lecture *domain.Lecture) bool {
	return lecture.Status == domain.ReceptionStatusClosed
}
