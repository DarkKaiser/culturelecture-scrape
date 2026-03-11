package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	// t.TempDir()을 사용하면 테스트 종료 시 자동으로 임시 디렉토리와 파일이 깔끔하게 삭제됩니다.
	tempDir := t.TempDir()

	// 전문가 수준의 Go 테스트 관례인 Table-Driven Test(테이블 기반 테스트) 기법을 사용합니다.
	tests := []struct {
		name        string
		yamlContent string
		fileName    string // 명시적으로 존재하지 않는 파일명 등을 테스트할 때 사용
		wantErr     bool
		errContains string
		validate    func(t *testing.T, cfg *Config)
	}{
		{
			name: "성공: 모든 설정값이 정상적으로 로드됨",
			yamlContent: `
search_year: "2026"
search_season: "봄"
student:
  birth_year: 2016
  birth_month: 5
  birth_day: 15
holidays:
  - "2026-05-05"
  - "2026-09-28"
providers:
  emart:
    auth_token: "test-token-123"
  homeplus:
    store_code: "100"
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.SearchYear != "2026" {
					t.Errorf("기댓값 SearchYear='2026', 실젯값='%s'", cfg.SearchYear)
				}
				if cfg.SearchSeason != "봄" {
					t.Errorf("기댓값 SearchSeason='봄', 실젯값='%s'", cfg.SearchSeason)
				}
				if cfg.Student.BirthYear != 2016 {
					t.Errorf("기댓값 Student.BirthYear=2016, 실젯값=%d", cfg.Student.BirthYear)
				}
				if len(cfg.Holidays) != 2 || cfg.Holidays[0] != "2026-05-05" {
					t.Errorf("Holidays 배열이 기대한 값과 다름")
				}
				if token, ok := cfg.Providers["emart"]["auth_token"]; !ok || token != "test-token-123" {
					t.Errorf("기댓값 emart auth_token='test-token-123', 실젯값='%s'", token)
				}
			},
		},
		{
			name: "실패: 검색연도(search_year) 필수값 누락",
			yamlContent: `
search_season: "여름"
student:
  birth_year: 2016
`,
			wantErr:     true,
			errContains: "필수 파라미터인 검색연도(search_year) 및 검색시즌(search_season) 값이 누락",
		},
		{
			name: "실패: 검색시즌(search_season) 필수값 누락",
			yamlContent: `
search_year: "2026"
student:
  birth_year: 2016
`,
			wantErr:     true,
			errContains: "필수 파라미터인 검색연도(search_year) 및 검색시즌(search_season) 값이 누락",
		},
		{
			name: "실패: 잘못된 YAML 형식 (파싱 오류)",
			yamlContent: `
search_year: 2026
	search_season: "가을" # 잘못된 들여쓰기(탭 혼용 등)
:invalid_yaml_format
`,
			wantErr:     true,
			errContains: "설정 파싱 오류",
		},
		{
			name:        "실패: 파일이 존재하지 않음 거나 권한 없음",
			fileName:    "not_exist_file_for_test.yaml", // 임의의 가짜 파일명
			wantErr:     true,
			errContains: "설정 로드 오류",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := ""

			if tt.fileName != "" {
				// 명시적으로 세팅된 파일명 (예: 존재하지 않는 파일 테스트용)
				filePath = filepath.Join(tempDir, tt.fileName)
			} else {
				// 각 테스트 케이스마다 고유한 임시 YAML 파일 생성
				f, err := os.CreateTemp(tempDir, "config-test-*.yaml")
				if err != nil {
					t.Fatalf("임시 설정 파일 생성 실패: %v", err)
				}
				defer f.Close()

				_, err = f.WriteString(tt.yamlContent)
				if err != nil {
					t.Fatalf("임시 설정 파일 쓰기 실패: %v", err)
				}
				filePath = f.Name()
			}

			// (실행) Config 로드 함수 호출
			cfg, err := Load(filePath)

			// (검증 1) 에러 유무 확인
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}

			// (검증 2) 에러가 있다면 기대한 에러 메시지(SubString)가 포함되었는지 확인
			if tt.wantErr && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Load() 에러 메시지 검증 실패\nWant contain: %v\nGot: %v", tt.errContains, err.Error())
				}
			}

			// (검증 3) 통과 케이스의 경우 반환된 Config 내부 필드 세부 검증
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}
