package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/PuerkitoBio/goquery"
	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
	"github.com/darkkaiser/notify-server/pkg/strutil"
)

var (
	// lmStartDateRegex 개강일/요일/시간 문자열에서 개강일을 추출합니다.
	// 예) "2020.12.05(토) 15:20~16:00" -> "2020.12.05"
	lmStartDateRegex = regexp.MustCompile(`^[0-9]{4}\.[0-9]{2}\.[0-9]{2}`)

	// lmStartTimeRegex 개강일/요일/시간 문자열에서 강좌 시작 시간을 추출합니다.
	// 예) "2020.12.05(토) 15:20~16:00" -> " 15:20"
	lmStartTimeRegex = regexp.MustCompile(" [0-9]{2}:[0-9]{2}")

	// lmEndTimeRegex 개강일/요일/시간 문자열에서 강좌 종료 시간을 추출합니다.
	// 예) "2020.12.05(토) 15:20~16:00" -> "16:00"
	lmEndTimeRegex = regexp.MustCompile("[0-9]{2}:[0-9]{2}$")

	// lmWeekdayRegex 개강일/요일/시간 문자열에서 수업 요일을 추출합니다.
	// 예) "2020.12.05(토) 15:20~16:00" -> "(토"
	lmWeekdayRegex = regexp.MustCompile(`\([월화수목금토일]`)

	// lmPriceRegex 수강료 문자열에서 수강료를 추출합니다.
	// 예) "12회 80,000원 60,000원" -> "60,000원"
	lmPriceRegex = regexp.MustCompile("[0-9,]{1,8}원$")

	// lmSessionCountRegex 수강료 문자열에서 총 강좌 횟수를 추출합니다.
	// 예) "12회 80,000원 60,000원" -> "12회"
	lmSessionCountRegex = regexp.MustCompile("[0-9]{1,3}회")
)

// Lottemart 롯데마트 문화센터 강좌 정보를 수집하는 스크래퍼 구현체입니다.
type Lottemart struct {
	// name 스크래퍼가 수집 중인 대상이 어디인지 식별하기 위한 이름입니다. (예: "롯데마트")
	name string

	// cultureBaseURL 롯데마트 문화센터 웹사이트 기본 도메인 주소입니다.
	cultureBaseURL string

	// fetcher HTTP 요청을 수행하는 공유 클라이언트입니다.
	fetcher *scraper.Fetcher

	// searchTermCode 롯데마트 API 요청 시 사용되는 조회 대상 학기(시즌)의 고유 식별 코드입니다.
	// 검색 연도(예: "2024")와 학기 코드(예: "1")를 조합하여 "yyyy0s" 형식으로 생성되며, API의 'search_term_cd' 파라미터에 매핑됩니다.
	// 예: "202401" (2024년 봄학기)
	searchTermCode string

	// stores 수집 대상 점포 목록입니다. (점포코드 -> 점포명)
	stores map[string]string

	// lectureGroups 수집 대상 강좌군 목록입니다. (강좌군 탭 ID -> (강좌군코드 -> 강좌군명))
	lectureGroups map[string]map[string]string
}

// 컴파일 타임에 인터페이스 구현 여부를 검증합니다.
var _ scraper.Scraper = (*Lottemart)(nil)

