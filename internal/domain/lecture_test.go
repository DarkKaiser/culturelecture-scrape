package domain

import (
	"testing"
)

// TestReceptionStatus_Mapping은 열거형 상수들이
// 한글 문자열 배열(ReceptionStatusStrings)에 올바르게 매핑되어 있는지 검증합니다.
func TestReceptionStatus_Mapping(t *testing.T) {
	tests := []struct {
		name     string
		status   ReceptionStatus
		expected string
	}{
		{"알수없음", ReceptionStatusUnknown, "알수없음"},
		{"접수예정", ReceptionStatusPlanned, "접수예정"},
		{"접수가능", ReceptionStatusPossible, "접수가능"},
		{"접수마감", ReceptionStatusClosed, "접수마감"},
		{"대기신청", ReceptionStatusStandBy, "대기신청"},
		{"방문상담", ReceptionStatusOnsiteConsultation, "방문상담"},
		{"방문선착순", ReceptionStatusOnsiteFCFS, "방문선착순"},
		{"현장문의", ReceptionStatusOnsiteInquiry, "현장문의"},
		{"전화문의", ReceptionStatusPhoneInquiry, "전화문의"},
		{"당일참여", ReceptionStatusWalkIn, "당일참여"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.status) >= len(ReceptionStatusStrings) {
				t.Fatalf("상태 인덱스(%d)가 배열 크기를 초과합니다", tt.status)
			}

			actual := ReceptionStatusStrings[tt.status]
			if actual != tt.expected {
				t.Errorf("매핑 오류: 기대값 = %q, 실제값 = %q", tt.expected, actual)
			}
		})
	}
}

// TestReceptionStatusMax_IndexMatch는 상태값 개수 상수(ReceptionStatusMax)가
// 실제 문자열 배열의 길이와 정확히 일치하는지 방어적으로 검증합니다.
func TestReceptionStatusMax_IndexMatch(t *testing.T) {
	expectedLen := int(ReceptionStatusMax)
	actualLen := len(ReceptionStatusStrings)

	if expectedLen != actualLen {
		t.Errorf("배열 구멍 또는 상수 누락 방지 규칙 위반: ReceptionStatusMax(%d) != len(ReceptionStatusStrings)(%d)", expectedLen, actualLen)
	}
}
