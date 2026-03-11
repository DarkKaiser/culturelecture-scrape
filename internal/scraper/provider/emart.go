package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
	"github.com/darkkaiser/notify-server/pkg/strutil"
)

// emSearchPageSize 이마트 문화센터 GraphQL API의 단일 호출당 요청할 강좌 데이터의 최대 건수입니다.
// 전체 강좌 수가 이 값을 초과하는 경우, Scrape()는 오프셋(from) 기반의 페이지네이션으로 병렬 분할 요청을 수행합니다.
const emSearchPageSize = 20

// Emart 이마트 문화센터 강좌 정보를 수집하는 스크래퍼 구현체입니다.
type Emart struct {
	// name 스크래퍼가 수집 중인 대상이 어디인지 식별하기 위한 이름입니다. (예: "이마트")
	name string

	// cultureBaseURL 이마트 문화센터 웹사이트 기본 도메인 주소입니다.
	cultureBaseURL string

	// apiBaseURL 이마트 문화센터 GraphQL API 엔드포인트 도메인 주소입니다.
	apiBaseURL string

	// fetcher HTTP 요청을 수행하는 공유 클라이언트입니다.
	fetcher *scraper.Fetcher

	// apiKey 이마트 문화센터 GraphQL API 인증에 사용되는 x-api-key 토큰입니다.
	apiKey string

	// stores 수집 대상 점포 목록입니다. (점포코드 -> 점포명)
	stores map[string]string

	// lectureGroups 수집 대상 강좌군 목록입니다. (강좌군 카테고리코드 -> 강좌군명)
	lectureGroups map[string]string
}

// 컴파일 타임에 인터페이스 구현 여부를 검증합니다.
var _ scraper.Scraper = (*Emart)(nil)

// emGraphQLRequest 이마트 GraphQL API 요청 시 사용하는 공용 페이로드 구조체입니다.
type emGraphQLRequest struct {
	Query     string `json:"query"`
	Variables any    `json:"variables"`
}

// emLectureAPIResponse 이마트 강좌 검색 GraphQL API의 응답 데이터를 언마샬링하기 위한 구조체입니다.
type emLectureAPIResponse struct {
	Data struct {
		GetClassByFiltering struct {
			Total int                `json:"total"`
			Data  []emLectureAPIData `json:"data"`
		} `json:"getClassByFiltering"`
	} `json:"data"`
}