// NewLottemart 롯데마트 스크래퍼를 생성하여 반환합니다.
func NewLottemart(criteria scraper.SearchCriteria) (*Lottemart, error) {
	searchYear := strutil.NormalizeSpace(criteria.SearchYear)
	searchSeasonCode := strutil.NormalizeSpace(criteria.SearchSeasonCode)

	// 검색년도와 학기(시즌) 코드는 API 요청에 필수적인 식별자이므로 누락을 허용하지 않습니다.
	if searchYear == "" || searchSeasonCode == "" {
		return nil, fmt.Errorf("유효하지 않은 검색 조건입니다: 롯데마트 API 요청에 필수적인 검색 연도 및 학기(시즌) 코드가 누락되었습니다 (입력값 - 검색 연도: '%s', 학기 코드: '%s')", searchYear, searchSeasonCode)
	}

	return &Lottemart{
		name:           "롯데마트",
		cultureBaseURL: "https://culture.lottemart.com",
		fetcher:        scraper.NewFetcher(),

		// 롯데마트 API 명세에 맞춰 검색 연도와 학기(시즌) 코드를 결합하여 검색용 고유 식별자("yyyy0s" 포맷)를 생성합니다.
		// 예: 2024년 봄학기(시즌 코드: "1")의 경우, "2024"와 "1"을 조합하여 "202401"로 변환
		searchTermCode: fmt.Sprintf("%s0%s", searchYear, searchSeasonCode),

		stores: map[string]string{
			"705": "여수점",
		},

		// 강좌군 탭 ID를 키로, 해당 탭에 속한 강좌군 코드와 강좌군명을 값으로 가집니다.
		// 강좌군명이 빈 문자열("")인 항목은 코드 존재 여부만 확인하고 명칭 검증은 건너뜁니다.
		lectureGroups: map[string]map[string]string{
			"baby-tit": { // 영아강좌(0~5세)
				"21": "음악감성",
				"81": "",
				"22": "미술표현",
				"82": "",
				"23": "언어인지",
				"83": "",
				"24": "통합놀이",
				"84": "",
				"25": "신체발달",
				"85": "",
				"26": "조기영재",
				"86": "",
				"27": "창의적체험활동",
				"87": "",
			},
			"toddler-tit": { // 유아 강좌(5~7세)
				"31": "음악 감성",
				"32": "미술표현",
				"33": "창의인지",
				"34": "언어인지",
				"35": "신체발달",
				"36": "키즈쿠킹",
				"37": "창의적체험활동",
			},
			"child-tit": { // 어린이청소년
				"41": "음악감성",
				"42": "미술표현",
				"43": "창의인지",
				"44": "진로/직업체험",
				"45": "언어인지",
				"46": "신체발달",
				"47": "키즈쿠킹",
				"48": "창의적체험활동",
			},
		},
	}, nil
}

// Validate 스크래핑 작업을 시작하기 전, 설정값이 실제 롯데마트 시스템과 정합성이 맞는지 사전 검증합니다.
func (l *Lottemart) Validate(ctx context.Context) error {
	// 수집 대상으로 설정된 각 강좌군이 롯데마트 강좌 목록 페이지의 실제 카테고리 메뉴에 존재하는지 확인합니다.
	validLectureGroups, err := l.validateLectureGroups(ctx)
	if err != nil {
		return fmt.Errorf("%s 문화센터 강좌군 정보 검증 중 내/외부 시스템 오류가 발생했습니다: %w", l.name, err)
	}
	if !validLectureGroups {
		return fmt.Errorf("%s 문화센터에 설정된 강좌군 코드가 유효하지 않거나 실제 시스템 데이터와 일치하지 않습니다", l.name)
	}

	// 수집 대상으로 설정된 각 점포가 롯데마트 점포 페이지에 실제로 존재하는지 확인합니다.
	for storeCode, storeName := range l.stores {
		validStore, err := l.validateStore(ctx, storeCode, storeName)
		if err != nil {
			return fmt.Errorf("%s 문화센터 점포 정보 검증 중 내/외부 시스템 오류가 발생했습니다. (대상 점포 코드: '%s'): %w", l.name, storeCode, err)
		}
		if !validStore {
			return fmt.Errorf("%s 문화센터에 설정된 점포 코드가 유효하지 않거나 실제 시스템 데이터와 일치하지 않습니다. (점포 코드: '%s')", l.name, storeCode)
		}
	}

	return nil
}

// validateStore 롯데마트 점포 상세 웹페이지를 직접 호출하고 문서 구조(HTML)를 분석하여,
// 인자로 받은 점포 코드에 매핑된 실제 점포 이름이 우리가 수집 대상으로 설정한 점포명(storeName)과 정확히 일치하는지 대조합니다.
func (l *Lottemart) validateStore(ctx context.Context, storeCode, storeName string) (bool, error) {
	// 주어진 점포 코드를 URL 파라미터로 넣어 해당 점포의 상세 정보 페이지를 요청하는 GET 요청 객체를 생성합니다.
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/cu/branch/main.do?search_str_cd=%s", l.cultureBaseURL, storeCode), nil)
	if err != nil {
		return false, fmt.Errorf("롯데마트 점포 상세 페이지 조회를 위한 HTTP 요청 객체 생성 중 오류가 발생하였습니다: %w", err)
	}

	// 점포 상세 페이지의 HTML을 파싱하여 DOM 구조로 가져옵니다.
	doc, err := l.fetcher.FetchHTML(req)
	if err != nil {
		return false, err
	}

	// 점포명이 표시된 h3 요소를 탐색하여, 설정된 점포명과 일치하는지 검증합니다.
	// 요소가 정확히 1개 존재하고, 텍스트가 설정된 점포명과 완벽히 일치해야 유효한 점포로 판단합니다.
	sel := doc.Find("#contents div.branch_main-wrap div.branch_info-area > div.branch_spot-area > h3")
	if sel.Length() != 1 || strutil.NormalizeSpace(sel.Text()) != storeName {
		return false, nil
	}

	return true, nil
}

