package filter

import (
	"math"
	"testing"
	"time"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

func TestExtractAgeRange(t *testing.T) {
	now := time.Now()
	currentYear := now.Year()

	tests := []struct {
		name       string
		title      string
		wantUnit   ageUnit
		wantMinAge int
		wantMaxAge int
		wantErr    bool
	}{
		// ------------------------------------------------------------------
		// [패턴 그룹 1] N세/개월 이상, N세/개월~성인
		// ------------------------------------------------------------------
		{"Group1_Year_Over", "재미있는 미술교실 (7세 이상)", ageUnitYear, 7, math.MaxInt32, false},
		{"Group1_Month_Over", "오감발달 놀이 (24개월 이상)", ageUnitMonth, 24, math.MaxInt32, false},
		{"Group1_To_Adult", "코딩 기초 (5세~성인)", ageUnitYear, 5, math.MaxInt32, false},
		{"Group1_To_Anyone", "가족 요리 (24개월~누구나)", ageUnitMonth, 24, math.MaxInt32, false},

		// ------------------------------------------------------------------
		// [패턴 그룹 2] 양쪽 경계값이 있는 범위 표기
		// ------------------------------------------------------------------
		{"Group2_Year_Range_Tilde", "어린이 수영 (5~10세)", ageUnitYear, 5, 10, false},
		{"Group2_Year_Range_Hyphen", "종이접기 (3세-7세)", ageUnitYear, 3, 7, false},
		{"Group2_Month_Range", "촉감 놀이 (6~24개월)", ageUnitMonth, 6, 24, false},
		{"Group2_Year_Range_Reverse", "순서가 바뀐 경우 (10~5세)", ageUnitYear, 5, 10, false},

		// ------------------------------------------------------------------
		// [패턴 그룹 3] 초등 상한 (만 7세 입학, 만 13세 상한 적용)
		// ------------------------------------------------------------------
		{"Group3_Year_To_Elem", "창의 수학 (5세~초등)", ageUnitYear, 5, 13, false},
		{"Group3_Month_To_Elem", "영재 놀이 (24개월~초등)", ageUnitMonth, 24, 156, false}, // 13 * 12 = 156개월

		// ------------------------------------------------------------------
		// [패턴 그룹 4] 특정 초등 학년 상한
		// ------------------------------------------------------------------
		{"Group4_Year_To_Grade3", "키즈 스피치 (5세~초3)", ageUnitYear, 5, 10, false}, // 3 + 7 = 10세
		{"Group4_Year_To_Grade6", "논술 교실 (5세~초6)", ageUnitYear, 5, 13, false}, // 6 + 7 = 13세

		// ------------------------------------------------------------------
		// [패턴 그룹 5] 괄호 안 단일 연령
		// ------------------------------------------------------------------
		{"Group5_Year_Exact", "일일 특강 (7세)", ageUnitYear, 7, 7, false},
		{"Group5_Month_Exact", "베이비 마사지 (24개월)", ageUnitMonth, 24, 24, false},

		// ------------------------------------------------------------------
		// [고정 패턴 1] 초등 학년 범위
		// ------------------------------------------------------------------
		{"Fixed1_Grade_Range", "초등 과학교실 (초1~초3)", ageUnitYear, 8, 10, false}, // 1+7=8, 3+7=10
		{"Fixed1_Grade_Range_Hyphen", "역사 교실 (초2-초6)", ageUnitYear, 9, 13, false}, // 2+7=9, 6+7=13
		{"Fixed1_Grade_Range_Reverse", "거꾸로 학년 (초6-초2)", ageUnitYear, 9, 13, false},

		// ------------------------------------------------------------------
		// [고정 패턴 2] 4자리 출생연도 범위 (동적 계산)
		// ------------------------------------------------------------------
		{
			name:       "Fixed2_Year4_Range",
			title:      "수리 논술 (2010~2015년생)",
			wantUnit:   ageUnitYear,
			wantMinAge: currentYear - 2015 + 1, // 더 늦게 태어난(나이가 적은) 쪽이 minAge
			wantMaxAge: currentYear - 2010 + 1, // 더 먼저 태어난(나이가 많은) 쪽이 maxAge
			wantErr:    false,
		},

		// ------------------------------------------------------------------
		// [고정 패턴 3] 4자리 + 2자리 출생연도 범위 (동적 계산)
		// ------------------------------------------------------------------
		{
			name:       "Fixed3_Year4_Year2",
			title:      "어린이 경제 (2012년~18년생)",
			wantUnit:   ageUnitYear,
			wantMinAge: currentYear - 2018 + 1,
			wantMaxAge: currentYear - 2012 + 1,
			wantErr:    false,
		},

		// ------------------------------------------------------------------
		// [고정 패턴 4] 2자리 출생연도 범위 (동적 계산)
		// ------------------------------------------------------------------
		{
			name:       "Fixed4_Year2_Range",
			title:      "주산 암산 (14~18년생)",
			wantUnit:   ageUnitYear,
			wantMinAge: currentYear - 2018 + 1,
			wantMaxAge: currentYear - 2014 + 1,
			wantErr:    false,
		},

		// ------------------------------------------------------------------
		// [고정 패턴 5, 6] 연도 하한 표기 (상한 없음)
		// ------------------------------------------------------------------
		{
			name:       "Fixed5_Year4_Over",
			title:      "성장 요가 (2015년생 이상)",
			wantUnit:   ageUnitYear,
			wantMinAge: currentYear - 2015 + 1,
			wantMaxAge: math.MaxInt32,
			wantErr:    false,
		},
		{
			name:       "Fixed6_Year2_Over",
			title:      "키성장 체조 (15년생 이상)",
			wantUnit:   ageUnitYear,
			wantMinAge: currentYear - 2015 + 1,
			wantMaxAge: math.MaxInt32,
			wantErr:    false,
		},

		// ------------------------------------------------------------------
		// [고정 패턴 7] 성인 ~ 특정 출생연도
		// ------------------------------------------------------------------
		{
			name:       "Fixed7_Adult_To_Year4",
			title:      "가족 테니스 (성인~2015년생)",
			wantUnit:   ageUnitYear,
			wantMinAge: currentYear - 2015 + 1, // "성인"은 최하한값 무한에 가깝지만 로직상 명시된 숫자인 2015를 최소로 우선 잡음 (수정불요, 일관성)
			wantMaxAge: math.MaxInt32,
			wantErr:    false,
		},
		{
			name:       "Fixed8_Adult_To_Year2",
			title:      "월ㅣ강명헌의 통기타(성인~17년생) A ♥개강확정)",
			wantUnit:   ageUnitYear,
			wantMinAge: currentYear - 2017 + 1,
			wantMaxAge: math.MaxInt32,
			wantErr:    false,
		},
		{
			name:       "Fixed8_Adult_To_Year2_Short",
			title:      "월ㅣ강명헌의 통기타(성인~17년) A ♥개강확정)",
			wantUnit:   ageUnitYear,
			wantMinAge: currentYear - 2017 + 1,
			wantMaxAge: math.MaxInt32,
			wantErr:    false,
		},

		// ------------------------------------------------------------------
		// [고정 패턴 9, 10] 개월 ~ 출생연도 범위 (동적 계산)
		// ------------------------------------------------------------------
		{
			name:       "Fixed9_Month_To_Year4",
			title:      "재미있는 놀이 (36개월~2015년생)",
			wantUnit:   ageUnitMonth,
			wantMinAge: 36,
			wantMaxAge: (currentYear - 2015 + 1) * 12,
			wantErr:    false,
		},
		{
			name:       "Fixed10_Month_To_Year2",
			title:      "놀이체육 (36개월-14년생)",
			wantUnit:   ageUnitMonth,
			wantMinAge: 36,
			wantMaxAge: (currentYear - 2014 + 1) * 12,
			wantErr:    false,
		},

		// ------------------------------------------------------------------
		// [고정 패턴 11, 12] 나이(세) ~ 출생연도 범위 (동적 계산)
		// ------------------------------------------------------------------
		{
			name:       "Fixed11_Year_To_Year4",
			title:      "코딩 입문 (5세~2015년생)",
			wantUnit:   ageUnitYear,
			wantMinAge: 5,
			wantMaxAge: currentYear - 2015 + 1,
			wantErr:    false,
		},
		{
			name:       "Fixed12_Year_To_Year2",
			title:      "어린이 바둑 (5세-15년생)",
			wantUnit:   ageUnitYear,
			wantMinAge: 5,
			wantMaxAge: currentYear - 2015 + 1,
			wantErr:    false,
		},

		// ------------------------------------------------------------------
		// [특정 키워드 매핑]
		// ------------------------------------------------------------------
		{"Keyword_Elem", "파닉스 영어 (초등)", ageUnitYear, 8, 13, false},
		{"Keyword_AllAge", "가족 오케스트라 (모든 연령)", ageUnitYear, 0, math.MaxInt32, false},
		{"Keyword_Adult", "바리스타 자격증 (성인)", ageUnitYear, 20, math.MaxInt32, false},

		// ------------------------------------------------------------------
		// 아무것도 매칭되지 않는 경우
		// ------------------------------------------------------------------
		// 아무것도 매칭되지 않는 경우
		// ------------------------------------------------------------------
		{"No_Match_Empty", "", ageUnitUnknown, 0, math.MaxInt32, false},
		{"No_Match_Irrelevant", "연령 정보가 없는 일반 강좌입니다.", ageUnitUnknown, 0, math.MaxInt32, false},

		// ------------------------------------------------------------------
		// Atoi 파싱 에러 유도 케이스
		// (정규식은 통과하나, 추출해 낸 문자열이 Atoi가 허용하지 않는 범위를 넘어서 반환하는 케이스를 찾기 어렵기 때문에
		// 매우 한계적인 환경에서만 커버리지가 도달합니다. 아래 케이스로 최대한 유도합니다.)
		// * 현재 정규식 구조상(예: [0-9]{1,2}세) 정해진 자릿수만 매치되므로 
		// "정상적이지 않은 문자가 섞인 채로" MatchString을 통과시키기가 사실상 불가능합니다.
		// 즉 해당 하위 if err != nil 분기들은 `strconv.Atoi`에 숫자가 아닌 쓰레기값이 들어가야 타는 방어코드인데,
		// 상단의 `regexp` 패턴이 이미 순수 숫자만 걸러내도록 너무 강력하게 짜여 있어서 (Unreachable Code에 가까움)
		// 커버리지가 여기서 깎이는 현상입니다.
		// ------------------------------------------------------------------
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lecture := &domain.Lecture{
				Title:          tt.title,
				ScrapeExcluded: false,
			}

			gotUnit, gotMinAge, gotMaxAge, err := extractAgeRange(lecture)

			if (err != nil) != tt.wantErr {
				t.Fatalf("extractAgeRange() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotUnit != tt.wantUnit {
				t.Errorf("extractAgeRange() gotUnit = %v, want %v", gotUnit, tt.wantUnit)
			}
			if gotMinAge != tt.wantMinAge {
				t.Errorf("extractAgeRange() gotMinAge = %v, want %v", gotMinAge, tt.wantMinAge)
			}
			if gotMaxAge != tt.wantMaxAge {
				t.Errorf("extractAgeRange() gotMaxAge = %v, want %v", gotMaxAge, tt.wantMaxAge)
			}
		})
	}
}