// emLectureAPIData 이마트 강좌 검색 결과 배열 내 개별 강좌의 상세 정보를 담고 있는 데이터 구조체입니다.
type emLectureAPIData struct {
	PK                 string   `json:"PK"`
	SK                 string   `json:"SK"`
	InstructorID       string   `json:"instructorId"`
	ClassID            string   `json:"classId"`
	InitialClassID     string   `json:"initialClassId"`
	ClassStatus        string   `json:"classStatus"`
	ClassStatusBO      string   `json:"classStatusBO"`
	ClassStatusTeacher string   `json:"classStatusTeacher"`
	ClassFlag          bool     `json:"classFlag"`
	ClassTitle         string   `json:"classTitle"`
	ClassDay           []string `json:"classDay"`
	ClassTime          struct {
		StartTime string `json:"startTime"`
		EndTime   string `json:"endTime"`
	} `json:"classTime"`
	MainCategory struct {
		MainCategoryOrder int    `json:"mainCategoryOrder"`
		SubCategoryOrder  int    `json:"subCategoryOrder"`
		CategoryCode      string `json:"categoryCode"`
		CategoryName      string `json:"categoryName"`
	} `json:"mainCategory"`
	SubCategory struct {
		MainCategoryOrder int    `json:"mainCategoryOrder"`
		SubCategoryOrder  int    `json:"subCategoryOrder"`
		CategoryCode      string `json:"categoryCode"`
		CategoryName      string `json:"categoryName"`
	} `json:"subCategory"`
	MainStoreInfo struct {
		StoreName   string `json:"storeName"`
		StoreCode   string `json:"storeCode"`
		StoreCenter string `json:"storeCenter"`
	} `json:"mainStoreInfo"`
	StoreInfo        []string `json:"storeInfo"`
	Classroom        string   `json:"classroom"`
	MinClassCapacity string   `json:"minClassCapacity"`
	ClassCapacity    int      `json:"classCapacity"`
	ClassTimes       int      `json:"classTimes"`
	SemesterYear     int      `json:"semesterYear"`
	Semester         string   `json:"semester"`
	ClassOriginalFee any      `json:"classOriginalFee"`
	ClassFee         int      `json:"classFee"`
	ClassMaterialFee string   `json:"classMaterialFee"`
	ClassType        any      `json:"classType"`
	Channel          struct {
		Online  string `json:"online"`
		Offline string `json:"offline"`
	} `json:"channel"`
	ClassDateInfo struct {
		ClassStartDate         string `json:"classStartDate"`
		ClassEndDate           string `json:"classEndDate"`
		ClassClosedDate        string `json:"classClosedDate"`
		ClassRegisterStartDate string `json:"classRegisterStartDate"`
		ClassRegisterEndDate   string `json:"classRegisterEndDate"`
		ClassCancelStartDate   string `json:"classCancelStartDate"`
		ClassCancelEndDate     string `json:"classCancelEndDate"`
	} `json:"classDateInfo"`
	ClassDetail struct {
		ClassDetailInfo struct {
			ClassDetailInfoTitle   string `json:"classDetailInfoTitle"`
			ClassDetailInfoContent string `json:"classDetailInfoContent"`
		} `json:"classDetailInfo"`
	} `json:"classDetail"`
	MainImage struct {
		Bucket any `json:"bucket"`
		Region any `json:"region"`
		Key    any `json:"key"`
	} `json:"mainImage"`
	CategoryImage struct {
		Bucket string `json:"bucket"`
		Region string `json:"region"`
		Key    string `json:"key"`
	} `json:"categoryImage"`
	MaterialCalculate struct {
		MaterialFee int `json:"materialFee"`
	} `json:"materialCalculate"`
}

// emStoreAPIResponse 이마트 점포 조회 API 응답을 파싱하여, 수집 대상 점포의 존재 여부를 사전 검증할 때 사용하는 구조체입니다.
type emStoreAPIResponse struct {
	Data struct {
		GetStoreAreaList []struct {
			PK            string `json:"PK"`
			Area          string `json:"area"`
			StoreListInfo []struct {
				StoreName   string `json:"storeName"`
				StoreCode   string `json:"storeCode"`
				StoreCenter string `json:"storeCenter"`
			} `json:"storeListInfo"`
		} `json:"getStoreAreaList"`
	} `json:"data"`
}

// emCategoryAPIResponse 이마트 카테고리(강좌군) API 응답을 파싱하여, 수집 대상 강좌군이 유효한지 사전 검증할 때 사용하는 구조체입니다.
type emCategoryAPIResponse struct {
	Data struct {
		GetCategoryList struct {
			Message []struct {
				MainCategory struct {
					PK                string `json:"PK"`
					SK                string `json:"SK"`
					MainCategoryOrder int    `json:"mainCategoryOrder"`
					SubCategoryOrder  int    `json:"subCategoryOrder"`
					CategoryCode      string `json:"categoryCode"`
					CategoryName      string `json:"categoryName"`
					UseFlag           string `json:"useFlag"`
					IconFileName      string `json:"iconFileName"`
				} `json:"mainCategory"`
				SubCategory []struct {
					PK                string `json:"PK"`
					SK                string `json:"SK"`
					MainCategoryOrder int    `json:"mainCategoryOrder"`
					SubCategoryOrder  int    `json:"subCategoryOrder"`
					CategoryCode      string `json:"categoryCode"`
					CategoryName      string `json:"categoryName"`
					UseFlag           string `json:"useFlag"`
					IconFileName      string `json:"iconFileName"`
					MainDisplayFlag   bool   `json:"mainDisplayFlag"`
					IconFilePath      struct {
						Bucket   string `json:"bucket"`
						Filename string `json:"filename"`
						Key      string `json:"key"`
						Region   string `json:"region"`
					} `json:"iconFilePath"`
				} `json:"subCategory"`
			} `json:"message"`
		} `json:"getCategoryList"`
	} `json:"data"`
}

