package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/darkkaiser/culturelecture-scrape/internal/config"
	"github.com/darkkaiser/culturelecture-scrape/internal/exporter"
	"github.com/darkkaiser/culturelecture-scrape/internal/filter"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper/provider"
	"github.com/darkkaiser/notify-server/pkg/strutil"
)

const (
	// Version 애플리케이션의 현재 릴리스 버전을 나타냅니다.
	Version = "v0.0.2"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("프로그램 실행 중 치명적인 오류가 발생하여 애플리케이션을 안전하게 종료합니다. (상세 내역: %v)", err)
	}
}

func run(args []string, mockScrapers ...scraper.Scraper) error {
	// ------------------------------------------------------------------
	// 1단계: 실행 인자 파싱 및 설정 파일 로드
	// ------------------------------------------------------------------

	// -config 플래그로 설정 파일의 경로를 직접 지정할 수 있으며, 생략 시에는
	// 실행 파일과 같은 위치에 있는 culturelecture-scrape.yaml 파일을 자동으로 사용합니다.
	var configPath string
	fs := flag.NewFlagSet("culturelecture-scrape", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "culturelecture-scrape.yaml", "설정 파일 경로")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("애플리케이션 실행 매개변수를 해석하는 과정에서 오류가 발생했습니다: %w", err)
	}

	// 지정된 경로의 YAML 설정 파일을 읽어 Config 구조체를 생성합니다.
	// 파일이 없거나 필수 항목이 누락된 경우 더 이상 진행이 불가능하므로 즉시 에러를 반환합니다.
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("애플리케이션 설정 초기화 중 치명적인 오류가 발생했습니다: %v", err)
	}

	// 프로그램 시작 시 버전 정보와 제작자 정보를 출력합니다.
	fmt.Println("########################################################")
	fmt.Println("###                                                  ###")
	fmt.Printf("###           culturelecture-scrape %-17s###\n", Version)
	fmt.Println("###                                                  ###")
	fmt.Println("###                         developed by DarkKaiser  ###")
	fmt.Println("###                                                  ###")
	fmt.Println("########################################################")
	fmt.Println("")

	// ------------------------------------------------------------------
	// 2단계: 수집 조건 파악
	// ------------------------------------------------------------------

	now := time.Now()

	// 설정값에 실수로 포함된 앞뒤 공백 또는 중간 연속 공백을 정리합니다.
	// 공백이 섞이면 이후 문자열 비교나 API 호출에서 예상치 못한 오류가 발생할 수 있습니다.
	searchYear := strutil.NormalizeSpace(cfg.SearchYear)
	searchSeason := strutil.NormalizeSpace(cfg.SearchSeason)

	// 강좌 나이 제한 필터링을 위해 수강자의 현재 나이를 미리 계산해 둡니다.
	// 나이(Age)와 개월 수(Months)를 함께 계산하는 이유는, 강좌마다 적용 기준이 '만 N세'와 '생후 N개월' 두 가지로 혼재하기 때문입니다.
	studentAge := cfg.Student.CalculateAge(now)
	studentMonths := cfg.Student.CalculateMonths(now)

	log.Printf("▶ %s년 %s 문화센터 강좌를 수집합니다.", searchYear, searchSeason)
	log.Printf("▶ 문화센터 강좌 수강자는 %d세(%d개월) 아이입니다.", studentAge, studentMonths)

	// 공백만으로 이루어진 값은 유효하지 않으므로, 빈 값으로 판정되는 경우 즉시 에러를 반환합니다.
	if searchYear == "" || searchSeason == "" {
		return fmt.Errorf("검증 오류: 검색연도 및 검색시즌은 공백으로만 구성될 수 없습니다")
	}

	// 일부 API는 검색시즌을 한글이 아닌 숫자 코드로 요구합니다.(봄:1, 여름:2, 가을:3, 겨울:4)
	// 설정 파일에는 사람이 읽기 쉬운 한글로 입력받고, 여기서 각 API가 요구하는 숫자 코드로 변환합니다.
	var searchSeasonCode string
	switch searchSeason {
	case "봄":
		searchSeasonCode = "1"
	case "여름":
		searchSeasonCode = "2"
	case "가을":
		searchSeasonCode = "3"
	case "겨울":
		searchSeasonCode = "4"
	default:
		return fmt.Errorf("검증 오류: 유효하지 않은 검색시즌입니다. 허용되는 값: 봄, 여름, 가을, 겨울 (입력값: %q)", searchSeason)
	}

	// 이마트 문화센터 API는 AWS AppSync 기반으로, 모든 요청에 Bearer 인증 토큰이 필요합니다.
	// 유효 기간이 있어 토큰이 만료되면 설정 파일의 providers.emart.auth_token 값을 새 토큰으로 갱신해야 합니다.
	// 설정 파일에 emart 섹션 자체가 없으면 빈 문자열로 처리합니다.(401 에러 발생)
	emartAuthToken := ""
	if p, ok := cfg.Providers["emart"]; ok {
		emartAuthToken = p["auth_token"]
	}

	// 이후 수집기들을 생성할 때 필요한 수집 조건(검색 연도, 시즌, 토큰 등)들을 하나의 Config 객체로 모아둡니다.
	scraperCfg := scraper.Config{
		SearchYear:       searchYear,
		SearchSeason:     searchSeason,
		SearchSeasonCode: searchSeasonCode,
		EmartAuthToken:   emartAuthToken,
	}

	// ------------------------------------------------------------------
	// 3단계: 수집기(Scraper) 초기화
	// ------------------------------------------------------------------

	var scrapers []scraper.Scraper
	if len(mockScrapers) > 0 {
		scrapers = mockScrapers
	} else {
		hp, err := provider.NewHomeplus(scraperCfg)
		if err != nil {
			return fmt.Errorf("초기화 오류: 홈플러스 수집기(Scraper)를 구성할 수 없습니다. 상세 오류: %v", err)
		}

		lm, err := provider.NewLottemart(scraperCfg)
		if err != nil {
			return fmt.Errorf("초기화 오류: 롯데마트 수집기(Scraper)를 구성할 수 없습니다. 상세 오류: %v", err)
		}

		em, err := provider.NewEmart(scraperCfg)
		if err != nil {
			return fmt.Errorf("초기화 오류: 이마트 수집기(Scraper)를 구성할 수 없습니다. 상세 오류: %v", err)
		}

		scrapers = []scraper.Scraper{hp, lm, em}
	}

	// ------------------------------------------------------------------
	// 4단계: 병렬 강좌 수집
	// ------------------------------------------------------------------

	// 전체 수집 작업에 5분 타임아웃을 부여합니다.
	// 네트워크 지연이나 서버 응답 지연으로 프로그램이 무한히 멈춰있는 상황을 방지하기 위함입니다.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 각 지점의 강좌를 고루틴으로 동시에 수집하여 속도를 높입니다.
	// 수집기 중 단 한 곳이라도 데이터를 가져오지 못하면 즉시 전체 작업을 중단합니다.
	// 이는 누락이 있는 불완전한 결과물을 엑셀로 내보내지 않기 위한 안전 장치입니다.
	lectures, err := scraper.Scrape(ctx, scrapers)
	if err != nil {
		return fmt.Errorf("수집 작업 중단: 일부 수집기에서 치명적인 오류가 발생하여 전체 수집 프로세스를 안전하게 종료합니다.\n상세 원인: %v", err)
	}

	// ------------------------------------------------------------------
	// 5단계: 필터링
	// ------------------------------------------------------------------

	// 수집된 전체 강좌 중 설정 파일에 정의된 조건(나이 제한, 요일/시간, 마감, 키워드 등)에 부합하지 않는 강좌를 걸러냅니다.
	// 실제로 목록에서 삭제하는 것이 아니라, 각 강좌의 ScrapeExcluded 필드를 true로 표시하며, CSV 내보내기 단계에서 이 필드를 보고 제외합니다.
	filter.Filter(lectures, studentMonths, studentAge, cfg.Holidays)

	// ------------------------------------------------------------------
	// 6단계: CSV 파일로 저장
	// ------------------------------------------------------------------

	filename := fmt.Sprintf("culturelecture-scrape-%d%02d%02d%02d%02d%02d.csv", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())

	csvWriter := exporter.NewCSV(filename)
	err = csvWriter.Export(lectures)
	if err != nil {
		return fmt.Errorf("저장 오류: 수집된 강좌 데이터를 CSV 파일로 내보내는 데 실패하였습니다. 상세 오류: %v", err)
	}

	return nil
}
