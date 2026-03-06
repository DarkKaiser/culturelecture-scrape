package provider

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
	"github.com/darkkaiser/notify-server/pkg/strutil"
)

type Emart struct {
	name           string
	cultureBaseUrl string

	searchYearCode string // 검색년도
	searchSmstCode string // 검색시즌 코드(미사용)
	authToken      string // 외부 주입 이마트 API 토큰
	client         *scraper.Fetcher

	storeCodeMap        map[string]string // 점포
	lectureGroupCodeMap map[string]string // 강좌군
}

// 컴파일 타임에 인터페이스 구현 여부를 검증합니다.
var _ scraper.Scraper = (*Emart)(nil)

// TODO: 이마트 수집 중 401 Unauthorized 에러가 발생하면 설정(Config) 파일이나 환경변수 수정을 통해 토큰을 교체해야 합니다.
const emartApiKey = "da2-ua6i7vyww5cmjkqzwv6gwdqhly"

type emartLectureSearchResultData struct {
	Data struct {
		GetClassByFiltering struct {
			Total int                                   `json:"total"`
			Data  []emartLectureSearchResultLectureData `json:"data"`
		} `json:"getClassByFiltering"`
	} `json:"data"`
}

type emartLectureSearchResultLectureData struct {
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
	StoreInfo        []string    `json:"storeInfo"`
	Classroom        string      `json:"classroom"`
	MinClassCapacity string      `json:"minClassCapacity"`
	ClassCapacity    int         `json:"classCapacity"`
	ClassTimes       int         `json:"classTimes"`
	SemesterYear     int         `json:"semesterYear"`
	Semester         string      `json:"semester"`
	ClassOriginalFee interface{} `json:"classOriginalFee"`
	ClassFee         int         `json:"classFee"`
	ClassMaterialFee string      `json:"classMaterialFee"`
	ClassType        interface{} `json:"classType"`
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
		Bucket interface{} `json:"bucket"`
		Region interface{} `json:"region"`
		Key    interface{} `json:"key"`
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

type emartStoreSearchResultData struct {
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

type emartLectureGroupSearchResultData struct {
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

func NewEmart(criteria scraper.SearchCriteria, authToken string) (*Emart, error) {
	searchYear := strutil.NormalizeSpace(criteria.SearchYear)

	if searchYear == "" {
		return nil, fmt.Errorf("검색년도는 빈 문자열을 허용하지 않습니다(검색년도:%s)", searchYear)
	}

	return &Emart{
		name: "이마트",

		cultureBaseUrl: "https://www.cultureclub.emart.com",

		searchYearCode: searchYear,
		searchSmstCode: "",
		authToken:      authToken,
		client:         scraper.NewFetcher(),

		storeCodeMap: map[string]string{
			"560": "여수",
			"900": "순천",
		},

		lectureGroupCodeMap: map[string]string{
			"402": "With Mom",
			"403": "With mom(event)",
			"404": "Kids & Children",
			"406": "Kids & Children(event)",
		},
	}, nil
}

func (e *Emart) Validate(ctx context.Context) error {
	// 강좌군이 유효한지 확인한다.
	validLG, err := e.validCultureLectureGroup(ctx)
	if err != nil {
		return fmt.Errorf("%s 문화센터 강좌군 검증 중 오류 발생: %v", e.name, err)
	}
	if !validLG {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌군코드 불일치)", e.name)
	}

	// 모든 점포가 유효한지 확인한다.
	for storeCode, storeName := range e.storeCodeMap {
		validStore, err := e.validCultureLectureStore(ctx, storeCode, storeName)
		if err != nil {
			return fmt.Errorf("%s 문화센터 점포 검증 중 오류 발생(점포코드:%s): %w", e.name, storeCode, err)
		}
		if !validStore {
			return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(점포코드 불일치:%s)", e.name, storeCode)
		}
	}

	return nil
}

func (e *Emart) Scrape(ctx context.Context) ([]domain.Lecture, error) {

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(5) // HTTP 요청 부하 분산을 위한 동시성 제한

	var lectureList []domain.Lecture
	var mu sync.Mutex

	// 한번에 검색할 강좌 갯수
	const sizeOfLectureToSearch = 20

	var count int64 = 0
	for storeCode, storeName := range e.storeCodeMap {
		storeCode, storeName := storeCode, storeName

		// 점포 단위 스크래핑을 백그라운드 태스크로 분리하여 컨텍스트 및 에러 관리를 errgroup 내에 둔다.
		g.Go(func() error {
			// 불러올 전체 강좌 갯수를 구한다.
			lsrd, err := e.searchCultureLecture(groupCtx, storeCode, e.lectureGroupCodeMap, 0, sizeOfLectureToSearch)
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
					lsrd0, err := e.searchCultureLecture(innerCtx, storeCode, e.lectureGroupCodeMap, index0, sizeOfLectureToSearch)
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

				index += sizeOfLectureToSearch
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

func (e *Emart) searchCultureLecture(ctx context.Context, storeCode string, lectureGroupCodeMap map[string]string, startIndex, size int) (*emartLectureSearchResultData, error) {
	// 불러올 강좌군 코드 목록을 생성한다.
	lectureGroupCodeString := ""
	for code := range lectureGroupCodeMap {
		if lectureGroupCodeString != "" {
			lectureGroupCodeString += ", "
		}
		lectureGroupCodeString += fmt.Sprintf("\"%s\"", code)
	}

	var lsrd emartLectureSearchResultData
	err := e.requestSite(ctx, fmt.Sprintf("{\"query\":\"query getClassByFiltering($keyword: String, $filterData: [FilterData], $sortKey: String, $from: Int, $size: Int) {\\n  getClassByFiltering(keyword: $keyword, filterData: $filterData, sortKey: $sortKey, from: $from, size: $size) {\\n    total\\n    data {\\n      PK\\n      SK\\n      instructorId\\n      classId\\n      initialClassId\\n      classStatus\\n      classStatusBO\\n      classStatusTeacher\\n      classFlag\\n      classTitle\\n      classDay\\n      classTime {\\n        startTime\\n        endTime\\n      }\\n      mainCategory {\\n        mainCategoryOrder\\n        subCategoryOrder\\n        categoryCode\\n        categoryName\\n      }\\n      subCategory {\\n        mainCategoryOrder\\n        subCategoryOrder\\n        categoryCode\\n        categoryName\\n      }\\n      mainStoreInfo {\\n        storeName\\n        storeCode\\n        storeCenter\\n      }\\n      storeInfo\\n      classroom\\n      minClassCapacity\\n      classCapacity\\n      classTimes\\n      semesterYear\\n      semester\\n      classOriginalFee\\n      classFee\\n      classMaterialFee\\n      classType\\n      channel {\\n        online\\n        offline\\n      }\\n      classDateInfo {\\n        classStartDate\\n        classEndDate\\n        classClosedDate\\n        classRegisterStartDate\\n        classRegisterEndDate\\n        classCancelStartDate\\n        classCancelEndDate\\n      }\\n      classDetail {\\n        classDetailInfo {\\n          classDetailInfoTitle\\n          classDetailInfoContent\\n        }\\n      }\\n      mainImage {\\n        bucket\\n        region\\n        key\\n      }\\n      categoryImage {\\n        bucket\\n        region\\n        key\\n      }\\n      materialCalculate {\\n        materialFee\\n      }\\n    }\\n  }\\n}\\n\",\"variables\":{\"keyword\":\"\",\"filterData\":[{\"type\":\"mainStoreInfo.storeCode\",\"data\":[\"%s\"]},{\"type\":\"subCategory\",\"data\":[%s]}],\"sortKey\":\"deadline\",\"from\":%d,\"size\":%d}}", storeCode, lectureGroupCodeString, startIndex, size), &lsrd)
	if err != nil {
		return nil, err
	}

	return &lsrd, nil
}

func (e *Emart) getDetailPageURL(classID string) string {
	return fmt.Sprintf("%s/class/%s", e.cultureBaseUrl, classID)
}

func (e *Emart) extractCultureLecture(ctx context.Context, storeName string, lsrld emartLectureSearchResultLectureData) (*domain.Lecture, error) {
	// 개강일
	startDate := lsrld.ClassDateInfo.ClassStartDate
	if len(startDate) != 8 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(개강일 파싱, URL:%s)", e.name, e.getDetailPageURL(lsrld.ClassID))
	}
	startDate = fmt.Sprintf("%s-%s-%s", startDate[0:4], startDate[4:6], startDate[6:8])

	// 시작시간, 종료시간
	startTime := lsrld.ClassTime.StartTime
	endTime := lsrld.ClassTime.EndTime
	if len(startTime) != 4 || len(endTime) != 4 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(시간 파싱, URL:%s)", e.name, e.getDetailPageURL(lsrld.ClassID))
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
			DetailPageURL:  e.getDetailPageURL(lsrld.ClassID),
			ScrapeExcluded: false,
		}, nil
	}
}

func (e *Emart) validCultureLectureStore(ctx context.Context, storeCode, storeName string) (bool, error) {
	var ssrd emartStoreSearchResultData
	err := e.requestSite(ctx, "{\"query\":\"query getStoreAreaList($isAll: Boolean!) {\\n  getStoreAreaList(isAll: $isAll) {\\n    PK\\n    area\\n    storeListInfo {\\n      storeName\\n      storeCode\\n      storeCenter\\n    }\\n  }\\n}\\n\",\"variables\":{\"isAll\":false}}", &ssrd)
	if err != nil {
		return false, err
	}

	for _, storeArea := range ssrd.Data.GetStoreAreaList {
		for _, store := range storeArea.StoreListInfo {
			if store.StoreCode == storeCode && store.StoreName == storeName {
				return true, nil
			}
		}
	}

	return false, nil
}

func (e *Emart) validCultureLectureGroup(ctx context.Context) (bool, error) {
	var lgsrd emartLectureGroupSearchResultData
	err := e.requestSite(ctx, "{\"query\":\"query getCategoryList {\\n  getCategoryList {\\n    message {\\n      mainCategory {\\n        PK\\n        SK\\n        mainCategoryOrder\\n        subCategoryOrder\\n        categoryCode\\n        categoryName\\n        useFlag\\n        iconFileName\\n      }\\n      subCategory {\\n        PK\\n        SK\\n        mainCategoryOrder\\n        subCategoryOrder\\n        categoryCode\\n        categoryName\\n        useFlag\\n        iconFileName\\n        mainDisplayFlag\\n        iconFilePath {\\n          bucket\\n          filename\\n          key\\n          region\\n        }\\n      }\\n    }\\n  }\\n}\\n\",\"variables\":{}}", &lgsrd)
	if err != nil {
		return false, err
	}

	for lgCode, lgName := range e.lectureGroupCodeMap {
		exist := false
		for _, m := range lgsrd.Data.GetCategoryList.Message {
			for _, sc := range m.SubCategory {
				if sc.CategoryCode == lgCode && sc.CategoryName == lgName {
					exist = true
					break
				}
			}
			if exist == true {
				break
			}
		}

		if exist == false {
			return false, nil
		}
	}

	return true, nil
}

func (e *Emart) requestSite(ctx context.Context, body string, v interface{}) error {
	clPageUrl := "https://o27tfdumlrbf7jmrvql76qbhsm.appsync-api.ap-northeast-2.amazonaws.com/graphql"

	req, err := http.NewRequestWithContext(ctx, "POST", clPageUrl, bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("http.NewRequestWithContext failed: %w", err)
	}

	req.Header.Add("authorization", e.authToken)
	req.Header.Set("origin", e.cultureBaseUrl)
	req.Header.Set("referer", e.cultureBaseUrl)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("x-amz-user-agent", "aws-amplify/3.8.14 js")
	req.Header.Set("x-api-key", emartApiKey)

	if err := e.client.FetchJSON(req, v); err != nil {
		return fmt.Errorf("이마트 API 요청 실패: %w", err)
	}

	return nil
}