// NewEmart 이마트 스크래퍼를 생성하여 반환합니다.
func NewEmart(criteria scraper.SearchCriteria, apiKey string) (*Emart, error) {
	// 검색년도는 API 요청에 필수적인 식별자이므로 누락을 허용하지 않습니다.
	searchYear := strutil.NormalizeSpace(criteria.SearchYear)
	if searchYear == "" {
		return nil, fmt.Errorf("유효하지 않은 검색 조건입니다: 이마트 API 요청에 필수적인 검색 연도가 누락되었습니다 (입력값 - 검색 연도: '%s')", searchYear)
	}

	return &Emart{
		name:           "이마트",
		cultureBaseURL: "https://www.cultureclub.emart.com",
		apiBaseURL:     "https://tjcdarnuonge5epm44y2nvckk4.appsync-api.ap-northeast-2.amazonaws.com/graphql",
		fetcher:        scraper.NewFetcher(),

		apiKey: apiKey,

		stores: map[string]string{
			"560": "여수",
			"900": "순천",
		},

		lectureGroups: map[string]string{
			"402": "With Mom",
			"403": "With mom(event)",
			"404": "Kids & Children",
			"406": "Kids & Children(event)",
		},
	}, nil
}

func (e *Emart) Name() string {
	return e.name
}

// Validate 스크래핑 작업을 시작하기 전, 설정값이 실제 이마트 시스템과 정합성이 맞는지 사전 검증합니다.
func (e *Emart) Validate(ctx context.Context) error {
	// GraphQL API(getCategoryList)를 호출하여, 수집 대상으로 설정된 강좌군 코드·명칭이 실제 시스템에 존재하는지 확인합니다.
	validLectureGroups, err := e.validateLectureGroups(ctx)
	if err != nil {
		return fmt.Errorf("%s 문화센터 강좌군 정보 검증 중 내/외부 시스템 오류가 발생했습니다: %w", e.name, err)
	}
	if !validLectureGroups {
		return fmt.Errorf("%s 문화센터에 설정된 강좌군 코드가 유효하지 않거나 실제 시스템 데이터와 일치하지 않습니다", e.name)
	}

	// GraphQL API(getStoreAreaList)를 호출하여, 수집 대상으로 설정된 각 점포가 실제 시스템에 존재하는지 확인합니다.
	for storeCode, storeName := range e.stores {
		validStore, err := e.validateStore(ctx, storeCode, storeName)
		if err != nil {
			return fmt.Errorf("%s 문화센터 점포 정보 검증 중 내/외부 시스템 오류가 발생했습니다. (대상 점포 코드: '%s'): %w", e.name, storeCode, err)
		}
		if !validStore {
			return fmt.Errorf("%s 문화센터에 설정된 점포 코드가 유효하지 않거나 실제 시스템 데이터와 일치하지 않습니다. (점포 코드: '%s')", e.name, storeCode)
		}
	}

	return nil
}