// validateLectureGroups 롯데마트 강좌 목록 웹페이지를 직접 호출하고 문서 구조(HTML)를 분석하여,
// 수집 대상으로 설정된 강좌군(카테고리) 탭과 강좌군 코드·명칭이 실제 UI 메뉴 상에 빠짐없이 존재하는지 교차 검증합니다.
func (l *Lottemart) validateLectureGroups(ctx context.Context) (bool, error) {
	// 롯데마트 강좌 목록 페이지의 HTML을 가져오기 위한 GET 요청 객체를 생성합니다.
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/cu/gus/course/courseinfo/courselist.do", l.cultureBaseURL), nil)
	if err != nil {
		return false, fmt.Errorf("롯데마트 강좌군(카테고리) 정보 조회를 위한 HTTP 요청 객체 생성 중 오류가 발생하였습니다: %w", err)
	}

	// 강좌 목록 페이지의 HTML을 파싱하여 DOM 구조로 가져옵니다.
	doc, err := l.fetcher.FetchHTML(req)
	if err != nil {
		return false, err
	}

	// 설정된 각 강좌군 탭을 순회하며, 탭 ID와 탭 내 강좌군 코드·명칭 양면으로 정합성을 교차 검증합니다.
	for tabID, subGroups := range l.lectureGroups {
		// 강좌군 탭 ID에 해당하는 DOM 요소가 정확히 1개 존재하는지 확인합니다.
		sel := doc.Find(fmt.Sprintf("#%s", tabID))
		if sel.Length() != 1 {
			return false, nil
		}

		// 탭 내의 각 강좌군 코드를 순회하며, input 요소의 존재 여부와 표시 명칭을 검증합니다.
		// 강좌군명이 빈 문자열인 항목은 코드 존재 여부만 확인하므로 명칭 검증 없이 건너뜁니다.
		for code, name := range subGroups {
			if name == "" {
				continue
			}

			// 강좌군 코드와 매칭되는 input 요소를 탐색하고, 인접한 표시 텍스트가 설정된 강좌군명과 일치하는지 검증합니다.
			subSel := sel.Parent().Parent().Parent().Find(fmt.Sprintf("dd > ul > li > div > input[value='%s']", code))
			if subSel.Length() != 1 || strutil.NormalizeSpace(subSel.Parent().Text()) != name {
				return false, nil
			}
		}
	}

	return true, nil
}

