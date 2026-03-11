package provider

import (
	"context"
	"fmt"
	"math"
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

// hpSearchPageSize 홈플러스 강좌 목록을 조회할 때 한 페이지당 몇 개의 강좌 데이터를 가져올지 결정하는 값입니다.
const hpSearchPageSize = 20

var (
	// hpInstructorRegex 강사 정보 문자열에서 강사명을 추출합니다.
	// 예) "신혜정 강사" -> "신혜정 강사"
	hpInstructorRegex = regexp.MustCompile("^(.)*강사")

	// hpStartDateRegex 강좌 날짜/기간 문자열에서 개강일을 추출합니다.
	// 예) "2023.08.20 ~ 2023.11.19" -> "2023.08.20 ~"
	hpStartDateRegex = regexp.MustCompile("[0-9]{4}.[0-9]{2}.[0-9]{2} ~")

	// hpStartTimeRegex 요일/시간 문자열에서 강좌 시작 시간을 추출합니다.
	// 예) "일 14:20 ~ 15:00" -> "14:20 ~"
	hpStartTimeRegex = regexp.MustCompile("[0-9]{2}:[0-9]{2} ~")

	// hpEndTimeRegex 요일/시간 문자열에서 강좌 종료 시간을 추출합니다.
	// 예) "일 14:20 ~ 15:00" -> "~ 15:00"
	hpEndTimeRegex = regexp.MustCompile("~ [0-9]{2}:[0-9]{2}")

	// hpWeekdayRegex 요일/시간 문자열에서 수업 요일을 추출합니다.
	// 예) "일 14:20 ~ 15:00" -> "일 "
	hpWeekdayRegex = regexp.MustCompile("^[월화수목금토일] ")

	// hpPeopleStandardRegex 수강료 문자열에서 인원 기준 표기를 제거하기 위한 패턴입니다.
	// 예) "1회 7,000원 (2인 기준)" -> "1회 7,000원"
	hpPeopleStandardRegex = regexp.MustCompile(`\s*\(\d+인 기준\)`)

	// hpPriceRegex 강좌 정보 문자열에서 수강료를 추출합니다.
	// 예) "1회 6,000원" -> " 6,000원"
	hpPriceRegex = regexp.MustCompile(" [0-9]{1,3}(,[0-9]{3})*원$")

	// hpSessionCountRegex 강좌 정보 문자열에서 총 강좌 횟수를 추출합니다.
	// 예) "1회 6,000원" -> "1회"
	hpSessionCountRegex = regexp.MustCompile("^[0-9]{1,3}회")
)

// Homeplus 홈플러스 문화센터 강좌 정보를 수집하는 스크래퍼 구현체입니다.
type Homeplus struct {
	// name 스크래퍼가 수집 중인 대상이 어디인지 식별하기 위한 이름입니다. (예: "홈플러스")
	name string

	// cultureBaseURL 홈플러스 문화센터 웹사이트 기본 도메인 주소입니다.
	cultureBaseURL string

	// fetcher HTTP 요청을 수행하는 공유 클라이언트입니다.
	fetcher *scraper.Fetcher

	// stores 수집 대상 점포 목록입니다. (점포코드 -> 점포명)
	stores map[string]string

	// lectureGroups 수집 대상 강좌군 목록입니다. (강좌군코드 -> 강좌군명)
	lectureGroups map[string]string
}

// 컴파일 타임에 인터페이스 구현 여부를 검증합니다.
var _ scraper.Scraper = (*Homeplus)(nil)

// hpStoreAPIResponse 홈플러스 '점포 찾기' API 응답을 언마샬링(Unmarshal)하기 위한 데이터 구조체입니다.
// 스크래핑 작업 시작 전, 대상 점포의 존재 여부 및 데이터 유효성을 사전 검증(Validation)하는 단계에서 활용됩니다.
type hpStoreAPIResponse struct {
	RstCode    int    `json:"RstCode"`
	RstMessage string `json:"RstMessage"`
	Data       struct {
		StoreList []struct {
			StoreAreaName        string `json:"StoreAreaName"`
			StoreCode            string `json:"StoreCode"`
			StoreName            string `json:"StoreName"`
			RegionHQID           int    `json:"RegionHQID"`
			SortingNumber        int    `json:"SortingNumber"`
			RealStoreCode        string `json:"RealStoreCode"`
			PhoneNumber          string `json:"PhoneNumber"`
			FaxNumber            any    `json:"FaxNumber"`
			ZipCode              string `json:"ZipCode"`
			Address1             string `json:"Address1"`
			Address2             string `json:"Address2"`
			AddressPrevVer       string `json:"AddressPrevVer"`
			OperaterName         any    `json:"OperaterName"`
			OperatorMobileNumber string `json:"OperatorMobileNumber"`
		} `json:"StoreList"`
		MyStoreList []struct {
			StoreAreaName        any    `json:"StoreAreaName"`
			StoreCode            string `json:"StoreCode"`
			StoreName            string `json:"StoreName"`
			RegionHQID           int    `json:"RegionHQID"`
			SortingNumber        int    `json:"SortingNumber"`
			RealStoreCode        any    `json:"RealStoreCode"`
			PhoneNumber          any    `json:"PhoneNumber"`
			FaxNumber            any    `json:"FaxNumber"`
			ZipCode              any    `json:"ZipCode"`
			Address1             any    `json:"Address1"`
			Address2             any    `json:"Address2"`
			AddressPrevVer       any    `json:"AddressPrevVer"`
			OperaterName         any    `json:"OperaterName"`
			OperatorMobileNumber any    `json:"OperatorMobileNumber"`
		} `json:"MyStoreList"`
	} `json:"Data"`
}

// NewHomeplus 홈플러스 스크래퍼를 생성하여 반환합니다.
func NewHomeplus(criteria scraper.SearchCriteria) (*Homeplus, error) {
	return &Homeplus{
		name:           "홈플러스",
		cultureBaseURL: "https://mschool.homeplus.co.kr",
		fetcher:        scraper.NewFetcher(),

		stores: map[string]string{
			"0035": "광양점",
			"0030": "순천점",
		},

		lectureGroups: map[string]string{
			"MH|EL|IF": "Kids 전체",
			"BB":       "Baby 전체",
		},
	}, nil
}

func (h *Homeplus) Name() string {
	return h.name
}

// Validate 스크래핑 작업을 시작하기 전, 설정값이 실제 홈플러스 시스템과 정합성이 맞는지 사전 검증합니다.
func (h *Homeplus) Validate(ctx context.Context) error {
	// 수집 대상으로 설정된 각 점포가 홈플러스 점포 API에 실제로 등록되어 있는지 확인합니다.
	validStores, err := h.validateStores(ctx)
	if err != nil {
		return fmt.Errorf("%s 문화센터 점포 정보 검증 중 내/외부 시스템 오류가 발생했습니다: %w", h.name, err)
	}
	if !validStores {
		return fmt.Errorf("%s 문화센터에 설정된 점포 코드가 유효하지 않거나 실제 시스템 데이터와 일치하지 않습니다", h.name)
	}

	// 수집 대상으로 설정된 각 강좌군이 홈플러스 강좌 검색 페이지의 카테고리 메뉴에 실제로 존재하는지 확인합니다.
	validLectureGroups, err := h.validateLectureGroups(ctx)
	if err != nil {
		return fmt.Errorf("%s 문화센터 강좌군 정보 검증 중 내/외부 시스템 오류가 발생했습니다: %w", h.name, err)
	}
	if !validLectureGroups {
		return fmt.Errorf("%s 문화센터에 설정된 강좌군 코드가 유효하지 않거나 실제 시스템 데이터와 일치하지 않습니다", h.name)
	}

	return nil
}

// validateStores 홈플러스 점포 API를 호출하여, 설정된 수집 대상 점포들이 실제 시스템에 존재하는지 검증합니다.
func (h *Homeplus) validateStores(ctx context.Context) (bool, error) {
	// 홈플러스 전국 점포 목록을 조회하는 API 요청을 생성합니다.
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/Store/GetStoreList", h.cultureBaseURL), nil)
	if err != nil {
		return false, fmt.Errorf("홈플러스 점포 목록 조회를 위한 HTTP 요청 객체 생성 중 오류가 발생하였습니다: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	// API 응답을 JSON으로 파싱하여 전국 점포 목록을 가져옵니다.
	var storeListResp hpStoreAPIResponse
	err = h.fetcher.FetchJSON(req, &storeListResp)
	if err != nil {
		return false, err
	}

	// 설정된 수집 대상 점포 목록과 시스템 원천 데이터 간의 정합성을 교차 검증합니다.
	// 점포의 식별자(StoreCode)와 명칭(StoreName)이 완벽히 일치하는 경우에만 유효한 엔티티로 간주하며,
	// 단 하나의 점포라도 일치하지 않을 시 즉각적으로 검증 실패를 반환합니다.
	for storeCode, storeName := range h.stores {
		foundStore := false
		for _, store := range storeListResp.Data.StoreList {
			if storeCode == store.StoreCode && storeName == store.StoreName {
				foundStore = true
				break
			}
		}
		if foundStore == false {
			return false, nil
		}
	}

	return true, nil
}

// validateLectureGroups 홈플러스 강좌 검색 페이지의 실제 HTML을 파싱하여,
// 설정된 수집 대상 강좌군(카테고리)들이 현재 시스템의 UI 메뉴에 정확히 존재하는지 검증합니다.
func (h *Homeplus) validateLectureGroups(ctx context.Context) (bool, error) {
	// 홈플러스 강좌 검색 페이지의 HTML을 가져오기 위한 GET 요청 객체를 생성합니다.
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/Lecture/Search", h.cultureBaseURL), nil)
	if err != nil {
		return false, fmt.Errorf("홈플러스 강좌군(카테고리) 정보 조회를 위한 HTTP 요청 객체 생성 중 오류가 발생하였습니다: %w", err)
	}

	// 강좌 검색 페이지의 HTML을 파싱하여 DOM 구조로 가져옵니다.
	doc, err := h.fetcher.FetchHTML(req)
	if err != nil {
		return false, err
	}

	// 설정된 각 강좌군을 순회하며, 실제 UI 메뉴와 코드‧명칭 양면으로 정합성을 교차 검증합니다.
	for code, name := range h.lectureGroups {
		// CSS 선택자로 해당 강좌군 코드에 매칭되는 버튼 요소를 탐색합니다.
		// 동일한 코드를 가진 버튼이 정확히 1개 존재해야 유효한 강좌군으로 판단합니다.
		sel := doc.Find(fmt.Sprintf("section.search_body div.menu_depth_2_wrap ul.tree_menu_2 > li.depth_2 > ul.depth_3 > li:first-child > button[data-lecture-target='%s']", code))
		if sel.Length() != 1 {
			return false, nil
		}

		// 버튼의 표시 텍스트가 로컬에 설정된 강좌군 명칭과 일치하는지 추가로 검증합니다.
		// 강좌군 코드가 존재하더라도 명칭이 변경된 경우를 감지하기 위한 이중 안전장치입니다.
		val := sel.Text()
		if strutil.NormalizeSpace(val) != name {
			return false, nil
		}
	}

	return true, nil
}

// Scrape 설정된 모든 점포를 대상으로 홈플러스 문화센터 강좌 정보를 수집하여 반환합니다.
//
// 수집은 2단계 병렬 구조로 진행됩니다.
//  1. 점포(Store) 단위 병렬 처리: 여러 점포를 동시에 스크래핑합니다.
//  2. 페이지(Page) 단위 병렬 처리: 동일한 점포 내에서도 여러 목록 페이지를 동시에 가져옵니다.
//
// 각 계층의 동시성은 SetLimit으로 제한하여 대상 서버에 과도한 부하를 주지 않도록 합니다.
func (h *Homeplus) Scrape(ctx context.Context) ([]domain.Lecture, error) {
	// [1단계] 점포 단위 병렬 스크래핑 환경 구성
	// 대상 서버의 과부하 및 IP 차단을 방지하기 위해 최대 5개의 점포만 동시에 스크래핑합니다.
	storeGroup, storeCtx := errgroup.WithContext(ctx)
	storeGroup.SetLimit(5)

	// 수집된 전체 강좌 데이터를 안전하게 취합하기 위한 공용 슬라이스와 뮤텍스입니다.
	// 고루틴 간 락(Lock) 충돌로 인한 성능 저하를 막기 위해, 페이지 단위로 모아서 한 번에 추가합니다.
	var lectures []domain.Lecture
	var mu sync.Mutex

	for storeCode, storeName := range h.stores {
		// 루프 변수 클로저 캡처 방지 (Go 1.22 이전 버전 구문 호환 보장)
		storeCode, storeName := storeCode, storeName

		storeGroup.Go(func() error {
			// [사전 단계] 페이지네이션 메타데이터 확보
			// 첫 번째 페이지를 우선 렌더링하여 강좌 총 건수와 수집해야 할 전체 페이지 수를 계산합니다.
			_, doc, err := h.fetchSearchPage(storeCtx, 1, storeCode, storeName)
			if err != nil {
				return fmt.Errorf("%s 문화센터 전체 강좌 갯수 조회를 위한 사전 페이지 요청 중 오류가 발생하였습니다 (대상 점포: %s): %w", h.name, storeName, err)
			}

			rawTotalCount := doc.Find("#divTotalCnt").Text()
			if len(rawTotalCount) == 0 {
				return fmt.Errorf("%s 문화센터 강좌 수집 중 전체 강좌 갯수 추출에 실패하였습니다 (대상 점포: %s)", h.name, storeName)
			}

			totalLectureCount, err := strconv.Atoi(rawTotalCount)
			if err != nil {
				return fmt.Errorf("전체 강좌 갯수 파싱 중 캐스팅 오류가 발생하였습니다 (대상 점포: %s): %w", storeName, err)
			}

			// 올림 처리를 통해 마지막 페이지까지 누락 없이 수집 범위를 설정합니다.
			totalPages := int(math.Ceil(float64(totalLectureCount) / hpSearchPageSize))

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
					searchURL, doc, err := h.fetchSearchPage(pageCtx, page, storeCode, storeName)
					if err != nil {
						return fmt.Errorf("%s 문화센터 강좌 목록 페이지 요청 중 오류가 발생하였습니다 (대상 점포: %s, 대상 페이지: %d): %w", h.name, storeName, page, err)
					}

					var extractErr error // goquery.Each() 내부에서 발생하는 에러를 담아두기 위한 변수입니다.
					var pageLectures []domain.Lecture

					sel := doc.Find("li > div.result_info_wrap")
					sel.Each(func(i int, s *goquery.Selection) {
						if extractErr != nil {
							return // 선행된 요소 파싱에서 에러가 발생한 경우, 후속 DOM 순회를 즉시 중단합니다.
						}

						lecture, err := h.extractLecture(pageCtx, s, storeName, searchURL)
						if err != nil {
							extractErr = fmt.Errorf("%s 문화센터 개별 강좌 데이터 정보 파싱 중 오류가 발생했습니다 (점포명: '%s', 대상 페이지: %d): %w", h.name, storeName, page, err)
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

// fetchSearchPage 홈플러스 강좌 검색 API를 호출하여 특정 페이지의 강좌 목록 HTML을 가져옵니다.
//
// 반환값:
//   - string: 요청에 사용된 검색 URL
//   - *goquery.Document: 파싱된 HTML DOM
//   - error: HTTP 요청 생성 또는 데이터 수신 중 발생한 오류
func (h *Homeplus) fetchSearchPage(ctx context.Context, page int, storeCode, storeName string) (string, *goquery.Document, error) {
	searchURL := fmt.Sprintf("%s/Lecture/GetSearchResult", h.cultureBaseURL)

	// 페이징 파라미터를 설정합니다.
	form := url.Values{}
	form.Set("page", strconv.Itoa(page))                 // 조회할 페이지 번호
	form.Set("pageSize", strconv.Itoa(hpSearchPageSize)) // 한 페이지에 표시할 강좌 수

	// 검색 필터 조건을 'prm[N]' 배열 파라미터 형식으로 조립합니다.
	// 첫 번째 조건(prm[0])은 고정값인 대상 점포이며, 이후 수집 대상 강좌군의 수만큼 인덱스를 증가시키며 조건을 추가합니다.
	var paramIdx = 0
	h.addSearchCondition(form, paramIdx, "", storeName, storeCode, "")
	for lectureGroupCode, lectureGroupName := range h.lectureGroups {
		paramIdx++
		h.addSearchCondition(form, paramIdx, "", lectureGroupName, "", lectureGroupCode)
	}

	// 키워드 검색 없이 전체 강좌를 조회하고, 기본 정렬(1) 기준으로 결과를 가져옵니다.
	form.Set("word", "")
	form.Set("sort", "1")

	// 폼 데이터를 URL 인코딩 문자열로 직렬화하여 요청 본문(io.Reader)을 구성하고, 이를 담은 HTTP POST 요청 객체를 생성합니다.
	reqBody := strings.NewReader(form.Encode())
	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("%s 문화센터 강좌 검색 페이지 조회를 위한 HTTP 요청 객체 생성 중 오류가 발생하였습니다: %w", h.name, err)
	}

	// Content-Type 헤더를 설정하여 서버가 요청 본문을 폼 인코딩 형식으로 해석하도록 지정합니다.
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	// HTTP 요청을 실행하고, 응답받은 HTML을 파싱하여 탐색 가능한 DOM 구조로 변환합니다.
	doc, err := h.fetcher.FetchHTML(req)
	if err != nil {
		return "", nil, fmt.Errorf("%s 문화센터 강좌 검색 페이지 데이터를 가져오는 중 내/외부 통신 오류가 발생하였습니다: %w", h.name, err)
	}

	return searchURL, doc, nil
}

// addSearchCondition 홈플러스 강좌 검색 API가 요구하는 'prm[N]' 배열 형식의 검색 조건 파라미터 한 블록을 form에 추가합니다.
// paramIdx는 배열의 인덱스(N)를 결정하며, 호출 측에서 0부터 시작하여 조건을 추가할 때마다 1씩 증가시켜야 합니다.
func (h *Homeplus) addSearchCondition(form url.Values, paramIdx int, id, text, storeCode, lectureGroupCode string) {
	// 'prm[N]' 형태의 파라미터 키 접두사를 구성합니다. (예: paramIdx=1 -> "prm[1]")
	prefix := fmt.Sprintf("prm[%d]", paramIdx)

	form.Set(prefix+"[Id]", id)                                // 조건 식별자
	form.Set(prefix+"[Txt]", text)                             // 조건의 표시 텍스트 (예: 점포명, 강좌군명)
	form.Set(prefix+"[Data][StoreCode]", storeCode)            // 점포 코드 필터
	form.Set(prefix+"[Data][LectureTarget]", lectureGroupCode) // 강좌군 코드 필터

	// 아래 필드들은 추가 필터 조건이나, 현재는 모두 사용하지 않아 빈 문자열로 전달합니다.
	form.Set(prefix+"[Data][LectureGroup]", "")
	form.Set(prefix+"[Data][LectureType]", "")
	form.Set(prefix+"[Data][LectureWeek]", "")
	form.Set(prefix+"[Data][ClassCount]", "")
	form.Set(prefix+"[Data][LectureTime]", "")
	form.Set(prefix+"[Data][LectureStatusSearch]", "")
	form.Set(prefix+"[Data][LectureStartMonth]", "")
	form.Set(prefix+"[Data][DeadLine]", "")
	form.Set(prefix+"[Data][Confirmed]", "")
	form.Set(prefix+"[Data][Discount]", "")
	form.Set(prefix+"[Data][LectureTimeGroup]", "")
	form.Set(prefix+"[Data][LectureAge]", "")
	form.Set(prefix+"[Data][LectureOnly]", "")
	form.Set(prefix+"[Data][WebTheme]", "")
	form.Set(prefix+"[Data][Description]", "")
}

// extractLecture 강좌 목록 페이지의 개별 강좌 DOM 요소(s)를 파싱하여 domain.Lecture 구조체로 변환하여 반환합니다.
func (h *Homeplus) extractLecture(ctx context.Context, s *goquery.Selection, storeName, searchURL string) (*domain.Lecture, error) {
	// ------------------------------------------------------------------
	// 1단계: 원시 데이터 파싱 (Raw Data Parsing)
	// ------------------------------------------------------------------

	// 1. 기본 정보 추출: 강좌 카테고리, 강좌명, 일정 정보 파싱
	rawCategory := strutil.NormalizeSpace(s.Find("div.title_1").Text()) // 형식: "Kids 전체"
	rawTitle := strutil.NormalizeSpace(s.Find("div.title_2").Text())    // 형식: "발레"
	rawSchedule := strutil.NormalizeSpace(s.Find("div.info_4").Text())  // 형식: "일 14:20 ~ 15:00"

	// 2. 상세 정보 추출: 'div.info_5' 블록에 담긴 3개의 데이터 파싱
	// 만약 블록의 개수가 3개가 아니라면 홈플러스 웹사이트의 UI 구조가 변경된 것으로 간주하여 즉시 에러를 반환합니다.
	infoBlocks := s.Find("div.info_5")
	if infoBlocks.Length() != 3 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱 중 구조적 오류가 감지되었습니다 (원인: 상세 정보 컬럼 개수 불일치, 기대값: 3, 실제값: %d, 대상 URL: %s)", h.name, infoBlocks.Length(), searchURL)
	}

	rawSessionAndPrice := strutil.NormalizeSpace(infoBlocks.Eq(0).Text()) // [0] 횟수/수강료 (형식: "1회 6,000원")
	rawStartDate := strutil.NormalizeSpace(infoBlocks.Eq(1).Text())       // [1] 개강일 기간 (형식: "2023.08.20 ~ 2023.11.19")
	rawInstructor := strutil.NormalizeSpace(infoBlocks.Eq(2).Text())      // [2] 강사명 (형식: "신혜정 강사")

	// ------------------------------------------------------------------
	// 2단계: 핵심 텍스트 필드 검증 (Validation)
	// ------------------------------------------------------------------
	// 카테고리명과 강좌명은 필터링·저장의 기준이 되는 핵심 식별자이므로, 가장 먼저 비어있음 여부를 확인합니다.
	// 이 필드들이 비어있는 경우, 이후의 정규식 파싱을 불필요하게 수행하지 않도록 최대한 일찍 에러를 반환합니다.

	if len(rawCategory) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 필수 식별자가 누락되었습니다 (원인: 강좌 카테고리(그룹명) 텍스트 부재, 대상 URL: %s)", h.name, searchURL)
	}
	category := rawCategory

	if len(rawTitle) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 필수 식별자가 누락되었습니다 (원인: 강좌명 텍스트 부재, 대상 URL: %s)", h.name, searchURL)
	}
	title := rawTitle

	// ------------------------------------------------------------------
	// 3단계: 정규식을 이용한 세부 필드 파싱 (Regex Parsing)
	// ------------------------------------------------------------------

	// 강사명: "신혜정 강사" 형식의 문자열에서 정규식을 통해 강사 정보를 추출합니다.
	instructor := strutil.NormalizeSpace(hpInstructorRegex.FindString(rawInstructor))
	if len(instructor) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 강사명 정규식 매칭 실패, 분석 데이터: '%s', 대상 URL: %s)", h.name, rawInstructor, searchURL)
	}

	// 개강일: "2023.08.20 ~ 2023.11.19" 형식에서 날짜를 추출한 뒤, 도메인 표준 형식인 "YYYY-MM-DD"로 변환합니다.
	// 정규식 매칭 결과의 끝 2자리(" ~")를 잘라내고, 구분자 "."을 "-"로 치환합니다.
	startDate := strutil.NormalizeSpace(hpStartDateRegex.FindString(rawStartDate))
	if len(startDate) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 개강일 정규식 매칭 실패, 분석 데이터: '%s', 대상 URL: %s)", h.name, rawStartDate, searchURL)
	}
	startDate = strings.ReplaceAll(startDate[:len(startDate)-2], ".", "-")

	// 시작·종료 시간: "일 14:20 ~ 15:00" 형식의 rawSchedule 문자열을 두 개의 정규식으로 각각 파싱합니다.
	// 매칭 결과에 포함된 불필요한 "~" 기호와 공백을 슬라이싱과 NormalizeSpace로 최종 정제합니다.
	startTime := hpStartTimeRegex.FindString(rawSchedule)
	endTime := hpEndTimeRegex.FindString(rawSchedule)
	if len(startTime) == 0 || len(endTime) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 시작/종료 시간 정규식 매칭 실패, 분석 데이터: '%s', 대상 URL: %s)", h.name, rawSchedule, searchURL)
	}
	startTime = strutil.NormalizeSpace(startTime[:len(startTime)-1]) // 끝의 " ~" 제거
	endTime = strutil.NormalizeSpace(endTime[1:])                    // 앞의 "~" 제거

	// 수업 요일: rawSchedule 맨 앞의 요일 한 글자(예: "일 ")를 추출합니다.
	weekday := strutil.NormalizeSpace(hpWeekdayRegex.FindString(rawSchedule))
	if len(weekday) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 수업 요일 정규식 매칭 실패, 분석 데이터: '%s', 대상 URL: %s)", h.name, rawSchedule, searchURL)
	}

	// 수강료 및 강좌 횟수: 두 정보가 "1회 6,000원" 형식으로 한 문자열에 혼재되어 있으므로 순서대로 파싱합니다.
	// 먼저 "(2인 기준)"과 같은 부가적인 인원 기준 표기를 제거하여 파싱 오류를 사전에 방지합니다.
	// (예: "1회 7,000원 (2인 기준)" => "1회 7,000원")
	rawSessionAndPrice = strutil.NormalizeSpace(hpPeopleStandardRegex.ReplaceAllString(rawSessionAndPrice, ""))

	// 수강료: 문자열 끝부분의 "원" 단위 금액을 추출합니다. (예: "1회 6,000원" => " 6,000원")
	price := strutil.NormalizeSpace(hpPriceRegex.FindString(rawSessionAndPrice))
	if len(price) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 수강료 정규식 매칭 실패, 분석 데이터: '%s', 대상 URL: %s)", h.name, rawSessionAndPrice, searchURL)
	}

	// 강좌 횟수: 문자열 앞부분의 "N회" 표기를 추출합니다. (예: "1회 6,000원" => "1회")
	sessionCount := strutil.NormalizeSpace(hpSessionCountRegex.FindString(rawSessionAndPrice))
	if len(sessionCount) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 강좌 횟수 정규식 매칭 실패, 분석 데이터: '%s', 대상 URL: %s)", h.name, rawSessionAndPrice, searchURL)
	}

	// ------------------------------------------------------------------
	// 4단계: 접수 상태 판별 (Reception Status Detection)
	// ------------------------------------------------------------------
	// 홈플러스는 강좌 신청 버튼(btn_class_cart)의 아이콘 이미지 파일명과 텍스트 라벨의 조합으로 접수 상태를 표현합니다.
	// 1차로 아이콘 URL에 따라 '활성(icon_cart_3)' / '비활성(icon_cart_4)' 계열로 분류하고,
	// 2차로 텍스트 라벨(statusLabel)을 통해 구체적인 상태(대기, 접수 가능, 마감, 방문, 문의)를 최종 결정합니다.

	statusIconURL, exists := s.Find("button.btn_class_cart > img").Attr("src")
	if !exists {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 상태 식별이 불가합니다 (원인: 접수 상태 아이콘(img) 요소 누락, 대상 URL: %s)", h.name, searchURL)
	}
	rawStatusLabel := strutil.NormalizeSpace(s.Find("button.btn_class_cart > span:last-child").Text())

	var receptionStatus = domain.ReceptionStatusUnknown
	switch statusIconURL {
	case "/images/ico/icon_cart_3.png": // 활성 계열 아이콘: 접수 가능 또는 대기 상태
		switch rawStatusLabel {
		case "대기":
			receptionStatus = domain.ReceptionStatusStandBy

		case "강의 장바구니 담기":
			receptionStatus = domain.ReceptionStatusPossible

		default:
			return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 미지원 상태가 감지되었습니다 (원인: 해석 불가한 접수 상태 라벨, 식별된 아이콘: '%s', 식별된 라벨: '%s', 대상 URL: %s)", h.name, statusIconURL, rawStatusLabel, searchURL)
		}

	case "/images/ico/icon_cart_4.png": // 비활성 계열 아이콘: 마감, 방문 상담, 전화 문의 상태
		switch rawStatusLabel {
		case "마감":
			receptionStatus = domain.ReceptionStatusClosed

		case "방문":
			receptionStatus = domain.ReceptionStatusOnsiteConsultation

		case "문의":
			receptionStatus = domain.ReceptionStatusOnsiteInquiry

		default:
			return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 미지원 상태가 감지되었습니다 (원인: 해석 불가한 접수 상태 라벨, 식별된 아이콘: '%s', 식별된 라벨: '%s', 대상 URL: %s)", h.name, statusIconURL, rawStatusLabel, searchURL)
		}

	default: // 알 수 없는 아이콘: 홈플러스가 신규 상태를 추가했거나 HTML 구조가 변경된 경우
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 미지원 상태가 감지되었습니다 (원인: 등록되지 않은 상태 아이콘 URL, 식별된 아이콘: '%s', 식별된 라벨: '%s', 대상 URL: %s)", h.name, statusIconURL, rawStatusLabel, searchURL)
	}

	// ------------------------------------------------------------------
	// 5단계: 상세 페이지 URL 조립을 위한 강좌 고유 식별자 추출
	// ------------------------------------------------------------------

	lectureMasterID, exists := s.Find("input[name=LectureMasterID]").Attr("value")
	if !exists {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 필수 식별자가 누락되었습니다 (원인: 강좌 고유 식별자(LectureMasterID) 요소 부재, 대상 URL: %s)", h.name, searchURL)
	}

	// ------------------------------------------------------------------
	// 6단계: 컨텍스트 취소 여부 최종 확인
	// ------------------------------------------------------------------
	// 모든 데이터 파싱이 완료된 시점에서 확인하여, 취소된 컨텍스트에 대해 domain.Lecture 객체 생성을 생략합니다.

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// ------------------------------------------------------------------
	// 7단계: 최종 도메인 모델 생성 및 반환
	// ------------------------------------------------------------------

	return &domain.Lecture{
		StoreName:      fmt.Sprintf("%s %s", h.name, storeName),
		Category:       category,
		Title:          title,
		Instructor:     instructor,
		StartDate:      startDate,
		StartTime:      startTime,
		EndTime:        endTime,
		Weekday:        fmt.Sprintf("%s요일", weekday),
		Price:          price,
		SessionCount:   sessionCount,
		Status:         receptionStatus,
		DetailPageURL:  h.buildDetailPageURL(lectureMasterID),
		ScrapeExcluded: false,
	}, nil
}

// buildDetailPageURL 강좌 고유 식별자를 받아 해당 강좌의 상세 페이지 URL을 조립하여 반환합니다.
func (h *Homeplus) buildDetailPageURL(lectureMasterID string) string {
	return fmt.Sprintf("%s/Lecture/GetLectureDetail?lectureMasterID=%s", h.cultureBaseURL, lectureMasterID)
}
