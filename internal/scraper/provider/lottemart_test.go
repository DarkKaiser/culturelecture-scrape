package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
)

// setupLottemartMockServer는 테스트용 로컬 HTTP 서버와 해당 서버를 바라보도록 설정된 Lottemart 인스턴스를 반환합니다.
// 반환된 서버는 테스트 종료 시 반드시 Close()를 호출해야 합니다.
func setupLottemartMockServer(handler http.HandlerFunc) (*httptest.Server, *Lottemart) {
	ts := httptest.NewServer(handler)

	l, _ := NewLottemart(scraper.SearchCriteria{
		SearchYear:       "2024",
		SearchSeasonCode: "1",
	})
	
	// 테스트용 로컬 서버로 요청을 보내도록 기본 URL을 덮어씌웁니다.
	l.cultureBaseURL = ts.URL
	return ts, l
}

func TestNewLottemart(t *testing.T) {
	criteria := scraper.SearchCriteria{
		SearchYear:       "2024",
		SearchSeasonCode: "1", // 봄학기
	}

	l, err := NewLottemart(criteria)

	if err != nil {
		t.Fatalf("NewLottemart() 예기치 않은 에러 발생: %v", err)
	}

	if l == nil {
		t.Fatal("NewLottemart()가 nil을 반환했습니다.")
	}

	if l.name != "롯데마트" {
		t.Errorf("NewLottemart() 스크래퍼 이름 = %s, 기대값 = %s", l.name, "롯데마트")
	}

	if len(l.stores) == 0 {
		t.Error("NewLottemart() 수집 대상 점포 목록이 비어 있습니다.")
	}

	if len(l.lectureGroups) == 0 {
		t.Error("NewLottemart() 수집 대상 강좌군 목록이 비어 있습니다.")
	}
}

