package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
)

// setupMockServer는 테스트용 로컬 HTTP 서버와 해당 서버를 바라보도록 설정된 Homeplus 인스턴스를 반환합니다.
// 반환된 서버는 테스트 종료 시 반드시 Close()를 호출해야 합니다.
func setupMockServer(handler http.HandlerFunc) (*httptest.Server, *Homeplus) {
	ts := httptest.NewServer(handler)

	h, _ := NewHomeplus(scraper.SearchCriteria{
		SearchYear:       "2024",
		SearchSeasonCode: "1",
	})
	
	// 테스트용 로컬 서버로 요청을 보내도록 기본 URL을 덮어씌웁니다.
	h.cultureBaseURL = ts.URL
	return ts, h
}

func TestNewHomeplus(t *testing.T) {
	criteria := scraper.SearchCriteria{
		SearchYear:       "2023",
		SearchSeasonCode: "2",
	}

	h, err := NewHomeplus(criteria)

	if err != nil {
		t.Fatalf("NewHomeplus() 예기치 않은 에러 발생: %v", err)
	}

	if h == nil {
		t.Fatal("NewHomeplus()가 nil을 반환했습니다.")
	}

	if h.name != "홈플러스" {
		t.Errorf("NewHomeplus() 스크래퍼 이름 = %s, 기대값 = %s", h.name, "홈플러스")
	}

	if len(h.stores) == 0 {
		t.Error("NewHomeplus() 수집 대상 점포 목록이 비어 있습니다.")
	}

	if len(h.lectureGroups) == 0 {
		t.Error("NewHomeplus() 수집 대상 강좌군 목록이 비어 있습니다.")
	}
}

