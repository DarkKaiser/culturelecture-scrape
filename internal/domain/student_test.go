package domain

import (
	"testing"
	"time"
)

func TestStudent_CalculateAge(t *testing.T) {
	// 구조체명 변경에 맞게 변현된 시나리오 기반 테스트 정의
	tests := []struct {
		name    string
		student Student
		now     time.Time
		wantAge int
	}{
		{
			name:    "기본 나이 계산 (해당 연도)",
			student: Student{BirthYear: 2016, BirthMonth: 5, BirthDay: 15},
			now:     time.Date(2026, 8, 10, 0, 0, 0, 0, time.Local), // 생일 지남
			wantAge: 11,                                             // 2026 - 2016 + 1 = 11 (한국식 나이)
		},
		{
			name:    "기본 나이 계산 (해당 연도 생일 이전)",
			student: Student{BirthYear: 2016, BirthMonth: 5, BirthDay: 15},
			now:     time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local), // 생일 안 지남 (한국식 나이는 생일 무관)
			wantAge: 11,
		},
		{
			name:    "태어난 당해",
			student: Student{BirthYear: 2026, BirthMonth: 1, BirthDay: 1},
			now:     time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local),
			wantAge: 1, // 태어난 해는 항상 1살
		},
		{
			name:    "태어나기 이전 시점으로 조사 (비정상 입력 대비 확인)",
			student: Student{BirthYear: 2026, BirthMonth: 5, BirthDay: 15},
			now:     time.Date(2025, 12, 31, 0, 0, 0, 0, time.Local),
			wantAge: 0, // 2025 - 2026 + 1 = 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.student.CalculateAge(tt.now); got != tt.wantAge {
				t.Errorf("CalculateAge() = %v, want %v", got, tt.wantAge)
			}
		})
	}
}

func TestStudent_CalculateMonths(t *testing.T) {
	// 달(Month) 계산의 핵심인 에지 케이스(월말, 윤년 등)를 집중적으로 검증합니다.
	tests := []struct {
		name       string
		student    Student
		now        time.Time
		wantMonths int
	}{
		{
			name:       "단순 개월 계산: 태어난지 정확히 1개월 지난 날",
			student:    Student{BirthYear: 2025, BirthMonth: 3, BirthDay: 10},
			now:        time.Date(2025, 4, 10, 0, 0, 0, 0, time.Local),
			wantMonths: 1,
		},
		{
			name:       "단순 개월 계산: 태어난지 1개월 되기 전날",
			student:    Student{BirthYear: 2025, BirthMonth: 3, BirthDay: 10},
			now:        time.Date(2025, 4, 9, 0, 0, 0, 0, time.Local),
			wantMonths: 0,
		},
		{
			name:       "연도 변경 포함: 한 해를 넘어감",
			student:    Student{BirthYear: 2025, BirthMonth: 12, BirthDay: 15},
			now:        time.Date(2026, 2, 15, 0, 0, 0, 0, time.Local),
			wantMonths: 2,
		},
		{
			name:    "에지 케이스 (월말 처리): 1월 31일생의 2월 28일",
			student: Student{BirthYear: 2025, BirthMonth: 1, BirthDay: 31},
			// time 패키지의 AddDate(0, 1, 0) 특성상 1월 31일 + 1개월 = 3월 3일로 계산됨 (내부 동작 구조)
			// 작성자의 의도(birthday.Unix() > now.Unix()) 구현에 맞춘 기댓값 검증
			now:        time.Date(2025, 2, 28, 0, 0, 0, 0, time.Local),
			wantMonths: 0, // 3월 3일 > 2월 28일 이므로 0개월 반환
		},
		{
			name:       "에지 케이스 (월말 처리): 1월 31일생의 3월 3일 (1개월 충족 기준일 확인)",
			student:    Student{BirthYear: 2025, BirthMonth: 1, BirthDay: 31},
			now:        time.Date(2025, 3, 3, 0, 0, 0, 0, time.Local), // time.AddDate()가 3/3을 한달로 잡는 시점
			wantMonths: 1,
		},
		{
			name:       "에지 케이스 (윤년 처리): 윤년 2월 29일생의 이듬해 평년 2월 28일",
			student:    Student{BirthYear: 2024, BirthMonth: 2, BirthDay: 29},
			now:        time.Date(2025, 2, 28, 0, 0, 0, 0, time.Local),
			wantMonths: 11, // 2025년 3월 1일이 되어야 정확히 12개월(1년)로 계산됨 (time.AddDate 규칙)
		},
		{
			name:       "에지 케이스 (윤년 처리): 윤년 2월 29일생의 이듬해 평년 3월 1일",
			student:    Student{BirthYear: 2024, BirthMonth: 2, BirthDay: 29},
			now:        time.Date(2025, 3, 1, 0, 0, 0, 0, time.Local),
			wantMonths: 12,
		},
		{
			name:       "비정상 케이스: 출생일 이전의 시간으로 조회",
			student:    Student{BirthYear: 2026, BirthMonth: 5, BirthDay: 15},
			now:        time.Date(2025, 12, 31, 0, 0, 0, 0, time.Local),
			wantMonths: 0, // 반복문 첫 시작부터 birthday(6월 15일) > now 이므로 0 반환
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.student.CalculateMonths(tt.now); got != tt.wantMonths {
				t.Errorf("CalculateMonths() = %v, want %v", got, tt.wantMonths)
			}
		})
	}
}