func TestLottemart_validateStore(t *testing.T) {
	tests := []struct {
		name       string
		storeCode  string
		storeName  string
		handler    http.HandlerFunc
		wantResult bool
		wantErr    bool
	}{
		{
			name:      "성공 - 점포 존재",
			storeCode: "0038",
			storeName: "송파점",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				// Test용 더미 HTML 응답 (점포명이 포함된 h3 태그)
				w.Write([]byte(`
					<html><body>
						<div id="contents">
							<div class="branch_main-wrap">
								<div class="branch_info-area">
									<div class="branch_spot-area">
										<h3>송파점</h3>
									</div>
								</div>
							</div>
						</div>
					</body></html>
				`))
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name:      "실패 - 점포 텍스트 불일치",
			storeCode: "0038",
			storeName: "송파점",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`
					<html><body>
						<div id="contents">
							<div class="branch_main-wrap">
								<div class="branch_info-area">
									<div class="branch_spot-area">
										<h3>양평점</h3>
									</div>
								</div>
							</div>
						</div>
					</body></html>
				`))
			},
			wantResult: false,
			wantErr:    false,
		},
		{
			name:      "에러 - HTTP 상태 코드 오류",
			storeCode: "0038",
			storeName: "송파점",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantResult: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, l := setupLottemartMockServer(tt.handler)
			defer ts.Close()

			got, err := l.validateStore(context.Background(), tt.storeCode, tt.storeName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Lottemart.validateStore() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantResult {
				t.Errorf("Lottemart.validateStore() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestLottemart_validateLectureGroups(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		lectureGroups map[string]map[string]string
		wantResult    bool
		wantErr       bool
	}{
		{
			name: "성공 - 카테고리 구성 일치",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				// 대상 엘리먼트가 존재하는 HTML 구조
				w.Write([]byte(`
					<html><body>
						<div class="wrapper1">
							<div class="wrapper2">
								<div class="wrapper3">
									<div id="baby-tit" class="tab-pane active"> <!-- 실제 lottemart.go가 여기서 시작 -->
									</div> <!-- 임시 탭 태그 -->
									<dd>
										<ul>
											<li><div><input value="21"> 음악감성</div></li>
											<li><div><input value="81"></div></li>
										</ul>
									</dd>
								</div>
							</div>
						</div>
					</body></html>
				`))
			},
			lectureGroups: map[string]map[string]string{
				"baby-tit": {
					"21": "음악감성",
					"81": "", // 명칭 없음 (존재 여부만 확인)
				},
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name: "실패 - 탭 ID를 찾을 수 없음",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`<html><body></body></html>`))
			},
			lectureGroups: map[string]map[string]string{
				"baby-tit": {
					"21": "음악감성",
				},
			},
			wantResult: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, l := setupLottemartMockServer(tt.handler)
			defer ts.Close()

			l.lectureGroups = tt.lectureGroups

			got, err := l.validateLectureGroups(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Lottemart.validateLectureGroups() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantResult {
				t.Errorf("Lottemart.validateLectureGroups() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestLottemart_fetchSearchPage(t *testing.T) {
	ts, l := setupLottemartMockServer(func(w http.ResponseWriter, r *http.Request) {
		// HTTP 응답 검증
		err := r.ParseForm()
		if err != nil {
			t.Fatalf("요청 데이터 파싱 실패: %v", err)
		}
		
		// 1. 기본 파라미터 확인
		if r.Form.Get("currPageNo") != "2" {
			t.Errorf("예상된 currPageNo 파라미터: 2, 수신: %s", r.Form.Get("currPageNo"))
		}
		if r.Form.Get("search_str_cd") != "0038" {
			t.Errorf("예상된 search_str_cd 파라미터: 0038, 수신: %s", r.Form.Get("search_str_cd"))
		}
		if r.Form.Get("search_term_cd") != "202401" {
			t.Errorf("예상된 search_term_cd 파라미터: 202401, 수신: %s", r.Form.Get("search_term_cd"))
		}

		// 2. 강좌군 배열 검증 (arr_cat_cd는 POST 바디 내에 배열 형태로 존재해야 함)
		arrCatCd := r.PostForm["arr_cat_cd"] // 여러 값을 가진 리스트 반환
		if len(arrCatCd) != 2 || (arrCatCd[0] != "11" && arrCatCd[0] != "22") { // 순서는 맵 순회이므로 보장할 수 없으나 2개 존재 확인
			t.Errorf("arr_cat_cd 배열이 예상과 다릅니다. 받은 값: %v", arrCatCd)
		}

		// 3. 단일 카테고리 코드 처리 방식 검증 (하나의 문자열로 전송)
		searchCatCd := r.Form.Get("search_cat_cd")
		if !strings.Contains(searchCatCd, "11") || !strings.Contains(searchCatCd, "22") {
			t.Errorf("search_cat_cd 단일 문자열 처리 오류: 수신: %s, 기대값에 11, 22 포함되야함", searchCatCd)
		}
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><div class="result">success</div></body></html>`))
	})
	defer ts.Close()

	l.stores = map[string]string{"0038": "송파점"}
	l.lectureGroups = map[string]map[string]string{
		"category1": {
			"11": "강좌분류1",
			"22": "강좌분류2",
		},
	}

	searchURL, doc, err := l.fetchSearchPage(context.Background(), 2, "0038")

	if err != nil {
		t.Fatalf("Lottemart.fetchSearchPage() 예기치 않은 에러: %v", err)
	}

	if searchURL == "" {
		t.Error("요청 URL이 반환되지 않았습니다.")
	}

	if doc == nil {
		t.Fatal("goquery Document가 nil로 반환되었습니다.")
	}

	if doc.Find("div.result").Text() != "success" {
		t.Errorf("HTML DOM 파싱 오류: 기대값 = success, 실제값 = %s", doc.Find("div.result").Text())
	}
}

func TestLottemart_Scrape(t *testing.T) {
	// Scrape()는 사전 단계(전체 페이지 수 조회) -> 병렬 데이터 수집 단계를 거칩니다.
	ts, l := setupLottemartMockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		err := r.ParseForm()
		if err != nil {
			t.Fatalf("요청 데이터 파싱 실패: %v", err)
		}
		pageNum := r.Form.Get("currPageNo")

		if pageNum == "1" {
			// 테스트 환경에서는 pageinfo 속성을 통해 총 1페이지(5개 강좌)가 있다고 가정합니다.
			w.Write([]byte(`
				<html><body>
					<table>
						<tbody>
							<tr pageinfo="1|1|5|3|0|2">
								<td><div class="info-txt"><a href="#" onclick="javascript:goDetail('L1234', '1111')">기구 필라테스</a></div></td>
								<td>홍길동</td>
								<td>2024.03.01(월) 10:00~11:00</td>
								<td>10회 150,000원</td>
								<td><div><div><a class="btn-status">바로신청</a></div></div></td>
							</tr>
						</tbody>
					</table>
				</body></html>
			`))
		} else {
			// 페이지가 1이 아닌 비정상 시나리오 (데이터 없음)
			w.Write([]byte(`<html><body><table><tbody></tbody></table></body></html>`))
		}
	})
	defer ts.Close()

	// 테스트 대상 점포 및 강좌군 설정
	l.stores = map[string]string{"0038": "송파점"}
	l.lectureGroups = map[string]map[string]string{
		"category1": {
			"11": "기구필라",
		},
	}

	lectures, err := l.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Lottemart.Scrape() 예상치 못한 에러: %v", err)
	}

	if len(lectures) != 1 {
		t.Errorf("Lottemart.Scrape() 수집된 강좌 수 = %d, 기대값 = 1", len(lectures))
	}

	if len(lectures) > 0 && lectures[0].Title != "기구 필라테스" {
		t.Errorf("수집된 데이터 불일치: 기대 이름 = 기구 필라테스, 실제 = %s", lectures[0].Title)
	}
}

func TestLottemart_extractLecture(t *testing.T) {
	l, _ := NewLottemart(scraper.SearchCriteria{SearchYear: "2024", SearchSeasonCode: "1"})
	storeCode := "0038"
	storeName := "송파점"
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
				<tr>
					<td><div class="info-txt"><a href="#" onclick="javascript:goDetail('CODE123', 'A1', 'B2')">성인 발레 피트니스</a></div></td>
					<td>김연아</td>
					<td>2024.04.15(목) 19:30~20:20</td>
					<td>8회 100,000원</td>
					<td><div><div><a class="btn-status">바로신청</a></div></div></td>
				</tr>`,
			wantTitle:  "성인 발레 피트니스",
			wantStatus: domain.ReceptionStatusPossible,
			wantErr:    false,
		},
		{
			name: "성공 - 대기자 신청 및 정규식 패턴 여백 포함",
			htmlSnippet: `
				<tr>
					<td><div class="info-txt"><a href="#" onclick="javascript:goDetail('CODE456', 'A1', 'B2')">  엄마랑 아가랑 동화 구연  </a></div></td>
					<td>  이순신  </td>
					<td>  2024.05.01(수)   14:00~15:00  </td>
					<td>  12회   50,000원   </td>
					<td><div><div><a class="btn-status">대기자 신청</a></div></div></td>
				</tr>`,
			wantTitle:  "엄마랑 아가랑 동화 구연",
			wantStatus: domain.ReceptionStatusStandBy,
			wantErr:    false,
		},
		{
			name: "성공 - 접수마감 (롯데마트 현장접수=현장문의 동일)",
			htmlSnippet: `
				<tr>
					<td><div class="info-txt"><a href="#" onclick="javascript:goDetail('CODE999', 'A1')">초보 요리교실</a></div></td>
					<td>백종원</td>
					<td>2024.06.01(토) 11:30~13:30</td>
					<td>4회 80,000원</td>
					<td><div><div><a class="btn-status">현장접수</a></div></div></td>
				</tr>`,
			wantTitle:  "초보 요리교실",
			wantStatus: domain.ReceptionStatusOnsiteInquiry,
			wantErr:    false,
		},
		{
			name: "실패 - 컬럼 부족 (HTML 테이블 구조 변경)",
			htmlSnippet: `
				<tr>
					<td><div class="info-txt"><a href="#" onclick="javascript:goDetail('CODE123')">강좌명만 있음</a></div></td>
				</tr>`,
			wantErr: true,
		},
		{
			name: "실패 - 강좌명 (titleAnchorNode) <a> 태그 누락 (구조적 오류)",
			htmlSnippet: `
				<tr>
					<td><div class="info-txt"><a>태그가 없음</a></div></td>
					<td>홍길동</td>
					<td>2024.04.15(목) 19:30~20:20</td>
					<td>8회 100,000원</td>
					<td><div><div><a class="btn-status">바로신청</a></div></div></td>
				</tr>`,
			wantErr: true, // onclick등 필수 속성 누락
		},
		{
			name: "실패 - 개강일 데이터 형식 오류 (규격 불일치)",
			htmlSnippet: `
				<tr>
					<td><div class="info-txt"><a href="#" onclick="javascript:goDetail('CODE999')">에러 테스트 강좌</a></div></td>
					<td>강사명</td>
					<td>비정상적인날짜형식(수) 10:00~11:00</td>
					<td>4회 80,000원</td>
					<td><div><div><a class="btn-status">접수마감</a></div></div></td>
				</tr>`,
			wantErr: true,
		},
		{
			name: "실패 - 수강료 데이터 형식 오류 (규격 불일치)",
			htmlSnippet: `
				<tr>
					<td><div class="info-txt"><a href="#" onclick="javascript:goDetail('CODE999')">에러 테스트 강좌</a></div></td>
					<td>강사명</td>
					<td>2024.12.31(수) 23:00~23:59</td>
					<td>4회 팔만원</td> <!-- "원" 앞의 숫자가 없음 -->
					<td><div><div><a class="btn-status">접수마감</a></div></div></td>
				</tr>`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// tr 태그만 단독으로 있으면 파싱이 제대로 안 되므로 table과 tbody로 감싸줍니다.
			wrappedHTML := fmt.Sprintf(`<html><body><table><tbody>%s</tbody></table></body></html>`, tt.htmlSnippet)
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(wrappedHTML))
			if err != nil {
				t.Fatalf("goquery 테스트 문서 로드 실패: %v", err)
			}
			
			// 첫 번째 tr 레코드 추출
			s := doc.Find("tr").First()
			if s.Length() == 0 {
				t.Fatalf("테스트용 DOM 구성 오류. HTML: %s", wrappedHTML)
			}

			lecture, err := l.extractLecture(context.Background(), s, storeCode, storeName, searchURL)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("Lottemart.extractLecture() error = %v, wantErr %v", err, tt.wantErr)
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