// validateStore 이마트 GraphQL API(getStoreAreaList)를 호출하여,
// 인자로 받은 점포 코드와 점포명이 실제 이마트 시스템에 존재하는 유효한 점포와 일치하는지 확인합니다.
func (e *Emart) validateStore(ctx context.Context, storeCode, storeName string) (bool, error) {
	// 이마트의 전국 점포 목록을 조회하기 위한 GraphQL 쿼리 요청을 구성합니다.
	// isAll: false 인 경우, 문화센터가 운영 중인 점포만 필터링하여 반환합니다.
	reqPayload := emGraphQLRequest{
		Query: `query getStoreAreaList($isAll: Boolean!) {
  getStoreAreaList(isAll: $isAll) {
    PK
    area
    storeListInfo {
      storeName
      storeCode
      storeCenter
    }
  }
}`,
		Variables: map[string]any{
			"isAll": false,
		},
	}

	// API 요청을 실행하고, 전국 점포 목록 응답을 파싱합니다.
	var storeListResp emStoreAPIResponse
	err := e.requestGraphQL(ctx, reqPayload, &storeListResp)
	if err != nil {
		return false, err
	}

	// 설정된 수집 대상 점포의 코드와 이름이 API 응답 목록에 존재하는지 교차 검증합니다.
	for _, storeArea := range storeListResp.Data.GetStoreAreaList {
		for _, store := range storeArea.StoreListInfo {
			if store.StoreCode == storeCode && store.StoreName == storeName {
				return true, nil
			}
		}
	}

	return false, nil
}

// validateLectureGroups 이마트 GraphQL API(getCategoryList)를 호출하여,
// 수집 대상으로 설정된 강좌군 코드·명칭이 실제 이마트 시스템의 서브 카테고리 목록에 빠짐없이 존재하는지 교차 검증합니다.
func (e *Emart) validateLectureGroups(ctx context.Context) (bool, error) {
	// 이마트의 전체 카테고리(강좌군) 목록을 조회하기 위한 GraphQL 쿼리 요청을 구성합니다.
	reqPayload := emGraphQLRequest{
		Query: `query getCategoryList {
  getCategoryList {
    message {
      mainCategory {
        PK
        SK
        mainCategoryOrder
        subCategoryOrder
        categoryCode
        categoryName
        useFlag
        iconFileName
      }
      subCategory {
        PK
        SK
        mainCategoryOrder
        subCategoryOrder
        categoryCode
        categoryName
        useFlag
        iconFileName
        mainDisplayFlag
        iconFilePath {
          bucket
          filename
          key
          region
        }
      }
    }
  }
}`,
		Variables: map[string]any{},
	}

	// API 요청을 실행하고, 카테고리(강좌군) 목록 응답을 파싱합니다.
	var categoryListResp emCategoryAPIResponse
	err := e.requestGraphQL(ctx, reqPayload, &categoryListResp)
	if err != nil {
		return false, err
	}

	// 설정된 각 강좌군을 순회하며, API 응답의 서브 카테고리 목록과 코드·명칭 양면으로 정합성을 교차 검증합니다.
	for lectureGroupCode, lectureGroupName := range e.lectureGroups {
		foundLectureGroup := false

		for _, categoryData := range categoryListResp.Data.GetCategoryList.Message {
			for _, subCategory := range categoryData.SubCategory {
				// 강좌군 코드(CategoryCode)와 명칭(CategoryName)이 모두 일치해야 유효한 강좌군으로 간주합니다.
				if subCategory.CategoryCode == lectureGroupCode && subCategory.CategoryName == lectureGroupName {
					foundLectureGroup = true
					break
				}
			}

			if foundLectureGroup {
				break
			}
		}

		if !foundLectureGroup {
			return false, nil
		}
	}

	return true, nil
}

