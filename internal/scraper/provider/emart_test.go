package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
)

// setupEmartMockServer는 테스트용 로컬 HTTP 서버와 해당 서버를 바라보도록 설정된 Emart 인스턴스를 반환합니다.
// 반환된 서버는 테스트 종료 시 반드시 Close()를 호출해야 합니다.
func setupEmartMockServer(handler http.HandlerFunc) (*httptest.Server, *Emart) {
	ts := httptest.NewServer(handler)

	// API 키는 테스트에서 검증 목적으로만 사용되므로 임의의 값을 주입합니다.
	e, _ := NewEmart(scraper.SearchCriteria{
		SearchYear:       "2024",
		SearchSeasonCode: "1",
	}, "test-api-key")

	// 테스트용 로컬 서버로 GraphQL 요청을 보내도록 기본 URL을 덮어씌웁니다.
	e.apiBaseURL = ts.URL
	return ts, e
}

func TestNewEmart(t *testing.T) {
	tests := []struct {
		name       string
		criteria   scraper.SearchCriteria
		apiKey     string
		wantErr    bool
	}{
		{
			name: "성공 - 정상적인 생성",
			criteria: scraper.SearchCriteria{
				SearchYear: "2024",
			},
			apiKey:  "dummy-key",
			wantErr: false,
		},
		{
			name: "실패 - 검색 연도 누락",
			criteria: scraper.SearchCriteria{
				SearchYear: "  ", // 공백만 있는 경우
			},
			apiKey:  "dummy-key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewEmart(tt.criteria, tt.apiKey)

			if (err != nil) != tt.wantErr {
				t.Fatalf("NewEmart() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if e == nil {
					t.Fatal("NewEmart()가 nil을 반환했습니다.")
				}
				if e.name != "이마트" {
					t.Errorf("NewEmart() 스크래퍼 이름 = %s, 기대값 = %s", e.name, "이마트")
				}
				if len(e.stores) == 0 {
					t.Error("NewEmart() 수집 대상 점포 목록이 비어 있습니다.")
				}
				if len(e.lectureGroups) == 0 {
					t.Error("NewEmart() 수집 대상 강좌군 목록이 비어 있습니다.")
				}
				if e.apiKey != tt.apiKey {
					t.Errorf("NewEmart() API Key = %s, 기대값 = %s", e.apiKey, tt.apiKey)
				}
			}
		})
	}
}

func TestEmart_Name(t *testing.T) {
	e, _ := NewEmart(scraper.SearchCriteria{SearchYear: "2024", SearchSeasonCode: "1"}, "dummy-key")
	if e.Name() != "이마트" {
		t.Errorf("Emart.Name() = %v, want %v", e.Name(), "이마트")
	}
}

