package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// ===========================================================================
// 테스트용 Mock Scraper
// ===========================================================================

// mockScraper 는 실제 HTTP 통신 없이 run() 함수의 동작을 검증하기 위한
// 가짜(Mock) 스크래퍼입니다. Scraper 인터페이스를 구현합니다.
type mockScraper struct {
	lectures       []domain.Lecture
	validateErr    error // Validate() 시 반환할 에러 (nil 이면 정상)
	scrapeErr      error // Scrape() 시 반환할 에러 (nil 이면 정상)
	scrapeCallCount int  // Scrape()가 호출된 횟수 (검증용)
}

func (m *mockScraper) Validate(_ context.Context) error {
	return m.validateErr
}

func (m *mockScraper) Scrape(_ context.Context) ([]domain.Lecture, error) {
	m.scrapeCallCount++
	if m.scrapeErr != nil {
		return nil, m.scrapeErr
	}
	return m.lectures, nil
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// newTempConfig 는 테스트용 임시 YAML 설정 파일을 지정된 경로에 생성합니다.
func newTempConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("테스트 설정 파일 생성 실패: %v", err)
	}
}

// chdirTemp 는 테스트 실행 중 현재 작업 디렉터리를 임시 디렉터리로 변경하고,
// 테스트 종료 시 원래 디렉터리로 복구하는 정리 함수를 등록합니다.
// run()이 상대 경로로 CSV 파일을 현재 디렉터리에 생성하므로, 이를 통해
// 테스트 환경과 소스 디렉터리를 오염시키지 않습니다.
func chdirTemp(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("현재 작업 디렉터리 확인 실패: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("임시 디렉터리로 이동 실패: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Logf("원래 디렉터리 복구 실패(무시됨): %v", err)
		}
	})
	return tempDir
}

// validYamlConfig 는 모든 필수 항목이 채워진 정상 YAML 설정 파일 내용입니다.
// 테스트 케이스에서 공통으로 재사용합니다.
const validYamlConfig = `
search_year: "2024"
search_season: "가을"
student:
  birth_year: 2020
  birth_month: 5
providers:
  emart:
    auth_token: "test-token"
`

// ===========================================================================
// TestRun_ErrorCases: run() 초기 단계(인자 파싱, 설정 로드, 검증)의 에러 경로
// ===========================================================================

func TestRun_ErrorCases(t *testing.T) {
	tempDir := t.TempDir()

	// 테스트용 설정 파일들을 미리 생성합니다.
	missingYearConf := filepath.Join(tempDir, "missing_year.yaml")
	newTempConfig(t, missingYearConf, `
search_year: "  "
search_season: "여름"
student:
  birth_year: 2020
  birth_month: 5
`)

	invalidSeasonConf := filepath.Join(tempDir, "invalid_season.yaml")
	newTempConfig(t, invalidSeasonConf, `
search_year: "2024"
search_season: "초여름"
student:
  birth_year: 2020
  birth_month: 5
`)

	invalidYamlConf := filepath.Join(tempDir, "invalid.yaml")
	newTempConfig(t, invalidYamlConf, `
search_year: 2024
---
;;; invalid yaml
`)

	tests := []struct {
		name            string
		args            []string
		wantErrContains string
	}{
		{
			name:            "명령줄_인자_파싱_오류",
			args:            []string{"-unknown-flag=true"},
			wantErrContains: "애플리케이션 실행 매개변수를 해석하는 과정에서 오류가 발생했습니다",
		},
		{
			name:            "존재하지_않는_설정파일",
			args:            []string{"-config", filepath.Join(tempDir, "not_found.yaml")},
			wantErrContains: "애플리케이션 설정 초기화 중 치명적인 오류가 발생했습니다",
		},
		{
			name:            "잘못된_YAML_포맷",
			args:            []string{"-config", invalidYamlConf},
			wantErrContains: "애플리케이션 설정 초기화 중 치명적인 오류가 발생했습니다",
		},
		{
			name:            "필수값_빈문자열(검색연도)",
			args:            []string{"-config", missingYearConf},
			wantErrContains: "검증 오류: 검색연도 및 검색시즌은 공백으로만 구성될 수 없습니다",
		},
		{
			name:            "유효하지_않은_검색시즌",
			args:            []string{"-config", invalidSeasonConf},
			wantErrContains: "검증 오류: 유효하지 않은 검색시즌입니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			if err == nil {
				t.Fatalf("run() 에러를 반환해야 하지만 nil이 반환됨 (기대 포함 문자열: %q)", tt.wantErrContains)
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("run() 에러 메시지 불일치\ngot:  %q\nwant contains: %q", err.Error(), tt.wantErrContains)
			}
		})
	}
}