// Scrape 설정된 모든 점포를 대상으로 이마트 문화센터 강좌 정보를 수집하여 반환합니다.
//
// 수집은 2단계 병렬 구조로 진행됩니다.
//  1. 점포(Store) 단위 병렬 처리: 여러 점포를 동시에 스크래핑합니다.
//  2. 오프셋(Offset) 단위 병렬 처리: 동일한 점포 내에서도 여러 데이터 구간을 동시에 가져옵니다.
//
// 각 계층의 동시성은 SetLimit으로 제한하여 대상 서버에 과도한 부하를 주지 않도록 합니다.
func (e *Emart) Scrape(ctx context.Context) ([]domain.Lecture, error) {
	// [1단계] 점포 단위 병렬 스크래핑 환경 구성
	// 대상 서버의 과부하 및 IP 차단을 방지하기 위해 최대 5개의 점포만 동시에 스크래핑합니다.
	storeGroup, storeCtx := errgroup.WithContext(ctx)
	storeGroup.SetLimit(5)

	// 수집된 전체 강좌 데이터를 안전하게 취합하기 위한 공용 슬라이스와 뮤텍스입니다.
	// 고루틴 간 락(Lock) 충돌로 인한 성능 저하를 막기 위해, 오프셋 구간 단위로 모아서 한 번에 추가합니다.
	var lectures []domain.Lecture
	var mu sync.Mutex

	for storeCode, storeName := range e.stores {
		// 루프 변수 클로저 캡처 방지 (Go 1.22 이전 버전 구문 호환 보장)
		storeCode, storeName := storeCode, storeName

		storeGroup.Go(func() error {
			// [사전 단계] 페이지네이션 메타데이터 확보
			// GraphQL API를 최초 1회 호출하여 해당 점포의 전체 강좌 수를 파악합니다.
			// 이마트는 페이지 번호가 아닌 오프셋(from) 기반의 API이므로, 총 강좌 수를 알아야 분할 요청 범위를 계산할 수 있습니다.
			totalResp, err := e.fetchLectureData(storeCtx, storeCode, 0, emSearchPageSize)
			if err != nil {
				return fmt.Errorf("%s 문화센터 전체 강좌 갯수 파악을 위한 초기 API 요청 중 오류가 발생하였습니다 (대상 점포: %s): %w", e.name, storeName, err)
			}
			if totalResp.Data.GetClassByFiltering.Total == 0 {
				return fmt.Errorf("%s 문화센터 강좌 수집 중 전체 강좌 갯수 추출에 실패하였습니다 (대상 점포: %s)", e.name, storeName)
			}

			totalLectureCount := totalResp.Data.GetClassByFiltering.Total

			// [2단계] 점포 내 오프셋 구간 단위 병렬 스크래핑 환경 구성
			// 단일 점포에 대한 과도한 동시 요청을 제한하기 위해 최대 10개의 구간만 동시에 수집합니다.
			offsetGroup, offsetCtx := errgroup.WithContext(storeCtx)
			offsetGroup.SetLimit(10)

			for startIndex := 0; startIndex < totalLectureCount; {
				// 취소된 컨텍스트에 대해 불필요한 고루틴 스케줄링이 발생하지 않도록 조기 차단합니다.
				if err := offsetCtx.Err(); err != nil {
					break
				}

				// 루프 변수 클로저 캡처 방지 (Go 1.22 이전 버전 구문 호환 보장)
				startIndex0 := startIndex

				offsetGroup.Go(func() error {
					// 오프셋(startIndex0)부터 최대 emSearchPageSize 건의 강좌 데이터를 조회합니다.
					offsetResp, err := e.fetchLectureData(offsetCtx, storeCode, startIndex0, emSearchPageSize)
					if err != nil {
						return fmt.Errorf("%s 문화센터 강좌 데이터 목록 요청 중 오류가 발생하였습니다 (대상 점포: %s, 오프셋 시작 위치: %d): %w", e.name, storeName, startIndex0, err)
					}

					var offsetLectures []domain.Lecture
					for _, lectureData := range offsetResp.Data.GetClassByFiltering.Data {
						lecture, err := e.extractLecture(offsetCtx, lectureData, storeName)
						if err != nil {
							return fmt.Errorf("%s 문화센터 개별 강좌 데이터 정보 파싱 중 오류가 발생했습니다 (점포명: '%s', 오프셋 시작 위치: %d): %w", e.name, storeName, startIndex0, err)
						}

						if lecture != nil {
							offsetLectures = append(offsetLectures, *lecture)
						}
					}

					// 현재 오프셋 구간에서 파싱한 강좌들을 전체 공유 목록에 안전하게 병합합니다.
					// 성능 병목을 방지하기 위해 강좌 낱개가 아닌 구간 단위로 모아서 한 번에 추가합니다.
					if len(offsetLectures) > 0 {
						mu.Lock()
						lectures = append(lectures, offsetLectures...)
						mu.Unlock()
					}

					return nil
				})

				startIndex += emSearchPageSize
			}

			// 현재 점포의 모든 오프셋 구간 스크래핑 작업이 완료될 때까지 대기합니다.
			if err := offsetGroup.Wait(); err != nil {
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

// fetchLectureData 이마트 GraphQL API(getClassByFiltering)를 호출하여 특정 점포의 강좌 목록 데이터를 가져옵니다.
//
// 반환값:
//   - *emLectureAPIResponse: 응답 데이터 (total: 전체 강좌 수, data: 강좌 상세 목록)
//   - error: HTTP 요청 생성 또는 데이터 수신 중 발생한 오류
func (e *Emart) fetchLectureData(ctx context.Context, storeCode string, startIndex, size int) (*emLectureAPIResponse, error) {
	// API 필터 조건에 넘겨줄 수집 대상 강좌군 코드 목록을 조립합니다.
	var categoryCodes []string
	for lectureGroupCode := range e.lectureGroups {
		categoryCodes = append(categoryCodes, lectureGroupCode)
	}

	reqPayload := emGraphQLRequest{
		Query: `query getClassByFiltering($keyword: String, $filterData: [FilterData], $sortKey: String, $from: Int, $size: Int) {
  getClassByFiltering(keyword: $keyword, filterData: $filterData, sortKey: $sortKey, from: $from, size: $size) {
    total
    data {
      PK
      SK
      instructorId
      classId
      initialClassId
      classStatus
      classStatusBO
      classStatusTeacher
      classFlag
      classTitle
      classDay
      classTime {
        startTime
        endTime
      }
      mainCategory {
        mainCategoryOrder
        subCategoryOrder
        categoryCode
        categoryName
      }
      subCategory {
        mainCategoryOrder
        subCategoryOrder
        categoryCode
        categoryName
      }
      mainStoreInfo {
        storeName
        storeCode
        storeCenter
      }
      storeInfo
      classroom
      minClassCapacity
      classCapacity
      classTimes
      semesterYear
      semester
      classOriginalFee
      classFee
      classMaterialFee
      classType
      channel {
        online
        offline
      }
      classDateInfo {
        classStartDate
        classEndDate
        classClosedDate
        classRegisterStartDate
        classRegisterEndDate
        classCancelStartDate
        classCancelEndDate
      }
      classDetail {
        classDetailInfo {
          classDetailInfoTitle
          classDetailInfoContent
        }
      }
      mainImage {
        bucket
        region
        key
      }
      categoryImage {
        bucket
        region
        key
      }
      materialCalculate {
        materialFee
      }
    }
  }
}`,
		Variables: map[string]any{
			"keyword": "",
			"filterData": []map[string]any{
				{"type": "mainStoreInfo.storeCode", "data": []string{storeCode}}, // 특정 점포만 대상으로 합니다.
				{"type": "subCategory", "data": categoryCodes},                   // 수집 대상 강좌군 코드 목록으로 필터링합니다.
			},
			"sortKey": "deadline", // 마감일 기준 오름차순 정렬
			"from":    startIndex, // 오프셋 기반 페이지네이션의 시작 인덱스
			"size":    size,       // 한 번의 요청으로 가져올 강좌 데이터의 최대 건수
		},
	}

	// GraphQL API 요청을 실행하고, 응답 JSON을 emLectureAPIResponse 구조체로 역직렬화합니다.
	var resp emLectureAPIResponse
	if err := e.requestGraphQL(ctx, reqPayload, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// requestGraphQL 이마트 AWS AppSync GraphQL 엔드포인트로 POST 요청을 전송하고, 응답 JSON을 v에 디코딩합니다.
// payload는 요청 본문으로 직렬화될 Go 객체(구조체 등)이며, v는 응답을 역직렬화할 목적지 포인터입니다.
func (e *Emart) requestGraphQL(ctx context.Context, payload any, v any) error {
	// 요청 payload(Go 구조체)를 JSON 형식의 바이트 슬라이스로 직렬화하여 HTTP 요청 본문을 구성합니다.
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("이마트 API에 보낼 요청 데이터를 JSON 형식으로 변환하는 데 실패하였습니다: %w", err)
	}

	// JSON 요청 본문을 담은 HTTP POST 요청 객체를 생성합니다.
	req, err := http.NewRequestWithContext(ctx, "POST", e.apiBaseURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("이마트 API 통신을 위한 HTTP 요청 객체 생성에 실패하였습니다: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("x-api-key", e.apiKey)                       // AWS AppSync가 요구하는 API 인증 토큰
	req.Header.Set("x-amz-user-agent", "aws-amplify/3.8.14 js") // AWS Amplify 클라이언트임을 식별하기 위한 커스텀 헤더
	req.Header.Set("Origin", e.cultureBaseURL)
	req.Header.Set("Referer", e.cultureBaseURL)

	// HTTP 요청을 실행하고, 응답받은 JSON을 v가 가리키는 구조체로 역직렬화합니다.
	if err := e.fetcher.FetchJSON(req, v); err != nil {
		return fmt.Errorf("이마트 API 서버와의 통신 및 데이터 수신 과정에서 오류가 발생하였습니다: %w", err)
	}

	return nil
}

// extractLecture API 응답의 개별 강좌 데이터(lectureData)를 파싱하여 domain.Lecture 구조체로 변환하여 반환합니다.
func (e *Emart) extractLecture(ctx context.Context, lectureData emLectureAPIData, storeName string) (*domain.Lecture, error) {
	// ------------------------------------------------------------------
	// 1단계: 핵심 텍스트 필드 검증 (Validation)
	// ------------------------------------------------------------------

	// 강좌명은 가장 기본적인 식별자이므로, 이 필드가 비어있다면 유의미한 강좌가 아닌 것으로 판단하여 즉시 건너뜁니다.
	if len(lectureData.ClassTitle) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 필수 식별자가 누락되었습니다 (원인: 강좌명 텍스트 부재, 대상 URL: %s)", e.name, e.buildDetailPageURL(lectureData.ClassID))
	}

	// ------------------------------------------------------------------
	// 2단계: API 원시 데이터 파싱 (Raw Data Parsing)
	// ------------------------------------------------------------------

	// 개강일: "20230820" (YYYYMMDD) 형식의 8자리 문자열을 도메인 표준 형식인 "YYYY-MM-DD"로 변환합니다.
	startDate := lectureData.ClassDateInfo.ClassStartDate
	if len(startDate) != 8 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 개강일 데이터 규격 불일치, 분석 데이터: '%s', 대상 URL: %s)", e.name, startDate, e.buildDetailPageURL(lectureData.ClassID))
	}
	startDate = fmt.Sprintf("%s-%s-%s", startDate[0:4], startDate[4:6], startDate[6:8])

	// 시작시간, 종료시간: "1420" (HHMM) 형식의 4자리 문자열을 도메인 표준 형식인 "HH:MM"으로 변환합니다.
	startTime := lectureData.ClassTime.StartTime
	endTime := lectureData.ClassTime.EndTime
	if len(startTime) != 4 || len(endTime) != 4 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 시작/종료 시간 데이터 규격 불일치, 시작: '%s', 종료: '%s', 대상 URL: %s)", e.name, startTime, endTime, e.buildDetailPageURL(lectureData.ClassID))
	}
	startTime = fmt.Sprintf("%s:%s", startTime[:2], startTime[2:])
	endTime = fmt.Sprintf("%s:%s", endTime[:2], endTime[2:])

	// 요일: ClassDay는 문자열 배열 형태이며, 첫 번째 요소(예: "토")를 수업 요일로 사용합니다.
	if len(lectureData.ClassDay) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 요일 배열 데이터 부재, 점포명: '%s', 대상 URL: %s)", e.name, storeName, e.buildDetailPageURL(lectureData.ClassID))
	}
	weekday := lectureData.ClassDay[0]
	if len(weekday) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 빈 요일 문자열, 점포명: '%s', 대상 URL: %s)", e.name, storeName, e.buildDetailPageURL(lectureData.ClassID))
	}

	// 강좌 횟수: 정수형(예: 12)을 문자열로 변환합니다.
	sessionCount := fmt.Sprintf("%d", lectureData.ClassTimes)
	if len(sessionCount) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 세부 필드 추출에 실패하였습니다 (원인: 강좌 횟수 데이터 누락, 점포명: '%s', 대상 URL: %s)", e.name, storeName, e.buildDetailPageURL(lectureData.ClassID))
	}

	// ------------------------------------------------------------------
	// 3단계: 접수 상태 판별 (Reception Status Detection)
	// ------------------------------------------------------------------
	// 이마트는 강좌 응답 데이터의 'classStatus' 문자열 값으로 접수 상태를 표현합니다.

	var receptionStatus = domain.ReceptionStatusUnknown
	switch lectureData.ClassStatus {
	case "접수중":
		receptionStatus = domain.ReceptionStatusPossible

	case "접수마감", "정원마감":
		receptionStatus = domain.ReceptionStatusClosed

	case "접수대기":
		receptionStatus = domain.ReceptionStatusStandBy

	default:
		return nil, fmt.Errorf("%s 문화센터 강좌 파싱 중 미지원 상태가 감지되었습니다 (원인: 해석 불가한 접수 상태 라벨, 식별된 라벨: '%s', 점포명: '%s', 대상 URL: %s)", e.name, lectureData.ClassStatus, storeName, e.buildDetailPageURL(lectureData.ClassID))
	}

	// ------------------------------------------------------------------
	// 4단계: 컨텍스트 취소 여부 최종 확인
	// ------------------------------------------------------------------
	// 모든 데이터 파싱이 완료된 시점에서 확인하여, 취소된 컨텍스트에 대해 domain.Lecture 객체 생성을 생략합니다.

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// ------------------------------------------------------------------
	// 5단계: 최종 도메인 모델 생성 및 반환
	// ------------------------------------------------------------------

	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	default:
		return &domain.Lecture{
			StoreName:      fmt.Sprintf("%s %s", e.name, storeName),
			Category:       "", // 이마트는 강좌 목록 뷰에 카테고리 텍스트를 별도로 제공하지 않음
			Title:          lectureData.ClassTitle,
			Instructor:     "", // 이마트는 강좌 목록 뷰에 강사명 텍스트를 별도로 제공하지 않음
			StartDate:      startDate,
			StartTime:      startTime,
			EndTime:        endTime,
			Weekday:        fmt.Sprintf("%s요일", weekday),
			Price:          fmt.Sprintf("%d", lectureData.ClassFee),
			SessionCount:   sessionCount,
			Status:         receptionStatus,
			DetailPageURL:  e.buildDetailPageURL(lectureData.ClassID),
			ScrapeExcluded: false,
		}, nil
	}
}

// buildDetailPageURL 강좌 고유 식별자를 받아 해당 강좌의 상세 페이지 URL을 조립하여 반환합니다.
func (e *Emart) buildDetailPageURL(lectureID string) string {
	return fmt.Sprintf("%s/class/%s", e.cultureBaseURL, lectureID)
}
