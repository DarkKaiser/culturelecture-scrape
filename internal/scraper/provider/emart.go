package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
	"github.com/darkkaiser/notify-server/pkg/strutil"
)

// emSearchPageSize 이마트 문화센터 GraphQL API의 단일 호출당 요청할 강좌 데이터의 최대 건수입니다.
// 전체 강좌 수가 이 값을 초과하는 경우, Scrape()는 오프셋(from) 기반의 페이지네이션으로 병렬 분할 요청을 수행합니다.
const emSearchPageSize = 20

// @@@@@
// TODO: 이마트 수집 중 401 Unauthorized 에러가 발생하면 설정(Config) 파일이나 환경변수 수정을 통해 토큰을 교체해야 합니다.
const emAPIKey = "da2-ua6i7vyww5cmjkqzwv6gwdqhly"

// Emart 이마트 문화센터 강좌 정보를 수집하는 스크래퍼 구현체입니다.
type Emart struct {
	// name 스크래퍼가 수집 중인 대상이 어디인지 식별하기 위한 이름입니다. (예: "이마트")
	name string

	// cultureBaseURL 이마트 문화센터 웹사이트 기본 도메인 주소입니다.
	cultureBaseURL string

	// fetcher HTTP 요청을 수행하는 공유 클라이언트입니다.
	fetcher *scraper.Fetcher

	// authToken 이마트 문화센터 GraphQL API 인증에 사용되는 Bearer 토큰입니다.
	authToken string

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
func NewEmart(criteria scraper.SearchCriteria, authToken string) (*Emart, error) {
	// 검색년도는 API 요청에 필수적인 식별자이므로 누락을 허용하지 않습니다.
	searchYear := strutil.NormalizeSpace(criteria.SearchYear)
	if searchYear == "" {
		return nil, fmt.Errorf("유효하지 않은 검색 조건입니다: 이마트 API 요청에 필수적인 검색 연도가 누락되었습니다 (입력값 - 검색 연도: '%s')", searchYear)
	}

	return &Emart{
		name:           "이마트",
		cultureBaseURL: "https://www.cultureclub.emart.com",
		fetcher:        scraper.NewFetcher(),

		authToken: authToken,

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

// @@@@@
func (e *Emart) validateStore(ctx context.Context, storeCode, storeName string) (bool, error) {
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

	var storeListResp emStoreAPIResponse
	err := e.requestGraphQL(ctx, reqPayload, &storeListResp)
	if err != nil {
		return false, err
	}

	for _, storeArea := range storeListResp.Data.GetStoreAreaList {
		for _, store := range storeArea.StoreListInfo {
			if store.StoreCode == storeCode && store.StoreName == storeName {
				return true, nil
			}
		}
	}

	return false, nil
}

// @@@@@
func (e *Emart) validateLectureGroups(ctx context.Context) (bool, error) {
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

	var categoryListResp emCategoryAPIResponse
	err := e.requestGraphQL(ctx, reqPayload, &categoryListResp)
	if err != nil {
		return false, err
	}

	for lectureGroupCode, lectureGroupName := range e.lectureGroups {
		foundLectureGroup := false

		for _, categoryData := range categoryListResp.Data.GetCategoryList.Message {
			for _, subCategory := range categoryData.SubCategory {
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

// @@@@@
func (e *Emart) Scrape(ctx context.Context) ([]domain.Lecture, error) {

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(5) // HTTP 요청 부하 분산을 위한 동시성 제한

	var lectureList []domain.Lecture
	var mu sync.Mutex

	var count int64 = 0
	for storeCode, storeName := range e.stores {
		storeCode, storeName := storeCode, storeName

		// 점포 단위 스크래핑을 백그라운드 태스크로 분리하여 컨텍스트 및 에러 관리를 errgroup 내에 둔다.
		g.Go(func() error {
			// 불러올 전체 강좌 갯수를 구한다.
			lsrd, err := e.searchCultureLecture(groupCtx, storeCode, e.lectureGroups, 0, emSearchPageSize)
			if err != nil {
				return fmt.Errorf("%s 문화센터(%s) 전체 강좌 갯수 검색 실패: %w", e.name, storeName, err)
			}
			if lsrd.Data.GetClassByFiltering.Total == 0 {
				return fmt.Errorf("%s 문화센터(%s) 강좌를 수집하는 중에 전체 강좌 갯수 추출이 실패하였습니다.", e.name, storeName)
			}

			totalLectureCount := lsrd.Data.GetClassByFiltering.Total

			// 강좌 데이터를 수집한다. (동일 점포 내에서도 병렬로 요청)
			var innerG *errgroup.Group
			var innerCtx context.Context
			// groupCtx가 취소되면 innerCtx도 같이 취소됨
			innerG, innerCtx = errgroup.WithContext(groupCtx)
			innerG.SetLimit(10) // 하위 요청(강좌 목록 페이징)에 대한 동시성 제한

			for index := 0; index < totalLectureCount; {
				if err := innerCtx.Err(); err != nil {
					break
				}
				index0 := index
				innerG.Go(func() error {
					lsrd0, err := e.searchCultureLecture(innerCtx, storeCode, e.lectureGroups, index0, emSearchPageSize)
					if err != nil {
						return fmt.Errorf("%s 문화센터(%s) 강좌 페이지 검색 실패(index:%d): %w", e.name, storeName, index0, err)
					}

					var pageLectures []domain.Lecture
					for _, lsrld := range lsrd0.Data.GetClassByFiltering.Data {
						atomic.AddInt64(&count, 1)

						lecture, err := e.extractCultureLecture(innerCtx, storeName, lsrld)
						if err != nil {
							return fmt.Errorf("%s 문화센터 추출 오류: %w", e.name, err)
						}
						if lecture != nil {
							pageLectures = append(pageLectures, *lecture)
						}
					}

					if len(pageLectures) > 0 {
						mu.Lock()
						lectureList = append(lectureList, pageLectures...)
						mu.Unlock()
					}
					return nil
				})

				index += emSearchPageSize
			}

			if err := innerG.Wait(); err != nil {
				return err
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return lectureList, nil
}

// @@@@@
func (e *Emart) searchCultureLecture(ctx context.Context, storeCode string, lectureGroupCodeMap map[string]string, startIndex, size int) (*emLectureAPIResponse, error) {
	// 불러올 강좌군 코드 목록을 생성한다.
	var categoryCodes []string
	for code := range lectureGroupCodeMap {
		categoryCodes = append(categoryCodes, code)
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
				{"type": "mainStoreInfo.storeCode", "data": []string{storeCode}},
				{"type": "subCategory", "data": categoryCodes},
			},
			"sortKey": "deadline",
			"from":    startIndex,
			"size":    size,
		},
	}

	var lsrd emLectureAPIResponse
	err := e.requestGraphQL(ctx, reqPayload, &lsrd)
	if err != nil {
		return nil, err
	}

	return &lsrd, nil
}

// @@@@@
func (e *Emart) requestGraphQL(ctx context.Context, payload any, v any) error {
	clPageURL := "https://o27tfdumlrbf7jmrvql76qbhsm.appsync-api.ap-northeast-2.amazonaws.com/graphql"

	bodyBytes, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return fmt.Errorf("GraphQL 요청 JSON 직렬화 실패: %w", marshalErr)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", clPageURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("http.NewRequestWithContext failed: %w", err)
	}

	req.Header.Add("authorization", e.authToken)
	req.Header.Set("origin", e.cultureBaseURL)
	req.Header.Set("referer", e.cultureBaseURL)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("x-amz-user-agent", "aws-amplify/3.8.14 js")
	req.Header.Set("x-api-key", emAPIKey)

	if err := e.fetcher.FetchJSON(req, v); err != nil {
		return fmt.Errorf("이마트 API 요청 실패: %w", err)
	}

	return nil
}

// @@@@@
func (e *Emart) extractCultureLecture(ctx context.Context, storeName string, lsrld emLectureAPIData) (*domain.Lecture, error) {
	// 개강일
	startDate := lsrld.ClassDateInfo.ClassStartDate
	if len(startDate) != 8 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(개강일 파싱, URL:%s)", e.name, e.buildDetailPageURL(lsrld.ClassID))
	}
	startDate = fmt.Sprintf("%s-%s-%s", startDate[0:4], startDate[4:6], startDate[6:8])

	// 시작시간, 종료시간
	startTime := lsrld.ClassTime.StartTime
	endTime := lsrld.ClassTime.EndTime
	if len(startTime) != 4 || len(endTime) != 4 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(시간 파싱, URL:%s)", e.name, e.buildDetailPageURL(lsrld.ClassID))
	}
	startTime = fmt.Sprintf("%s:%s", startTime[:2], startTime[2:])
	endTime = fmt.Sprintf("%s:%s", endTime[:2], endTime[2:])

	// 요일
	if len(lsrld.ClassDay) == 0 {
		return nil, fmt.Errorf("%s 문화센터(%s) 강좌 데이터 파싱이 실패하였습니다(요일이 없음)", e.name, storeName)
	}
	dayOfTheWeek := lsrld.ClassDay[0]
	if len(dayOfTheWeek) == 0 {
		return nil, fmt.Errorf("%s 문화센터(%s) 강좌 데이터 파싱이 실패하였습니다(요일:%s)", e.name, storeName, dayOfTheWeek)
	}

	// 강좌횟수
	count := fmt.Sprintf("%d", lsrld.ClassTimes)
	if len(count) == 0 {
		return nil, fmt.Errorf("%s 문화센터(%s) 강좌 데이터 파싱이 실패하였습니다(강좌 횟수:%s)", e.name, storeName, count)
	}

	// 접수상태
	var status = domain.ReceptionStatusUnknown
	switch lsrld.ClassStatus {
	case "접수중":
		status = domain.ReceptionStatusPossible
	case "접수마감", "정원마감":
		status = domain.ReceptionStatusClosed
	case "접수대기":
		status = domain.ReceptionStatusStandBy
	default:
		return nil, fmt.Errorf("%s 문화센터(%s) 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(%s)", e.name, storeName, lsrld.ClassStatus)
	}

	if len(lsrld.ClassTitle) == 0 {
		return nil, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return &domain.Lecture{
			StoreName:      fmt.Sprintf("%s %s", e.name, storeName),
			Category:       "",
			Title:          lsrld.ClassTitle,
			Instructor:     "",
			StartDate:      startDate,
			StartTime:      startTime,
			EndTime:        endTime,
			Weekday:        dayOfTheWeek + "요일",
			Price:          fmt.Sprintf("%d", lsrld.ClassFee),
			SessionCount:   count,
			Status:         status,
			DetailPageURL:  e.buildDetailPageURL(lsrld.ClassID),
			ScrapeExcluded: false,
		}, nil
	}
}

// @@@@@
func (e *Emart) buildDetailPageURL(classID string) string {
	return fmt.Sprintf("%s/class/%s", e.cultureBaseURL, classID)
}
