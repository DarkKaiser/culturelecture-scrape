package domain

import "time"

// Student 수강생(아이)의 기본 정보를 담는 구조체입니다.
type Student struct {
	BirthYear  int `yaml:"birth_year"`  // 태어난 년도 (예: 2016)
	BirthMonth int `yaml:"birth_month"` // 태어난 월 (1~12)
	BirthDay   int `yaml:"birth_day"`   // 태어난 일 (1~31)
}

// CalculateAge 수강생의 한국식 나이를 계산합니다.
func (s *Student) CalculateAge(now time.Time) int {
	return now.Year() - s.BirthYear + 1
}

// CalculateMonths 수강생의 생년월일을 기준으로 현재 개월 수를 계산합니다.
func (s *Student) CalculateMonths(now time.Time) int {
	months := 0

	// 생년월일을 time.Time으로 변환한다.
	birthday := time.Date(s.BirthYear, time.Month(s.BirthMonth), s.BirthDay, 0, 0, 0, 0, time.Local)

	// 생년월일에서 1개월씩 더해가며, now를 초과하면 반복을 멈춥니다.
	// 이 방식은 월별 마지막 날차 처리(예: 1월 31일 + 1M = 2월 28일)를 time 패키지에 위임해
	// DST/윤년 연도 등 에지 케이스를 안전하게 처리합니다.
	for {
		birthday = birthday.AddDate(0, 1, 0)
		if birthday.Unix() > now.Unix() {
			break
		}
		months++
	}

	return months
}