// Scrape 설정된 모든 점포를 대상으로 롯데마트 문화센터 강좌 정보를 수집하여 반환합니다.
//
// 수집은 2단계 병렬 구조로 진행됩니다.
//  1. 점포(Store) 단위 병렬 처리: 여러 점포를 동시에 스크래핑합니다.
//  2. 페이지(Page) 단위 병렬 처리: 동일한 점포 내에서도 여러 목록 페이지를 동시에 가져옵니다.
//
// 각 계층의 동시성은 SetLimit으로 제한하여 대상 서버에 과도한 부하를 주지 않도록 합니다.
func (l *Lottemart) Scrape(ctx context.Context) ([]domain.Lecture, error) {
	// [1단계] 점포 단위 병렬 스크래핑 환경 구성
	// 대상 서버의 과부하 및 IP 차단을 방지하기 위해 최대 5개의 점포만 동시에 스크래핑합니다.
	storeGroup, storeCtx := errgroup.WithContext(ctx)
	storeGroup.SetLimit(5)

	// 수집된 전체 강좌 데이터를 안전하게 취합하기 위한 공용 슬라이스와 뮤텍스입니다.
	// 고루틴 간 락(Lock) 충돌로 인한 성능 저하를 막기 위해, 페이지 단위로 모아서 한 번에 추가합니다.
	var lectures []domain.Lecture
	var mu sync.Mutex

	for storeCode, storeName := range l.stores {
		// 루프 변수 클로저 캡처 방지 (Go 1.22 이전 버전 구문 호환 보장)
		storeCode, storeName := storeCode, storeName

		storeGroup.Go(func() error {
			// [사전 단계] 페이지네이션 메타데이터 확보
			// 첫 번째 페이지를 우선 요청하여 수집해야 할 전체 페이지 수를 파악합니다.
			_, doc, err := l.fetchSearchPage(storeCtx, 1, storeCode)
			if err != nil {
				return fmt.Errorf("%s 문화센터의 강좌 페이지 수 파악을 위한 초기 정보 조회에 실패했습니다 (점포 코드: %s): %w", l.name, storeCode, err)
			}

			// 페이지 정보는 마지막 <tr> 요소의 'pageinfo' 속성에서 추출합니다.
			rawPageInfo, exists := doc.Find("tr:last-child").Attr("pageinfo")
			if !exists {
				return fmt.Errorf("%s 문화센터 강좌 목록에서 전체 페이지 수를 산정하기 위한 메타데이터 속성('pageinfo')을 찾을 수 없습니다", l.name)
			}

			// ---------------------------------
			// pageinfo 값 형식 : 1|5|85|61|0|24
			// ---------------------------------
			// 1  : 현재 페이지 번호
			// 5  : 전체 페이지 번호
			// 85 : 전체 강좌 갯수
			// 61 : 접수가능 갯수
			// 0  : 온라인마감 갯수
			// 24 : 접수마감 갯수
			pageInfoParts := strings.Split(rawPageInfo, "|")
			if len(pageInfoParts) != 6 {
				return fmt.Errorf("%s 문화센터 강좌의 페이지 속성 구조가 예상과 일치하지 않아 전체 페이지 수를 파악할 수 없습니다 (추출된 원본 데이터: '%s')", l.name, rawPageInfo)
			}

			totalPages, err := strconv.Atoi(pageInfoParts[1])
			if err != nil {
				return fmt.Errorf("%s 문화센터 강좌의 전체 페이지 수(문자열: '%s')를 정수형 데이터로 변환하는 데 실패했습니다: %w", l.name, pageInfoParts[1], err)
			}

			// [2단계] 점포 내 페이지 단위 병렬 스크래핑 환경 구성
			// 단일 점포에 대한 과도한 동시 요청을 제한하기 위해 최대 10페이지만 동시에 수집합니다.
			pageGroup, pageCtx := errgroup.WithContext(storeCtx)
			pageGroup.SetLimit(10)

			for page := 1; page <= totalPages; page++ {
				// 취소된 컨텍스트에 대해 불필요한 고루틴 스케줄링이 발생하지 않도록 조기 차단합니다.
				if err := pageCtx.Err(); err != nil {
					break
				}

				// 루프 변수 클로저 캡처 방지 (Go 1.22 이전 버전 구문 호환 보장)
				page := page

				pageGroup.Go(func() error {
					searchURL, doc, err := l.fetchSearchPage(pageCtx, page, storeCode)
					if err != nil {
						return fmt.Errorf("%s 문화센터 강좌 목록 페이지 로드에 실패했습니다 (점포명: '%s', 대상 페이지: %d): %w", l.name, storeName, page, err)
					}

					var extractErr error // goquery.Each() 내부에서 발생하는 에러를 담아두기 위한 변수입니다.
					var pageLectures []domain.Lecture

					sel := doc.Find("tr")
					sel.Each(func(i int, s *goquery.Selection) {
						if extractErr != nil {
							return // 선행된 요소 파싱에서 에러가 발생한 경우, 후속 DOM 순회를 즉시 중단합니다.
						}

						lecture, err := l.extractLecture(pageCtx, s, storeCode, storeName, searchURL)
						if err != nil {
							extractErr = fmt.Errorf("%s 문화센터 개별 강좌 데이터 정보 파싱 중 오류가 발생했습니다 (점포명: '%s', 대상 페이지: %d): %w", l.name, storeName, page, err)
							return
						}

						if lecture != nil {
							pageLectures = append(pageLectures, *lecture)
						}
					})

					if extractErr != nil {
						return extractErr // 발생한 에러를 반환하여, 함께 실행 중인 다른 페이지들의 수집 작업도 안전하게 취소시킵니다.
					}

					// 현재 페이지에서 파싱한 강좌들을 전체 공유 목록에 안전하게 병합합니다.
					// 성능 병목을 방지하기 위해 강좌 낱개가 아닌 페이지 단위로 모아서 한 번에 추가합니다.
					if len(pageLectures) > 0 {
						mu.Lock()
						lectures = append(lectures, pageLectures...)
						mu.Unlock()
					}

					return nil
				})
			}

			// 현재 점포의 모든 페이지 스크래핑 작업이 완료될 때까지 대기합니다.
			if err := pageGroup.Wait(); err != nil {
				return err
			}

			return nil
		})
	}

	// 모든 점포의 스크래핑 작업이 완료될 때까지 대기합니다. 이 중 하나라도 실패하면 해당 에러를 반환합니다.
	if err := storeGroup.Wait(); err != nil {
		return nil, err
	}

	return lectures, nil
}