func TestAgeLimitRule_IsExcluded(t *testing.T) {
	tests := []struct {
		name          string
		title         string // 파싱될 강좌명
		studentAge    int    // 수강생의 나이
		studentMonths int    // 수강생의 개월 수
		wantExcluded  bool   // true면 제외됨(조건 불만족), false면 통과됨(조건 만족)
	}{
		// ------------------------------------------------------------------
		// 정상 수용 (경계값 포함 테스트) - ageUnitYear
		// ------------------------------------------------------------------
		{"Year_Within_Range1", "어린이반 (7~9세)", 7, 0, false}, // 하한 경계
		{"Year_Within_Range2", "어린이반 (7~9세)", 8, 0, false}, // 범위 내
		{"Year_Within_Range3", "어린이반 (7~9세)", 9, 0, false}, // 상한 경계

		// ------------------------------------------------------------------
		// 제외 조건 (경계값 초과/미달) - ageUnitYear
		// ------------------------------------------------------------------
		{"Year_Under_Min", "어린이반 (7~9세)", 6, 0, true},  // 하한 미달
		{"Year_Over_Max", "어린이반 (7~9세)", 10, 0, true}, // 상한 초과

		// ------------------------------------------------------------------
		// 정상 수용 (경계값 포함 테스트) - ageUnitMonth
		// ------------------------------------------------------------------
		{"Month_Within_Range1", "유아반 (12~24개월)", 0, 12, false}, // 하한 경계
		{"Month_Within_Range2", "유아반 (12~24개월)", 0, 18, false}, // 범위 내
		{"Month_Within_Range3", "유아반 (12~24개월)", 0, 24, false}, // 상한 경계

		// ------------------------------------------------------------------
		// 제외 조건 (경계값 초과/미달) - ageUnitMonth
		// ------------------------------------------------------------------
		{"Month_Under_Min", "유아반 (12~24개월)", 0, 11, true}, // 하한 미달
		{"Month_Over_Max", "유아반 (12~24개월)", 0, 25, true}, // 상한 초과

		// ------------------------------------------------------------------
		// 연령 단위 미상인 경우 필터링 동작 확인 (무조건 수용)
		// ------------------------------------------------------------------
		{"Unknown_Unit", "연령 무관 강좌", 99, 999, false},

		// ------------------------------------------------------------------
		// 파싱 중 에러(Atoi 등) 발생 시 필터 동작 확인 (무조건 수용)
		// ------------------------------------------------------------------
		{"Parse_Error", "에러 발생 (99999999999999999999세 이상)", 99, 999, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := NewAgeLimitRule(tt.studentMonths, tt.studentAge)
			lecture := &domain.Lecture{Title: tt.title}

			got := rule.IsExcluded(lecture)

			if got != tt.wantExcluded {
				t.Errorf("IsExcluded() = %v, want %v (studentAge: %d, studentMonths: %d, title: %q)",
					got, tt.wantExcluded, tt.studentAge, tt.studentMonths, tt.title)
			}
		})
	}
}