func TestEmart_validateStore(t *testing.T) {
	tests := []struct {
		name       string
		storeCode  string
		storeName  string
		handler    http.HandlerFunc
		wantResult bool
		wantErr    bool
	}{
		{
			name:      "성공 - 점포 목록에 존재함",
			storeCode: "560",
			storeName: "여수",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				// GraphQL API 점포 목록 성공 응답 모킹
				resp := `{
					"data": {
						"getStoreAreaList": [
							{
								"PK": "AREA1",
								"area": "전라",
								"storeListInfo": [
									{"storeName": "여수", "storeCode": "560", "storeCenter": "01"},
									{"storeName": "순천", "storeCode": "900", "storeCenter": "02"}
								]
							}
						]
					}
				}`
				w.Write([]byte(resp))
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name:      "실패 - 점포 목록에 존재하지 않음 (코드 불일치)",
			storeCode: "999",
			storeName: "여수",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				// GraphQL API 점포 목록 성공 응답 모킹 (요청된 코드가 없음)
				resp := `{
					"data": {
						"getStoreAreaList": [
							{
								"PK": "AREA1",
								"area": "전라",
								"storeListInfo": [
									{"storeName": "여수", "storeCode": "560", "storeCenter": "01"}
								]
							}
						]
					}
				}`
				w.Write([]byte(resp))
			},
			wantResult: false,
			wantErr:    false,
		},
		{
			name:      "에러 - HTTP 상태 코드 오류",
			storeCode: "560",
			storeName: "여수",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantResult: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, e := setupEmartMockServer(tt.handler)
			defer ts.Close()

			got, err := e.validateStore(context.Background(), tt.storeCode, tt.storeName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Emart.validateStore() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantResult {
				t.Errorf("Emart.validateStore() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestEmart_validateLectureGroups(t *testing.T) {
	tests := []struct {
		name          string
		lectureGroups map[string]string
		handler       http.HandlerFunc
		wantResult    bool
		wantErr       bool
	}{
		{
			name: "성공 - 모든 강좌군(서브카테고리) 존재함",
			lectureGroups: map[string]string{
				"402": "With Mom",
				"404": "Kids & Children",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				// 카테고리 응답 모킹
				resp := `{
					"data": {
						"getCategoryList": {
							"message": [
								{
									"subCategory": [
										{"categoryCode": "402", "categoryName": "With Mom"},
										{"categoryCode": "404", "categoryName": "Kids & Children"}
									]
								}
							]
						}
					}
				}`
				w.Write([]byte(resp))
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name: "실패 - 일부 강좌군 명칭 불일치",
			lectureGroups: map[string]string{
				"402": "With Mom",
				"404": "키즈 앤 칠드런", // 매칭 실패
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				resp := `{
					"data": {
						"getCategoryList": {
							"message": [
								{
									"subCategory": [
										{"categoryCode": "402", "categoryName": "With Mom"},
										{"categoryCode": "404", "categoryName": "Kids & Children"}
									]
								}
							]
						}
					}
				}`
				w.Write([]byte(resp))
			},
			wantResult: false,
			wantErr:    false,
		},
		{
			name: "실패 - 일부 강좌군 누락",
			lectureGroups: map[string]string{
				"402": "With Mom",
				"999": "Unknown", // 매칭 실패
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				resp := `{
					"data": {
						"getCategoryList": {
							"message": [
								{
									"subCategory": [
										{"categoryCode": "402", "categoryName": "With Mom"}
									]
								}
							]
						}
					}
				}`
				w.Write([]byte(resp))
			},
			wantResult: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, e := setupEmartMockServer(tt.handler)
			defer ts.Close()

			e.lectureGroups = tt.lectureGroups

			got, err := e.validateLectureGroups(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Emart.validateLectureGroups() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantResult {
				t.Errorf("Emart.validateLectureGroups() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestEmart_Validate(t *testing.T) {
	// Validate() 내부적으로 호출되는 2가지 API(getCategoryList, getStoreAreaList)를 모두 적절히 핸들링해야 합니다.
	tests := []struct {
		name          string
		stores        map[string]string
		lectureGroups map[string]string
		handler       http.HandlerFunc
		wantErr       bool
	}{
		{
			name: "성공 - 점포와 강좌군 모두 유효함",
			stores: map[string]string{
				"560": "여수",
			},
			lectureGroups: map[string]string{
				"402": "With Mom",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				// 요청을 읽어서 어떤 쿼리인지 판단합니다.
				bodyBytes, _ := io.ReadAll(r.Body)
				reqBody := string(bodyBytes)

				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)

				if reqBody != "" && len(reqBody) > 0 {
					var payload emGraphQLRequest
					json.Unmarshal(bodyBytes, &payload)

					if len(payload.Query) > 0 && payload.Query[0:22] == "query getCategoryList " {
						// 카테고리 응답
						w.Write([]byte(`{ "data": { "getCategoryList": { "message": [ { "subCategory": [ {"categoryCode": "402", "categoryName": "With Mom"} ] } ] } } }`))
						return
					} else if len(payload.Query) > 0 && payload.Query[0:23] == "query getStoreAreaList(" {
						// 스토어 응답
						w.Write([]byte(`{ "data": { "getStoreAreaList": [ { "storeListInfo": [ {"storeName": "여수", "storeCode": "560", "storeCenter": "01"} ] } ] } }`))
						return
					}
				}
				// 기본 빈 응답
				w.Write([]byte(`{}`))
			},
			wantErr: false,
		},
		{
			name: "실패 - 점포 검증 실패 (강좌군은 성공 가정)",
			stores: map[string]string{
				"999": "없는점포",
			},
			lectureGroups: map[string]string{
				"402": "With Mom",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				bodyBytes, _ := io.ReadAll(r.Body)
				
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)

				var payload emGraphQLRequest
				json.Unmarshal(bodyBytes, &payload)

				if len(payload.Query) > 0 && payload.Query[0:22] == "query getCategoryList " {
					w.Write([]byte(`{ "data": { "getCategoryList": { "message": [ { "subCategory": [ {"categoryCode": "402", "categoryName": "With Mom"} ] } ] } } }`))
				} else {
					// 스토어 응답 (텅 빈 목록)
					w.Write([]byte(`{ "data": { "getStoreAreaList": [] } }`))
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, e := setupEmartMockServer(tt.handler)
			defer ts.Close()

			e.stores = tt.stores
			e.lectureGroups = tt.lectureGroups

			err := e.Validate(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Emart.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEmart_fetchLectureData(t *testing.T) {
	ts, e := setupEmartMockServer(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var payload emGraphQLRequest
		json.Unmarshal(bodyBytes, &payload)

		// 요청 파라미터 검증
		vars := payload.Variables.(map[string]interface{})
		from := int(vars["from"].(float64))
		size := int(vars["size"].(float64))

		if from != 20 {
			t.Errorf("예상된 from 파라미터: 20, 수신: %d", from)
		}
		if size != emSearchPageSize {
			t.Errorf("예상된 size 파라미터: %d, 수신: %d", emSearchPageSize, size)
		}
		
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"data": {
				"getClassByFiltering": {
					"total": 55,
					"data": [
						{
							"classTitle": "테스트 강좌",
							"classId": "123456"
						}
					]
				}
			}
		}`))
	})
	defer ts.Close()

	e.stores = map[string]string{"560": "여수"}
	e.lectureGroups = map[string]string{"402": "With Mom"}

	resp, err := e.fetchLectureData(context.Background(), "560", 20, emSearchPageSize)

	if err != nil {
		t.Fatalf("Emart.fetchLectureData() 예기치 않은 에러: %v", err)
	}

	if resp == nil {
		t.Fatal("응답 객체가 nil입니다.")
	}

	if resp.Data.GetClassByFiltering.Total != 55 {
		t.Errorf("Total= %d, expected 55", resp.Data.GetClassByFiltering.Total)
	}

	if len(resp.Data.GetClassByFiltering.Data) != 1 || resp.Data.GetClassByFiltering.Data[0].ClassTitle != "테스트 강좌" {
		t.Errorf("수신된 강좌 데이터가 기대와 일치하지 않습니다. title=%s", resp.Data.GetClassByFiltering.Data[0].ClassTitle)
	}
}

func TestEmart_Scrape(t *testing.T) {
	var requestCount int32

	ts, e := setupEmartMockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		bodyBytes, _ := io.ReadAll(r.Body)
		var payload emGraphQLRequest
		json.Unmarshal(bodyBytes, &payload)
		
		vars := payload.Variables.(map[string]interface{})
		from := int(vars["from"].(float64))

		atomic.AddInt32(&requestCount, 1)

		if from == 0 {
			// 첫번째(from 0) 응답에서는 total 25개로 응답하여 전체 갯수를 파악하게 함.
			// 그리고 0~19번째에 해당하는 강좌 1개를 반환.
			w.Write([]byte(`{
				"data": {
					"getClassByFiltering": {
						"total": 25,
						"data": [
							{
								"classTitle": "테스트 강좌 1",
								"classId": "TEST1",
								"classStatus": "접수중",
								"classDay": ["토"],
								"classTime": {"startTime": "1420", "endTime": "1520"},
								"classTimes": 12,
								"classFee": 60000,
								"classDateInfo": {"classStartDate": "20230820"}
							}
						]
					}
				}
			}`))
		} else {
			// 두번째(from 20) 요청에 대한 응답.
			w.Write([]byte(`{
				"data": {
					"getClassByFiltering": {
						"total": 25,
						"data": [
							{
								"classTitle": "테스트 강좌 2",
								"classId": "TEST2",
								"classStatus": "정원마감",
								"classDay": ["일"],
								"classTime": {"startTime": "1000", "endTime": "1100"},
								"classTimes": 4,
								"classFee": 20000,
								"classDateInfo": {"classStartDate": "20230901"}
							}
						]
					}
				}
			}`))
		}
	})
	defer ts.Close()

	// 테스트 대상 점포 축소
	e.stores = map[string]string{"560": "여수점"}
	e.lectureGroups = map[string]string{"402": "With Mom"}

	lectures, err := e.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Emart.Scrape() 예상치 못한 에러: %v", err)
	}

	// 총 3번의 요청이 발생해야 합니다:
	// 1: 전체 수를 파악하기 위한 from=0 (1회차)
	// 2: 전체 수에 기반한 for 루프의 첫번째, from=0 (이 로직은 전체 개수 파악 후 다시 from=0부터 요청함)
	// 3: for 루프의 두번째, from=20
	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("총 요청 수 기대값 = 3, 실제 = %d", atomic.LoadInt32(&requestCount))
	}

	if len(lectures) != 2 {
		t.Errorf("Emart.Scrape() 수집된 강좌 수 = %d, 기대값 = 2", len(lectures))
		return
	}

	// 고루틴 병렬 처리이므로 순서가 보장되지 않을 수 있습니다. 맵으로 확인합니다.
	titles := map[string]bool{}
	titles[lectures[0].Title] = true
	titles[lectures[1].Title] = true

	if !titles["테스트 강좌 1"] || !titles["테스트 강좌 2"] {
		t.Errorf("수집된 데이터에 기대한 강좌명이 없습니다. 수집된 강의: %v", lectures)
	}
}

func TestEmart_extractLecture(t *testing.T) {
	e, _ := NewEmart(scraper.SearchCriteria{SearchYear: "2024", SearchSeasonCode: "1"}, "dummy")
	storeName := "테스트 점포"

	tests := []struct {
		name        string
		lectureData emLectureAPIData
		wantTitle   string
		wantStatus  domain.ReceptionStatus
		wantErr     bool
	}{
		{
			name: "성공 - 완벽한 데이터 파싱 (접수 가능)",
			lectureData: emLectureAPIData{
				ClassTitle:  "어린이 수영",
				ClassID:     "12345",
				ClassStatus: "접수중",
				ClassDay:    []string{"토"},
				ClassTime: struct {
					StartTime string "json:\"startTime\""
					EndTime   string "json:\"endTime\""
				}{"1400", "1500"},
				ClassTimes: 4,
				ClassFee:   40000,
				ClassDateInfo: struct {
					ClassStartDate         string "json:\"classStartDate\""
					ClassEndDate           string "json:\"classEndDate\""
					ClassClosedDate        string "json:\"classClosedDate\""
					ClassRegisterStartDate string "json:\"classRegisterStartDate\""
					ClassRegisterEndDate   string "json:\"classRegisterEndDate\""
					ClassCancelStartDate   string "json:\"classCancelStartDate\""
					ClassCancelEndDate     string "json:\"classCancelEndDate\""
				}{ClassStartDate: "20230901"},
			},
			wantTitle:  "어린이 수영",
			wantStatus: domain.ReceptionStatusPossible,
			wantErr:    false,
		},
		{
			name: "성공 - 접수 대기 및 정원 마감 매핑",
			lectureData: emLectureAPIData{
				ClassTitle:  "오감놀이",
				ClassID:     "67890",
				ClassStatus: "접수대기",
				ClassDay:    []string{"월"},
				ClassTime: struct {
					StartTime string "json:\"startTime\""
					EndTime   string "json:\"endTime\""
				}{"1000", "1100"},
				ClassTimes: 1,
				ClassDateInfo: struct {
					ClassStartDate         string "json:\"classStartDate\""
					ClassEndDate           string "json:\"classEndDate\""
					ClassClosedDate        string "json:\"classClosedDate\""
					ClassRegisterStartDate string "json:\"classRegisterStartDate\""
					ClassRegisterEndDate   string "json:\"classRegisterEndDate\""
					ClassCancelStartDate   string "json:\"classCancelStartDate\""
					ClassCancelEndDate     string "json:\"classCancelEndDate\""
				}{ClassStartDate: "20231001"},
			},
			wantTitle:  "오감놀이",
			wantStatus: domain.ReceptionStatusStandBy,
			wantErr:    false,
		},
		{
			name: "실패 - 필수 식별자(강좌명) 누락",
			lectureData: emLectureAPIData{
				ClassTitle: "", // 비어있음
				ClassID:    "12345",
			},
			wantErr: true,
		},
		{
			name: "실패 - 개강일 날짜 형식 불일치 (8자리 아님)",
			lectureData: emLectureAPIData{
				ClassTitle: "에러강좌",
				ClassID:    "12345",
				ClassDateInfo: struct {
					ClassStartDate         string "json:\"classStartDate\""
					ClassEndDate           string "json:\"classEndDate\""
					ClassClosedDate        string "json:\"classClosedDate\""
					ClassRegisterStartDate string "json:\"classRegisterStartDate\""
					ClassRegisterEndDate   string "json:\"classRegisterEndDate\""
					ClassCancelStartDate   string "json:\"classCancelStartDate\""
					ClassCancelEndDate     string "json:\"classCancelEndDate\""
				}{ClassStartDate: "2023-09"}, // 8자리가 아님
			},
			wantErr: true,
		},
		{
			name: "실패 - 시작/종료 시간 데이터 규격 불일치",
			lectureData: emLectureAPIData{
				ClassTitle: "에러강좌",
				ClassID:    "12345",
				ClassTime: struct {
					StartTime string "json:\"startTime\""
					EndTime   string "json:\"endTime\""
				}{"140", "1500"}, // 4자리가 아님
				ClassDateInfo: struct {
					ClassStartDate         string "json:\"classStartDate\""
					ClassEndDate           string "json:\"classEndDate\""
					ClassClosedDate        string "json:\"classClosedDate\""
					ClassRegisterStartDate string "json:\"classRegisterStartDate\""
					ClassRegisterEndDate   string "json:\"classRegisterEndDate\""
					ClassCancelStartDate   string "json:\"classCancelStartDate\""
					ClassCancelEndDate     string "json:\"classCancelEndDate\""
				}{ClassStartDate: "20230901"},
			},
			wantErr: true,
		},
		{
			name: "실패 - 미지원 상태 라벨",
			lectureData: emLectureAPIData{
				ClassTitle:  "상태이상강좌",
				ClassID:     "12345",
				ClassStatus: "이상한상태",
				ClassDay:    []string{"월"},
				ClassTime: struct {
					StartTime string "json:\"startTime\""
					EndTime   string "json:\"endTime\""
				}{"1000", "1100"},
				ClassTimes: 1,
				ClassDateInfo: struct {
					ClassStartDate         string "json:\"classStartDate\""
					ClassEndDate           string "json:\"classEndDate\""
					ClassClosedDate        string "json:\"classClosedDate\""
					ClassRegisterStartDate string "json:\"classRegisterStartDate\""
					ClassRegisterEndDate   string "json:\"classRegisterEndDate\""
					ClassCancelStartDate   string "json:\"classCancelStartDate\""
					ClassCancelEndDate     string "json:\"classCancelEndDate\""
				}{ClassStartDate: "20231001"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lecture, err := e.extractLecture(context.Background(), tt.lectureData, storeName)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("Emart.extractLecture() error = %v, wantErr %v", err, tt.wantErr)
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