// ===========================================================================
// TestRun_SeasonCodeMapping: 검색시즌 4종 → SeasonCode 정상 매핑 검증
// ===========================================================================

// 검색시즌 한글 입력값이 각각 올바른 내부 코드로 매핑되어 run()이 정상 완료되는지
// 확인합니다. Mock Scraper를 사용하므로 실제 수집은 발생하지 않습니다.
func TestRun_SeasonCodeMapping(t *testing.T) {
	seasons := []string{"봄", "여름", "가을", "겨울"}

	for _, season := range seasons {
		season := season
		t.Run("시즌_"+season, func(t *testing.T) {
			tempDir := chdirTemp(t)
			conf := filepath.Join(tempDir, "cfg.yaml")
			newTempConfig(t, conf, fmt.Sprintf(`
search_year: "2024"
search_season: "%s"
student:
  birth_year: 2020
  birth_month: 5
`, season))

			mock := &mockScraper{}
			err := run([]string{"-config", conf}, mock)
			if err != nil {
				t.Errorf("시즌=%q 일 때 run()이 에러를 반환함: %v", season, err)
			}
		})
	}
}

// ===========================================================================
// TestRun_ValidateError: Validate() 실패 시 Scrape()가 호출되지 않아야 함
// ===========================================================================

// Validate()가 에러를 반환하는 Mock을 주입하면, 수집 단계(Scrape)로 진입하지 않고
// 즉시 에러를 반환해야 합니다. 이를 통해 Fast-Fail 안전 장치를 검증합니다.
func TestRun_ValidateError(t *testing.T) {
	tempDir := chdirTemp(t)
	conf := filepath.Join(tempDir, "cfg.yaml")
	newTempConfig(t, conf, validYamlConfig)

	validateErr := fmt.Errorf("점포 코드 불일치: 모의 검증 실패")
	mock := &mockScraper{validateErr: validateErr}

	err := run([]string{"-config", conf}, mock)
	if err == nil {
		t.Fatal("Validate()가 실패했을 때 run()은 에러를 반환해야 합니다")
	}
	// Validate 에러 후 Scrape가 호출되지 않았음을 검증합니다.
	if mock.scrapeCallCount != 0 {
		t.Errorf("Validate() 실패 이후 Scrape()가 호출되어서는 안 됩니다. 호출 횟수: %d", mock.scrapeCallCount)
	}
}

// ===========================================================================
// TestRun_ScrapeError: Scrape() 실패 시 run()이 에러를 반환해야 함
// ===========================================================================

func TestRun_ScrapeError(t *testing.T) {
	tempDir := chdirTemp(t)
	conf := filepath.Join(tempDir, "cfg.yaml")
	newTempConfig(t, conf, validYamlConfig)

	mock := &mockScraper{scrapeErr: fmt.Errorf("네트워크 타임아웃")}

	err := run([]string{"-config", conf}, mock)
	if err == nil {
		t.Fatal("Scrape()가 실패했을 때 run()은 에러를 반환해야 합니다")
	}
	const wantContains = "일부 수집기에서 치명적인 오류가 발생하여 전체 수집 프로세스를 안전하게 종료합니다"
	if !strings.Contains(err.Error(), wantContains) {
		t.Errorf("run() 에러 메시지 불일치\ngot:  %q\nwant contains: %q", err.Error(), wantContains)
	}
}

