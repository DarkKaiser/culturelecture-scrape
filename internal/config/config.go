package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// Config YAML 설정 파일에서 읽어 온 프로그램 전체 설정을 담는 구조체입니다.
type Config struct {
	// 수집 대상 연도 (예: "2026")
	SearchYear string `yaml:"search_year"`

	// 수집 대상 시즌 (봄/여름/가을/겨울)
	SearchSeason string `yaml:"search_season"`

	// 공휴일 목록 (YYYY-MM-DD 형식). 필터링 시 해당 날짜 강좌를 제외하는 데 사용됩니다.
	Holidays []string `yaml:"holidays"`

	// Provider별 추가 설정을 담는 중첩 맵입니다.
	Providers map[string]map[string]string `yaml:"providers"`

	// 나이 필터링에 사용할 수강생(아이) 기본 정보입니다.
	Lecturer domain.Lecturer `yaml:"lecturer"`
}

// Load 지정된 경로의 YAML 설정 파일을 읽어 Config 구조체로 파싱한 뒤 반환합니다.
func Load(path string) (*Config, error) {
	// 설정 파일 전체를 메모리로 읽어 들입니다.
	// 파일이 없거나 읽기 권한이 없으면 즉시 에러를 반환합니다.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("설정 로드 오류: 지정된 경로(%s)에서 파일을 읽을 수 없습니다. (파일 부재 또는 권한 부족) - %w", path, err)
	}

	// YAML 바이트 슬라이스를 Config 구조체로 역직렬화합니다.
	cfg := &Config{}
	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("설정 파싱 오류: YAML 형식이 올바르지 않습니다. (구문 오류 또는 타입 불일치) - %w", err)
	}

	// 프로그램 동작에 반드시 필요한 최소 설정값을 검증합니다.
	if cfg.SearchYear == "" || cfg.SearchSeason == "" {
		return nil, fmt.Errorf("설정 초기화 오류: 필수 파라미터인 검색연도(search_year) 및 검색시즌(search_season) 값이 누락되었습니다")
	}

	return cfg, nil
}
