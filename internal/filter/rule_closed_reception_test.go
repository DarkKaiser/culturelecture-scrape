package filter

import (
	"testing"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

func TestClosedReceptionRule_IsExcluded(t *testing.T) {
	rule := NewClosedReceptionRule()

	tests := []struct {
		name         string
		status       domain.ReceptionStatus
		wantExcluded bool
	}{
		// ------------------------------------------------------------------
		// 제외 조건 (필터링되어야 하는 경우)
		// ------------------------------------------------------------------
		{
			name:         "Status_Closed",
			status:       domain.ReceptionStatusClosed,
			wantExcluded: true, // 접수마감인 경우 테스트 통과(필터링 대상)
		},

		// ------------------------------------------------------------------
		// 통과 조건 (필터링되지 않아야 하는 경우)
		// ------------------------------------------------------------------
		{
			name:         "Status_Possible",
			status:       domain.ReceptionStatusPossible,
			wantExcluded: false,
		},
		{
			name:         "Status_Planned",
			status:       domain.ReceptionStatusPlanned,
			wantExcluded: false,
		},
		{
			name:         "Status_StandBy",
			status:       domain.ReceptionStatusStandBy,
			wantExcluded: false,
		},
		{
			name:         "Status_OnsiteConsultation",
			status:       domain.ReceptionStatusOnsiteConsultation,
			wantExcluded: false,
		},
		{
			name:         "Status_OnsiteFCFS",
			status:       domain.ReceptionStatusOnsiteFCFS,
			wantExcluded: false,
		},
		{
			name:         "Status_OnsiteInquiry",
			status:       domain.ReceptionStatusOnsiteInquiry,
			wantExcluded: false,
		},
		{
			name:         "Status_PhoneInquiry",
			status:       domain.ReceptionStatusPhoneInquiry,
			wantExcluded: false,
		},
		{
			name:         "Status_WalkIn",
			status:       domain.ReceptionStatusWalkIn,
			wantExcluded: false,
		},
		{
			name:         "Status_Unknown",
			status:       domain.ReceptionStatusUnknown,
			wantExcluded: false, // 알 수 없는 상태도 기본적으로 필터링하지 않음
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lecture := &domain.Lecture{
				Status: tt.status,
			}

			// 메서드 호출 결과(got)와 예상 결과(wantExcluded) 비교
			if got := rule.IsExcluded(lecture); got != tt.wantExcluded {
				t.Errorf("ClosedReceptionRule.IsExcluded() = %v, want %v (Status: %v)", got, tt.wantExcluded, tt.status)
			}
		})
	}
}