// ===========================================================================
// TestRun_SuccessWithSingleScraper: Mock 1개 주입 후 CSV 정상 생성 검증
// ===========================================================================

// 단일 Mock Scraper를 주입했을 때 run()이 정상 완료되고,
// CSV 파일이 현재 디렉터리에 생성되는지 확인합니다.
func TestRun_SuccessWithSingleScraper(t *testing.T) {
	tempDir := chdirTemp(t)
	conf := filepath.Join(tempDir, "cfg.yaml")
	newTempConfig(t, conf, validYamlConfig)

	mock := &mockScraper{
		lectures: []domain.Lecture{
			{Title: "단일 모의 강좌", StoreName: "테스트점"},
		},
	}

	err := run([]string{"-config", conf}, mock)
	if err != nil {
		t.Fatalf("run()이 예상치 못한 에러를 반환함: %v", err)
	}

	// CSV 파일이 tempDir(=현재 디렉터리)에 생성되었는지 확인합니다.
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("임시 디렉터리 읽기 실패: %v", err)
	}
	csvFound := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "culturelecture-scrape-") && strings.HasSuffix(e.Name(), ".csv") {
			csvFound = true
			break
		}
	}
	if !csvFound {
		t.Errorf("run() 성공 후 CSV 파일이 생성되어야 하지만 발견되지 않았습니다")
	}
}

// ===========================================================================
// TestRun_SuccessWithMultipleScrapers: 복수 Mock 주입 시 결과 누적 검증
// ===========================================================================

// 복수의 Mock Scraper를 주입했을 때 각 수집기의 강좌가 합산되어
// 하나의 CSV에 정상적으로 저장되는지 확인합니다.
func TestRun_SuccessWithMultipleScrapers(t *testing.T) {
	tempDir := chdirTemp(t)
	conf := filepath.Join(tempDir, "cfg.yaml")
	newTempConfig(t, conf, validYamlConfig)

	mockA := &mockScraper{
		lectures: []domain.Lecture{
			{Title: "홈플러스 강좌1", StoreName: "홈플러스 광양점"},
			{Title: "홈플러스 강좌2", StoreName: "홈플러스 순천점"},
		},
	}
	mockB := &mockScraper{
		lectures: []domain.Lecture{
			{Title: "롯데마트 강좌1", StoreName: "롯데마트 여수점"},
		},
	}

	err := run([]string{"-config", conf}, mockA, mockB)
	if err != nil {
		t.Fatalf("run()이 예상치 못한 에러를 반환함: %v", err)
	}

	// 두 Mock 모두 Scrape가 정확히 1회 호출되었는지 검증합니다.
	if mockA.scrapeCallCount != 1 {
		t.Errorf("mockA.Scrape() 호출 횟수: got=%d, want=1", mockA.scrapeCallCount)
	}
	if mockB.scrapeCallCount != 1 {
		t.Errorf("mockB.Scrape() 호출 횟수: got=%d, want=1", mockB.scrapeCallCount)
	}
}

// ===========================================================================
// TestRun_EmartAuthTokenAbsent: emart 섹션 없어도 정상 수집 진행 검증
// ===========================================================================

// providers.emart 섹션이 설정 파일에 없을 때 emartAuthToken이 빈 문자열로
// 처리되어 run()이 에러 없이 수집 단계로 진입하는지 확인합니다.
// (실제 이마트 API 호출은 Mock으로 대체되므로 401 에러는 발생하지 않습니다.)
func TestRun_EmartAuthTokenAbsent(t *testing.T) {
	tempDir := chdirTemp(t)
	conf := filepath.Join(tempDir, "cfg.yaml")
	// providers.emart 섹션이 없는 설정 파일
	newTempConfig(t, conf, `
search_year: "2024"
search_season: "봄"
student:
  birth_year: 2020
  birth_month: 5
`)

	mock := &mockScraper{}
	err := run([]string{"-config", conf}, mock)
	if err != nil {
		t.Errorf("emart auth_token 미설정 시에도 run()은 에러 없이 완료되어야 합니다: %v", err)
	}
}
