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

func TestRun_ErrorCases(t *testing.T) {
	// 임시 디렉터리를 생성하여 테스트용 설정 파일들을 생성합니다.
	tempDir := t.TempDir()

	// 1. 정상 포맷이지만 필수값이 누락된 설정 파일 (검색 연도 누락, 공백 문자)
	missingYearConf := filepath.Join(tempDir, "missing_year.yaml")
	createTempConfig(t, missingYearConf, `
search_year: "  "
search_season: "여름"
student:
  birth_year: 2020
  birth_month: 5
`)

	// 2. 검색 시즌 텍스트가 잘못된 설정 파일
	invalidSeasonConf := filepath.Join(tempDir, "invalid_season.yaml")
	createTempConfig(t, invalidSeasonConf, `
search_year: "2024"
search_season: "초여름"
student:
  birth_year: 2020
  birth_month: 5
`)

	// 3. Provider 설정이 잘못되어 수집기 생성 시 에러가 나는 설정 파일 (에마트 토큰 부재 등은 경고일 뿐 에러가 아니므로, 구조적 오류 등 다른 방식 유도 필요)
	// 하지만 현재 코드 구조상 Provider New 함수들은 대부분 정상 구성됩니다.
	// 대신 YAML 파싱 자체가 안되는 완전 텍스트 쓰레기 파일을 만듭니다.
	invalidYamlConf := filepath.Join(tempDir, "invalid.yaml")
	createTempConfig(t, invalidYamlConf, `
	search_year: 2024
	---
	;;; invalid yaml
	`)

	tests := []struct {
		name          string
		args          []string
		wantErrPrefix string
	}{
		{
			name:          "명령줄_인자_파싱_오류",
			args:          []string{"-unknown-flag=true"},
			wantErrPrefix: "애플리케이션 실행 매개변수를 해석하는 과정에서 오류가 발생했습니다",
		},
		{
			name:          "존재하지_않는_설정파일",
			args:          []string{"-config", filepath.Join(tempDir, "not_found.yaml")},
			wantErrPrefix: "애플리케이션 설정 초기화 중 치명적인 오류가 발생했습니다",
		},
		{
			name:          "잘못된_YAML_포맷",
			args:          []string{"-config", invalidYamlConf},
			wantErrPrefix: "애플리케이션 설정 초기화 중 치명적인 오류가 발생했습니다", // config.Load 내부 에러
		},
		{
			name:          "필수값_누락_빈문자열",
			args:          []string{"-config", missingYearConf},
			wantErrPrefix: "검증 오류: 검색연도 및 검색시즌은 공백으로만 구성될 수 없습니다",
		},
		{
			name:          "잘못된_검색시즌_매핑_오류",
			args:          []string{"-config", invalidSeasonConf},
			wantErrPrefix: "검증 오류: 유효하지 않은 검색시즌입니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			if err == nil {
				t.Fatalf("run() 예상 에러가 발생하지 않음 (기대 에러 접두사: %q)", tt.wantErrPrefix)
			}

			if !strings.HasPrefix(err.Error(), tt.wantErrPrefix) {
				t.Errorf("run() 에러 메시지 불일치:\ngot:  %v\nwant prefix: %v", err.Error(), tt.wantErrPrefix)
			}
		})
	}
}

// ------------------------------------------------------------------
// Mock Scraper 정의 (성공 및 수집기 에러 케이스 테스트용)
// ------------------------------------------------------------------

type mockScraper struct {
	name        string
	shouldError bool
	lectures    []domain.Lecture
}

func (m *mockScraper) Name() string { return m.name }

func (m *mockScraper) Validate(ctx context.Context) error { return nil }

func (m *mockScraper) ScrapeCultureLectures(ctx context.Context) ([]domain.Lecture, error) {
	if m.shouldError {
		return nil, fmt.Errorf("모의 스크래퍼 %q에서 인위적 에러 발생", m.name)
	}
	return m.lectures, nil
}

func TestRun_SuccessCases(t *testing.T) {
	tempDir := t.TempDir()

	validConf := filepath.Join(tempDir, "valid.yaml")
	createTempConfig(t, validConf, `
search_year: "2024"
search_season: "가을"
student:
  birth_year: 2020
  birth_month: 5
providers:
  emart:
    auth_token: "test-token"
`)

	mockH := &mockScraper{
		name: "MockHomeplus",
		lectures: []domain.Lecture{
			{Title: "홈플러스 모의 강좌"},
		},
	}

	args := []string{"-config", validConf}
	err := run(args, mockH)
	if err != nil {
		t.Fatalf("run() 예상치 못한 에러 발생: %v", err)
	}
}

func TestRun_ScrapeError(t *testing.T) {
	tempDir := t.TempDir()

	validConf := filepath.Join(tempDir, "valid_for_err.yaml")
	createTempConfig(t, validConf, `
search_year: "2024"
search_season: "가을"
student:
  birth_year: 2020
  birth_month: 5
`)

	mockErrP := &mockScraper{
		name:        "MockErrorProvider",
		shouldError: true,
	}

	args := []string{"-config", validConf}
	// 에러 뱉는 mock 스크래퍼를 주입하여, run() 내부의 scraper.Scrape 에러 검증
	err := run(args, mockErrP)
	if err == nil {
		t.Fatal("run() 수집기 에러가 발생했으나 에러를 리턴하지 않음")
	}
	if !strings.Contains(err.Error(), "일부 수집기에서 치명적인 오류가 발생하여 전체 수집 프로세스를 안전하게 종료합니다") {
		t.Errorf("run() 비정상 에러 메시지: %v", err)
	}
}

// createTempConfig 테스트를 위한 임의의 설정 파일을 생성하는 헬퍼 함수
func createTempConfig(t *testing.T, path, content string) {
	t.Helper()
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("테스트 설정 파일 생성 실패: %v", err)
	}
}