// fetchSearchPage 롯데마트 강좌 검색 API를 호출하여 특정 페이지의 강좌 목록 데이터를 가져옵니다.
//
// 반환값:
//   - string: 요청에 사용된 검색 URL
//   - *goquery.Document: 파싱된 HTML DOM
//   - error: HTTP 요청 생성 또는 데이터 수신 중 발생한 오류
func (l *Lottemart) fetchSearchPage(ctx context.Context, page int, storeCode string) (string, *goquery.Document, error) {
	searchURL := fmt.Sprintf("%s/cu/gus/course/courseinfo/searchList.do", l.cultureBaseURL)

	// 조회할 페이지 번호, 점포 코드, 학기 코드 등 검색 결과에 직접적인 영향을 미치는 핵심 매개변수를 설정합니다.
	form := url.Values{}
	form.Set("currPageNo", strconv.Itoa(page))
	form.Set("search_str_cd", storeCode)
	form.Set("search_term_cd", l.searchTermCode)
	form.Set("is_category_open", "Y")

	// API 명세 준수를 위해 현재 사용하지 않는 선택적 필터 매개변수들을 빈 값으로 명시적으로 포함시킵니다.
	form.Set("cls_cd", "")
	form.Set("fam_no", "")
	form.Set("from_fg", "")
	form.Set("search_cls_nm", "")
	form.Set("search_day_fg", "")
	form.Set("search_list_type", "")
	form.Set("search_opt_cd", "")
	form.Set("search_order_gbn", "")
	form.Set("search_reg_status", "")
	form.Set("search_tit_cd", "")
	form.Set("wish_typ", "")

	// 수집 대상 강좌군 코드들을 순회하여 두 가지 형태의 매개변수로 동시에 구성합니다.
	// - 'search_cat_cd': 쉼표(,)로 구분된 단일 요약 값 (예: "21,22,31")
	// - 'arr_cat_cd': 코드당 하나씩 누적되는 배열 형태의 반복 매개변수 (예: arr_cat_cd=21&arr_cat_cd=22...)
	var categoryCodes []string
	for _, subGroups := range l.lectureGroups {
		for lectureGroupCode := range subGroups {
			form.Add("arr_cat_cd", lectureGroupCode)
			categoryCodes = append(categoryCodes, lectureGroupCode)
		}
	}
	form.Set("search_cat_cd", strings.Join(categoryCodes, ","))

	// 폼 데이터를 URL 인코딩 문자열로 직렬화하여 요청 본문(io.Reader)을 구성하고, 이를 담은 HTTP POST 요청 객체를 생성합니다.
	reqBody := strings.NewReader(form.Encode())
	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("%s 문화센터 강좌 검색 페이지 조회를 위한 HTTP 요청 객체 생성 중 오류가 발생하였습니다: %w", l.name, err)
	}

	// Content-Type 헤더를 설정하여 서버가 요청 본문을 폼 인코딩 형식으로 해석하도록 지정합니다.
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	// HTTP 요청을 실행하여 응답 본문을 바이트 슬라이스로 수신합니다.
	// [주의] 롯데마트 강좌 검색 API는 완전한 HTML 문서가 아닌 '<tr>/<td>' 태그만으로 구성된 HTML 파편(Fragment)을 반환합니다.
	// goquery는 루트 컨테이너 태그 없이 '<tr>'만 파싱하면 브라우저 규격에 따라 해당 요소를 자동으로 제거하므로,
	// 스트림을 직접 파싱하는 대신 바이트 배열로 먼저 수신한 뒤 '<table>' 태그로 감싸는 전처리를 수행합니다.
	rawHTML, err := l.fetcher.FetchBody(req)
	if err != nil {
		return "", nil, fmt.Errorf("%s 문화센터 강좌 검색 페이지 데이터를 가져오는 중 내/외부 통신 오류가 발생하였습니다: %w", l.name, err)
	}

	// 수신한 HTML 파편을 '<table>' 태그로 래핑하여 유효한 DOM 구조로 복원한 뒤, goquery 문서로 파싱합니다.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table>" + string(rawHTML) + "</table>"))
	if err != nil {
		return "", nil, fmt.Errorf("%s 문화센터 강좌 검색 페이지의 HTML을 파싱하는 중 오류가 발생하였습니다: %w", l.name, err)
	}

	return searchURL, doc, nil
}

