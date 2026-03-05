package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/darkkaiser/culturelecture-scrape/internal/config"
	"github.com/darkkaiser/culturelecture-scrape/internal/exporter"
	"github.com/darkkaiser/culturelecture-scrape/internal/filter"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper/provider"
	"github.com/darkkaiser/notify-server/pkg/strutil"
)

var (
	// Version 애플리케이션의 버전을 나타냅니다.
	// 소스 코드에 선언된 값은 개발 중 기본값으로만 사용되며,
	// 릴리스 빌드 시에는 아래와 같이 -ldflags로 실제 버전을 주입해야 합니다.
	// 예: go build -ldflags="-X main.Version=v0.0.2" .
	Version = "v0.0.2"
)

func main() {
	// ------------------------------------------------------------------
	// 1단계: 실행 인자 파싱 및 설정 파일 로드
	// ------------------------------------------------------------------

	// -config 플래그로 사용할 설정 파일 경로를 지정할 수 있습니다.
	// 별도 지정이 없으면 실행 파일과 같은 디렉토리의 culturelecture-scrape.yaml을 사용합니다.
	var configPath string
	flag.StringVar(&configPath, "config", "culturelecture-scrape.yaml", "설정 파일 경로")
	flag.Parse()

	// 설정 파일을 읽어 애플리케이션 전반에서 사용할 Config 구조체로 파싱합니다.
	// 파일이 없거나 필수 항목이 누락된 경우 더 이상 진행이 불가능하므로 즉시 종료합니다.
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("애플리케이션 설정 초기화 중 치명적인 오류가 발생했습니다: %v", err)
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

	// 사용자가 설정 파일에 입력 시 실수로 포함할 수 있는 앞뒤 공백 및 중간 연속 공백을 NormalizeSpace로 제거하여
	// 이후 비교 및 API 호출이 안전하게 이루어지도록 합니다.
	searchYear := strutil.NormalizeSpace(cfg.SearchYear)
	searchSeason := strutil.NormalizeSpace(cfg.SearchSeason)

	// 강좌 나이 제한 필터링을 위해 수강자의 현재 나이를 미리 계산해 둡니다.
	// 나이(Age)와 개월 수(Months)를 함께 계산하는 이유는, 강좌마다 적용 기준이 '만 N세'와 '생후 N개월' 두 가지로 혼재하기 때문입니다.
	lecturerAge := cfg.Lecturer.CalculateAge(now)
	lecturerMonths := cfg.Lecturer.CalculateMonths(now)

	log.Printf("▶ %s년 %s 문화센터 강좌를 수집합니다.", searchYear, searchSeason)
	log.Printf("▶ 문화센터 강좌 수강자는 %d세(%d개월) 아이입니다.", lecturerAge, lecturerMonths)

	// NormalizeSpace 처리 후에도 값이 비어 있으면 수집 조건을 특정할 수 없으므로 종료합니다.
	if searchYear == "" || searchSeason == "" {
		log.Fatalln("검증 오류: 검색연도 및 검색시즌은 공백으로만 구성될 수 없습니다.")
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
		log.Fatalf("검증 오류: 유효하지 않은 검색시즌입니다. 허용되는 값: 봄, 여름, 가을, 겨울 (입력값: %q)", searchSeason)
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

	// 홈플러스, 롯데마트, 이마트 각 브랜드별 수집기 인스턴스를 생성합니다.
	hp, err := provider.NewHomeplus(scraperCfg)
	if err != nil {
		log.Fatalf("초기화 오류: 홈플러스 수집기(Scraper)를 구성할 수 없습니다. 상세 오류: %v", err)
	}

	lm, err := provider.NewLottemart(scraperCfg)
	if err != nil {
		log.Fatalf("초기화 오류: 롯데마트 수집기(Scraper)를 구성할 수 없습니다. 상세 오류: %v", err)
	}

	em, err := provider.NewEmart(scraperCfg)
	if err != nil {
		log.Fatalf("초기화 오류: 이마트 수집기(Scraper)를 구성할 수 없습니다. 상세 오류: %v", err)
	}

	scrapers := []scraper.Scraper{hp, lm, em}

	// ------------------------------------------------------------------
	// 4단계: 병렬 강좌 수집
	// ------------------------------------------------------------------

	// @@@@@
	// 전체 수집 작업에 5분 타임아웃을 부여합니다. 네트워크 지연이나 서버 응답 지연으로
	// 프로그램이 무한히 멈춰있는 상황을 방지하기 위함입니다.
	// defer cancel()로 함수 종료 시 컨텍스트가 보유한 리소스를 반드시 해제합니다.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 모든 수집기를 고루틴으로 병렬 실행하여 수집 시간을 단축합니다.
	// 하나라도 실패하면 전체 수집이 실패 처리됩니다. 부분 성공은 허용하지 않으며,
	// 불완전한 데이터를 CSV로 내보내지 않기 위한 의도적인 설계입니다.
	lectures, err := scraper.Scrape(ctx, scrapers)
	if err != nil {
		log.Printf("강좌 수집 실패: %v", err)
		return
	}

	// ------------------------------------------------------------------
	// 5단계: 필터링
	// ------------------------------------------------------------------

	// 수집된 전체 강좌 중 설정 파일에 정의된 조건(나이 제한, 요일/시간, 마감, 키워드 등)에
	// 부합하지 않는 강좌를 걸러냅니다. 실제로 목록에서 삭제하는 것이 아니라, 각 강좌의
	// ScrapeExcluded 필드를 true로 표시하며, CSV 내보내기 단계에서 이 필드를 보고 제외합니다.
	filter.Filter(lectures, lecturerMonths, lecturerAge, cfg.Holidays)

	// ------------------------------------------------------------------
	// 6단계: CSV 파일로 저장
	// ------------------------------------------------------------------

	// 파일명에 프로그램 시작 시각을 포함시켜 매 실행마다 고유한 파일이 생성되도록 합니다.
	// 덕분에 이전 실행 결과를 덮어쓰지 않고 그대로 보존할 수 있습니다.
	fileName := fmt.Sprintf("culturelecture-scrape-%d%02d%02d%02d%02d%02d.csv", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())

	csvExporter := exporter.NewCSVExporter(fileName)
	err = csvExporter.Export(lectures)
	if err != nil {
		log.Printf("CSV 저장 실패: %v", err)
	}
}