func TestHomeplus_validateStores(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		stores     map[string]string // 테스트에 주입할 점포 정보
		wantResult bool
		wantErr    bool
	}{
		{
			name: "성공 - 모든 점포 존재",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				// Test용 더미 JSON 응답 (0030, 0035 점포 포함)
				w.Write([]byte(`{
					"RstCode": 0,
					"Data": {
						"StoreList": [
							{"StoreCode": "0030", "StoreName": "순천점"},
							{"StoreCode": "0035", "StoreName": "광양점"},
							{"StoreCode": "9999", "StoreName": "기타점"}
						]
					}
				}`))
			},
			stores: map[string]string{
				"0035": "광양점",
				"0030": "순천점",
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name: "실패 - 일부 점포 누락",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				// 순천점이 없는 응답
				w.Write([]byte(`{
					"RstCode": 0,
					"Data": {
						"StoreList": [
							{"StoreCode": "0035", "StoreName": "광양점"}
						]
					}
				}`))
			},
			stores: map[string]string{
				"0035": "광양점",
				"0030": "순천점", // 응답에 없음
			},
			wantResult: false,
			wantErr:    false,
		},
		{
			name: "에러 - HTTP 오류 응답",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			stores: map[string]string{
				"0030": "순천점",
			},
			wantResult: false,
			wantErr:    true,
		},
		{
			name: "에러 - JSON 파싱 실패",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{ invalid json }`))
			},
			stores: map[string]string{
				"0030": "순천점",
			},
			wantResult: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, h := setupMockServer(tt.handler)
			defer ts.Close()

			h.stores = tt.stores // 테스트용 점포 데이터 주입

			got, err := h.validateStores(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Homeplus.validateStores() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantResult {
				t.Errorf("Homeplus.validateStores() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestHomeplus_validateLectureGroups(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		lectureGroups map[string]string
		wantResult    bool
		wantErr       bool
	}{
		{
			name: "성공 - 카테고리 모두 존재",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				// 대상 버튼이 존재하는 최소 HTML 구조. 실제 서비스는 이런 depth를 갖습니다.
				w.Write([]byte(`
					<html><body>
					<section class="search_body">
						<div class="menu_depth_2_wrap">
							<ul class="tree_menu_2">
								<li class="depth_2">
									<ul class="depth_3">
										<li><button data-lecture-target="MH|EL|IF">Kids 전체</button></li>
									</ul>
								</li>
								<li class="depth_2">
									<ul class="depth_3">
										<li><button data-lecture-target="BB">Baby 전체</button></li>
									</ul>
								</li>
							</ul>
						</div>
					</section>
					</body></html>
				`))
			},
			lectureGroups: map[string]string{
				"MH|EL|IF": "Kids 전체",
				"BB":       "Baby 전체",
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name: "실패 - 카테고리 텍스트 불일치",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`
					<html><body>
					<section class="search_body">
						<div class="menu_depth_2_wrap">
							<ul class="tree_menu_2">
								<li class="depth_2">
									<ul class="depth_3">
										<li><button data-lecture-target="MH|EL|IF">키즈 모두</button></li> <!-- 텍스트 다름 -->
									</ul>
								</li>
								<li class="depth_2">
									<ul class="depth_3">
										<li><button data-lecture-target="BB">Baby 전체</button></li>
									</ul>
								</li>
							</ul>
						</div>
					</section>
					</body></html>
				`))
			},
			lectureGroups: map[string]string{
				"MH|EL|IF": "Kids 전체",
				"BB":       "Baby 전체",
			},
			wantResult: false,
			wantErr:    false,
		},
		{
			name: "실패 - 특정 서브 카테고리 요소가 아님",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				// li:first-child 규칙을 벗어난 HTML 구조 (버튼이 첫번째 자식이 아님)
				w.Write([]byte(`
					<html><body>
					<section class="search_body">
						<div class="menu_depth_2_wrap">
							<ul class="tree_menu_2">
								<li class="depth_2">
									<ul class="depth_3">
										<li>다른 텍스트 노드 또는 기능</li>
										<li><button data-lecture-target="MH|EL|IF">Kids 전체</button></li> <!-- 두번째 li이므로 매칭 안됨 -->
									</ul>
								</li>
							</ul>
						</div>
					</section>
					</body></html>
				`))
			},
			lectureGroups: map[string]string{
				"MH|EL|IF": "Kids 전체",
			},
			wantResult: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, h := setupMockServer(tt.handler)
			defer ts.Close()

			h.lectureGroups = tt.lectureGroups // 테스트 데이터 주입

			got, err := h.validateLectureGroups(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Homeplus.validateLectureGroups() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantResult {
				t.Errorf("Homeplus.validateLectureGroups() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestHomeplus_Validate(t *testing.T) {
	// 통합 Validate 메서드는 validateStores와 validateLectureGroups가 순차적으로 호출됩니다.
	// 두 검증이 모두 성공하는 시나리오와, 둘 중 하나라도 실패하는 시나리오를 모킹합니다.
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		stores     map[string]string
		groups     map[string]string
		wantErr    bool
	}{
		{
			name: "성공 - 두 검증 모두 성공",
			handler: func(w http.ResponseWriter, r *http.Request) {
				// URI 매핑을 통한 분기 응답 처리
				if r.URL.Path == "/Store/GetStoreList" {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"RstCode": 0, "Data": {"StoreList": [{"StoreCode": "0030", "StoreName": "순천점"}]}}`))
				} else if r.URL.Path == "/Lecture/Search" {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`<html><body><section class="search_body"><div class="menu_depth_2_wrap"><ul class="tree_menu_2"><li class="depth_2"><ul class="depth_3">
						<li><button data-lecture-target="BB">Baby 전체</button></li>
					</ul></li></ul></div></section></body></html>`))
				}
			},
			stores: map[string]string{"0030": "순천점"},
			groups: map[string]string{"BB": "Baby 전체"},
			wantErr: false,
		},
		{
			name: "실패 - 점포 검증 실패 (강좌 검증은 실행 안됨)",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Store/GetStoreList" {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"RstCode": 0, "Data": {"StoreList": [{"StoreCode": "9999", "StoreName": "기타점"}]}}`))
				}
			},
			stores: map[string]string{"0030": "순천점"},
			groups: map[string]string{"BB": "Baby 전체"},
			wantErr: true,
		},
		{
			name: "실패 - 강좌 검증 실패",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Store/GetStoreList" {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"RstCode": 0, "Data": {"StoreList": [{"StoreCode": "0030", "StoreName": "순천점"}]}}`))
				} else if r.URL.Path == "/Lecture/Search" {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.WriteHeader(http.StatusOK)
					// 카테고리 누락된 HTML
					w.Write([]byte(`<html><body></body></html>`))
				}
			},
			stores: map[string]string{"0030": "순천점"},
			groups: map[string]string{"BB": "Baby 전체"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, h := setupMockServer(tt.handler)
			defer ts.Close()

			h.stores = tt.stores
			h.lectureGroups = tt.groups

			err := h.Validate(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Homeplus.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHomeplus_fetchSearchPage(t *testing.T) {
	ts, h := setupMockServer(func(w http.ResponseWriter, r *http.Request) {
		// 요청 파라미터(Form) 검증
		r.ParseForm()
		if r.Form.Get("page") != "1" {
			t.Errorf("예상된 page 파라미터: 1, 수신: %s", r.Form.Get("page"))
		}
		if r.Form.Get("prm[0][Txt]") != "순천점" {
			t.Errorf("예상된 0번째 조건 텍스트: 순천점, 수신: %s", r.Form.Get("prm[0][Txt]"))
		}
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><div id="dummy">success</div></body></html>`))
	})
	defer ts.Close()

	h.stores = map[string]string{"0030": "순천점"}
	h.lectureGroups = map[string]string{"BB": "Baby 전체"}

	searchURL, doc, err := h.fetchSearchPage(context.Background(), 1, "0030", "순천점")

	if err != nil {
		t.Fatalf("Homeplus.fetchSearchPage() 예기치 않은 에러: %v", err)
	}

	if searchURL == "" {
		t.Error("요청 URL이 반환되지 않았습니다.")
	}

	if doc == nil {
		t.Fatal("goquery Document가 nil로 반환되었습니다.")
	}

	if doc.Find("#dummy").Text() != "success" {
		t.Errorf("HTML DOM 파싱 오류: 기대값 = success, 실제값 = %s", doc.Find("#dummy").Text())
	}
}

func TestHomeplus_Scrape(t *testing.T) {
	// Scrape()는 사전 단계(전체 페이지 수 조회) -> 병렬 데이터 수집 단계를 거칩니다.
	// 이 복합 흐름을 모킹하기 위해 요청된 페이지가 무엇인지에 따라 응답을 다르게 처리합니다.
	ts, h := setupMockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		r.ParseForm()
		pageNum := r.Form.Get("page")

		if pageNum == "1" {
			// 테스트 환경에서는 15개의 강좌가 있다고 가정 (hpSearchPageSize가 20이므로 총 1페이지 분량)
			w.Write([]byte(`
				<html><body>
					<div id="divTotalCnt">15</div>
					<ul>
						<li class="result_info_wrap">
							<div class="result_info_wrap"> <!-- extractLecture에 필요한 최소 DOM 구조 -->
								<div class="title_1">Kids 전체</div>
								<div class="title_2">테스트 강좌</div>
								<div class="info_4">일 14:20 ~ 15:00</div>
								<div class="info_5">1회 6,000원</div>
								<div class="info_5">2023.08.20 ~ 2023.11.19</div>
								<div class="info_5">홍길동 강사</div>
								<button class="btn_class_cart"><img src="/images/ico/icon_cart_3.png"><span>강의 장바구니 담기</span></button>
								<input name="LectureMasterID" value="TEST_ID_1">
							</div>
						</li>
					</ul>
				</body></html>
			`))
		} else {
			// 페이지가 1이 아닌 비정상 시나리오
			w.Write([]byte(`<html><body><div id="divTotalCnt">0</div></body></html>`))
		}
	})
	defer ts.Close()

	// 테스트 대상 점포 및 강좌군 축소
	h.stores = map[string]string{"0030": "순천점"}
	h.lectureGroups = map[string]string{"BB": "Baby 전체"}

	lectures, err := h.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Homeplus.Scrape() 예상치 못한 에러: %v", err)
	}

	if len(lectures) != 1 {
		t.Errorf("Homeplus.Scrape() 수집된 강좌 수 = %d, 기대값 = 1", len(lectures))
	}

	if len(lectures) > 0 && lectures[0].Title != "테스트 강좌" {
		t.Errorf("수집된 데이터 불일치: 기대 이름 = 테스트 강좌, 실제 = %s", lectures[0].Title)
	}
}

func TestHomeplus_extractLecture(t *testing.T) {
	// extractLecture는 goquery.Selection을 받아 파싱하므로, 
	// 각 시나리오별로 적절한 DOM 조각을 구성하여 테스트합니다.
	h, _ := NewHomeplus(scraper.SearchCriteria{SearchYear: "2024", SearchSeasonCode: "1"})
	storeName := "테스트 점포"
	searchURL := "http://test.com"

	tests := []struct {
		name        string
		htmlSnippet string
		wantTitle   string
		wantStatus  domain.ReceptionStatus
		wantErr     bool
	}{
		{
			name: "성공 - 완벽한 데이터 파싱 (접수 가능)",
			htmlSnippet: `
			<li class="result_info_wrap">
				<div class="result_info_wrap">
					<div class="title_1">Kids 전체</div>
					<div class="title_2">어린이 수영</div>
					<div class="info_4">토 14:00 ~ 15:00</div>
					<div class="info_5">4회 40,000원</div>
					<div class="info_5">2023.09.01 ~ 2023.09.30</div>
					<div class="info_5">박태환 강사</div>
					<button class="btn_class_cart"><img src="/images/ico/icon_cart_3.png"><span>강의 장바구니 담기</span></button>
					<input name="LectureMasterID" value="TEST1234">
				</div>
			</li>`,
			wantTitle:  "어린이 수영",
			wantStatus: domain.ReceptionStatusPossible,
			wantErr:    false,
		},
		{
			name: "성공 - 인원 기준 텍스트 제거 성공 및 대기 상태",
			htmlSnippet: `
			<li class="result_info_wrap">
				<div class="result_info_wrap">
					<div class="title_1">Baby 전체</div>
					<div class="title_2">오감놀이</div>
					<div class="info_4">월 10:00 ~ 11:00</div>
					<div class="info_5">1회 15,000원 (2인 기준)</div> <!-- 인원 기준 포함 -->
					<div class="info_5">2023.10.01 ~ 2023.10.28</div>
					<div class="info_5">김아무개 강사</div>
					<button class="btn_class_cart"><img src="/images/ico/icon_cart_3.png"><span>대기</span></button>
					<input name="LectureMasterID" value="TEST5678">
				</div>
			</li>`,
			wantTitle:  "오감놀이",
			wantStatus: domain.ReceptionStatusStandBy,
			wantErr:    false,
		},
		{
			name: "성공 - 접수 마감 상태 매핑",
			htmlSnippet: `
			<li class="result_info_wrap">
				<div class="result_info_wrap">
					<div class="title_1">성인 전체</div>
					<div class="title_2">필라테스</div>
					<div class="info_4">수 19:00 ~ 20:00</div>
					<div class="info_5">10회 150,000원</div>
					<div class="info_5">2023.11.01 ~ 2023.12.31</div>
					<div class="info_5">이효리 강사</div>
					<button class="btn_class_cart"><img src="/images/ico/icon_cart_4.png"><span>마감</span></button>
					<input name="LectureMasterID" value="TEST9999">
				</div>
			</li>`,
			wantTitle:  "필라테스",
			wantStatus: domain.ReceptionStatusClosed,
			wantErr:    false,
		},
		{
			name: "실패 - 카테고리 누락",
			htmlSnippet: `
			<li class="result_info_wrap">
				<div class="result_info_wrap">
					<div class="title_1"></div> <!-- 카테고리 없음 -->
					<div class="title_2">필라테스</div>
					<div class="info_4">수 19:00 ~ 20:00</div>
					<div class="info_5">10회 150,000원</div>
					<div class="info_5">...</div>
					<div class="info_5">...</div>
					<button class="btn_class_cart"><img src="/images/ico/icon_cart_4.png"><span>마감</span></button>
					<input name="LectureMasterID" value="TEST9999">
				</div>
			</li>`,
			wantErr: true,
		},
		{
			name: "실패 - 구조 변경 (info_5 컬럼 갯수 불일치)",
			htmlSnippet: `
			<li class="result_info_wrap">
				<div class="result_info_wrap">
					<div class="title_1">성인 전체</div>
					<div class="title_2">필라테스</div>
					<div class="info_4">수 19:00 ~ 20:00</div>
					<div class="info_5">10회 150,000원</div>
					<div class="info_5">2023.11.01 ~ 2023.12.31</div>
					<!-- 강사명 누락되어 info_5가 2개뿐임 -->
					<button class="btn_class_cart"><img src="/images/ico/icon_cart_4.png"><span>마감</span></button>
					<input name="LectureMasterID" value="TEST9999">
				</div>
			</li>`,
			wantErr: true,
		},
		{
			name: "실패 - 미지원 상태 라벨",
			htmlSnippet: `
			<li class="result_info_wrap">
				<div class="result_info_wrap">
					<div class="title_1">성인 전체</div>
					<div class="title_2">필라테스</div>
					<div class="info_4">수 19:00 ~ 20:00</div>
					<div class="info_5">10회 150,000원</div>
					<div class="info_5">2023.11.01 ~ 2023.12.31</div>
					<div class="info_5">이효리 강사</div>
					<button class="btn_class_cart"><img src="/images/ico/icon_cart_3.png"><span>이상한상태</span></button>
					<input name="LectureMasterID" value="TEST9999">
				</div>
			</li>`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.htmlSnippet))
			if err != nil {
				t.Fatalf("goquery 테스트 문서 로드 실패: %v", err)
			}
			
			// 첫 번째 강좌 블록 추출
			s := doc.Find("div.result_info_wrap").First()
			if s.Length() == 0 {
				t.Fatalf("테스트용 DOM 구성 오류")
			}

			lecture, err := h.extractLecture(context.Background(), s, storeName, searchURL)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("Homeplus.extractLecture() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && lecture != nil {
				if lecture.Title != tt.wantTitle {
					t.Errorf("Title 불일치: 기대 = %s, 실제 = %s", tt.wantTitle, lecture.Title)
				}
				if lecture.Status != tt.wantStatus {
					t.Errorf("Status 불일치: 기대 = %d, 실제 = %d", tt.wantStatus, lecture.Status)
				}
			}
		})
	}
}