// extractLecture 강좌 목록 페이지의 개별 강좌 DOM 요소(s)를 파싱하여 domain.Lecture 구조체로 변환하여 반환합니다.
func (l *Lottemart) extractLecture(ctx context.Context, s *goquery.Selection, storeCode, storeName, searchURL string) (*domain.Lecture, error) {
	// ------------------------------------------------------------------
	// 1단계: 원시 데이터 파싱 (Raw Data Parsing)
	// ------------------------------------------------------------------

	// 강좌 데이터가 담긴 5개의 테이블 컬럼(td) 요소를 추출합니다.
	// 예상과 다른 수의 컬럼이 존재하면 HTML 구조가 변경된 것으로 간주하여 즉시 중단합니다.
	columns := s.Find("td")
	if columns.Length() != 5 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다 (원인: 강좌 컬럼 개수 불일치, 기대값: 5, 실제값: %d, 대상 URL: %s)", l.name, columns.Length(), searchURL)
	}

	// HTML 테이블 구조에 따라 각 컬럼 요소를 의미에 맞는 변수에 명시적으로 언패킹합니다.
	// [0]강좌명, [1]강사명, [2]개강일/요일/시간, [3]수강료/횟수, [4]접수상태
	titleNode := columns.Eq(0)                                         // [0] 강좌명, 상세URL 등 하위 태그 탐색용 DOM 노드
	rawInstructor := strutil.NormalizeSpace(columns.Eq(1).Text())      // [1] 형식: "김준희"
	rawSchedule := strutil.NormalizeSpace(columns.Eq(2).Text())        // [2] 형식: "2020.12.05(토) 15:20~16:00"
	rawSessionAndPrice := strutil.NormalizeSpace(columns.Eq(3).Text()) // [3] 형식: "12회 80,000원 60,000원"
	statusNode := columns.Eq(4)                                        // [4] 접수상태 태그 탐색용 DOM 노드

	// ------------------------------------------------------------------
	// 2단계: 핵심 텍스트 필드 검증 및 추출
	// ------------------------------------------------------------------

	// 강좌명
	titleAnchorNode := titleNode.Find("div.info-txt > a")
	if titleAnchorNode.Length() == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 구조적 오류가 감지되었습니다 (원인: 강좌명 및 상세 식별자를 포함하는 <a> 태그 부재, 대상 URL: %s)", l.name, searchURL)
	}
	title := strutil.NormalizeSpace(titleAnchorNode.Text())
	if len(title) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 필수 식별자가 누락되었습니다 (원인: 강좌명 텍스트 부재, 대상 URL: %s)", l.name, searchURL)
	}

	// 상세페이지 URL 식별자를 onclick 속성에서 미리 추출합니다.
	rawOnClick, exists := titleAnchorNode.Attr("onclick")
	if !exists {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 필수 식별자가 누락되었습니다 (원인: 상세페이지 접근용 'onclick' 속성 부재, 대상 URL: %s)", l.name, searchURL)
	}
	// onclick 속성("goDetail('CLASS_CD', ...)")을 작은따옴표로 분리하여 두 번째 조각(index 1)인 강좌 코드를 추출합니다.
	onClickParts := strings.Split(rawOnClick, "'")
	if len(onClickParts) < 3 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 'onclick' 속성 내 강좌 식별코드 분리 규격 불일치, 대상 URL: %s)", l.name, searchURL)
	}
	lectureCode := onClickParts[1]

	// ------------------------------------------------------------------
	// 3단계: 정규식을 이용한 세부 필드 파싱 (Regex Parsing)
	// ------------------------------------------------------------------

	// 개강일
	startDate := lmStartDateRegex.FindString(rawSchedule)
	if len(startDate) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 개강일 데이터가 예상된 규격과 일치하지 않음, 원본 텍스트: '%s', 대상 URL: %s)", l.name, rawSchedule, searchURL)
	}
	startDate = strings.ReplaceAll(startDate, ".", "-")

	// 시작시간, 종료시간
	startTime := strings.TrimSpace(lmStartTimeRegex.FindString(rawSchedule))
	endTime := strings.TrimSpace(lmEndTimeRegex.FindString(rawSchedule))
	if len(startTime) == 0 || len(endTime) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 시작/종료 시간 데이터가 예상된 규격과 일치하지 않음, 원본 텍스트: '%s', 대상 URL: %s)", l.name, rawSchedule, searchURL)
	}

	// 요일
	weekday := lmWeekdayRegex.FindString(rawSchedule)
	if len(weekday) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 수업 요일 데이터가 예상된 규격과 일치하지 않음, 원본 텍스트: '%s', 대상 URL: %s)", l.name, rawSchedule, searchURL)
	}
	weekday = string([]rune(weekday)[1:])

	// 수강료
	price := lmPriceRegex.FindString(rawSessionAndPrice)
	if strings.Contains(price, "원") == false {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 수강료 데이터가 예상된 규격과 일치하지 않음, 원본 텍스트: '%s', 대상 URL: %s)", l.name, rawSessionAndPrice, searchURL)
	}

	// 강좌횟수
	sessionCount := lmSessionCountRegex.FindString(rawSessionAndPrice)
	if len(sessionCount) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 강좌 횟수 데이터가 예상된 규격과 일치하지 않음, 원본 텍스트: '%s', 대상 URL: %s)", l.name, rawSessionAndPrice, searchURL)
	}

	// ------------------------------------------------------------------
	// 4단계: 접수 상태 판별 (Reception Status Detection)
	// ------------------------------------------------------------------

	rawStatusLabel := strutil.NormalizeSpace(statusNode.Find("div > div > a.btn-status:last-child").Text())

	var receptionStatus = domain.ReceptionStatusUnknown
	switch rawStatusLabel {
	case "바로신청":
		receptionStatus = domain.ReceptionStatusPossible

	case "접수마감":
		receptionStatus = domain.ReceptionStatusClosed

	case "대기자 신청":
		receptionStatus = domain.ReceptionStatusStandBy

	case "현장문의":
		receptionStatus = domain.ReceptionStatusOnsiteInquiry

	case "전화문의":
		receptionStatus = domain.ReceptionStatusPhoneInquiry

	case "현장접수":
		// 롯데마트는 '현장접수' 상태를 '현장문의'와 동일하게 취급합니다.
		receptionStatus = domain.ReceptionStatusOnsiteInquiry

	default:
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 미지원 상태가 감지되었습니다 (원인: 해석 불가한 접수 상태 라벨, 식별된 라벨: '%s', 대상 URL: %s)", l.name, rawStatusLabel, searchURL)
	}

	// ------------------------------------------------------------------
	// 5단계: 컨텍스트 취소 여부 최종 확인
	// ------------------------------------------------------------------
	// 모든 데이터 파싱이 완료된 시점에서 확인하여, 취소된 컨텍스트에 대해 domain.Lecture 객체 생성을 생략합니다.

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// ------------------------------------------------------------------
	// 6단계: 최종 도메인 모델 생성 및 반환
	// ------------------------------------------------------------------

	return &domain.Lecture{
		StoreName:      fmt.Sprintf("%s %s", l.name, storeName),
		Category:       "", // 롯데마트는 강좌 목록 뷰에 카테고리 텍스트를 제공하지 않음
		Title:          title,
		Instructor:     rawInstructor,
		StartDate:      startDate,
		StartTime:      startTime,
		EndTime:        endTime,
		Weekday:        fmt.Sprintf("%s요일", weekday),
		Price:          price,
		SessionCount:   sessionCount,
		Status:         receptionStatus,
		DetailPageURL:  l.buildDetailPageURL(lectureCode, storeCode),
		ScrapeExcluded: false,
	}, nil
}

// buildDetailPageURL 강좌 고유 식별자와 점포 코드를 받아 해당 강좌의 상세 페이지 URL을 조립하여 반환합니다.
func (l *Lottemart) buildDetailPageURL(lectureCode, storeCode string) string {
	return fmt.Sprintf("%s/cu/gus/course/courseinfo/courseview.do?cls_cd=%s&is_category_open=N&search_term_cd=%s&search_str_cd=%s", l.cultureBaseURL, lectureCode, l.searchTermCode, storeCode)
}
