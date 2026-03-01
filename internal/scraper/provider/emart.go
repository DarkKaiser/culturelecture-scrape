package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/notify-server/pkg/strutil"
)

type Emart struct {
	name           string
	cultureBaseUrl string

	searchYearCode string // 검색년도
	searchSmstCode string // 검색시즌 코드(미사용)

	storeCodeMap        map[string]string // 점포
	lectureGroupCodeMap map[string]string // 강좌군
}

// TODO: 이마트 수집 중 401 Unauthorized 에러가 발생하면, 브라우저 관리자 도구(F12)의 네트워크 탭에서 최근 Authorization 토큰을 복사하여 아래 상수를 교체해 주세요.
const emartAuthToken = "eyJraWQiOiJMdmZXelNObFM0WEFTU2RJcytiYXJlNHl6VWNyVmNWRExqcHQyanBDNlE0PSIsImFsZyI6IlJTMjU2In0.eyJzdWIiOiJmODU2YjQxNy0wMjQ4LTQ3ZmQtYTM5Ni01OGE2NDczODA3YjUiLCJiaXJ0aGRhdGUiOiIxOTc4LTA2LTE2IiwiY3VzdG9tOm1icktleSI6ImV5SmhiR2NpT2lKSVV6STFOaUlzSW5SNWNDSTZJa3BYVkNKOS5leUpqZEcwaU9pSkRNREF3TURBd05DSXNJbk5wWkNJNkltVnRZWEowWTNWc2RDSXNJbUYxWkNJNklrRlFVQ0lzSW5WcFpDSTZJbHd2UldaMk1VMDJSM1JJYUhOU2NYSXdaMVZTZG14blBUMGlMQ0psZUhBaU9qRTJOVEk0TURJNE1EWXNJbWx6Y3lJNklrTnNkV1JOWlcxaVpYSnphR2x3SWl3aWFtRjBJam94TmpVeU56VTVOakEyTENKcWRHa2lPaUppTTJJd05UUmhaUzA0TTJVeUxUUTVaR1V0T1RnME1DMDROV1UyWWpJM05qazJaallpZlEua01kNk5HX0RhX0RvcHBUeldVMmpFSjJWLWpQRUhDUktCclhNVTRQMk42YyIsImlzcyI6Imh0dHBzOlwvXC9jb2duaXRvLWlkcC5hcC1ub3J0aGVhc3QtMi5hbWF6b25hd3MuY29tXC9hcC1ub3J0aGVhc3QtMl9FMXRsWmcxY0UiLCJjb2duaXRvOnVzZXJuYW1lIjoiQzcxNTQ3MDI1IiwiY3VzdG9tOmVjY2lkIjoiMTk2NTc0MTkiLCJvcmlnaW5fanRpIjoiNzAyMTU1NWMtYTNhZS00YmI4LWFiNWEtYjFjMjhmMGUzY2Y5IiwiYXVkIjoiMWIwbTc2bXF1amtxczBtZDRsbGllaTQwMzIiLCJldmVudF9pZCI6ImYxNjc5OTZjLWMzNzgtNGJkMi04MDJjLTViNGNjYmMyMjkwNyIsInRva2VuX3VzZSI6ImlkIiwiYXV0aF90aW1lIjoxNjUyNzU5NjA3LCJuYW1lIjoi7Y647KeE7Zy0IiwiZXhwIjoxNjUyNzYzMjA3LCJpYXQiOjE2NTI3NTk2MDcsImp0aSI6ImI3YWYyMzk4LTUyYmItNDcxZi1hZDE4LTk5NWNiZjEzYWFiYyJ9.W7kO5Nui-bgUEQfbkbMgSYlwS-S4oyFs67CWKJlpkcDDP2JaLGN-kcPTOMT5J1Y8dHPNPc6LVXvj7XO2FdGUBNACl1NoTzkhV8d-UJUqDbWWAWRLwc0-v2ZFsX9NAMuM1oy4CrDnWzo02IEgfaj-r80ClaqZcoT969IJ5UMan7F_WtBTN1Ps6jYdI3n8arlRKSXugjJttbgGzUIjBJDFRyEqooUfeQVLFl0sY-70Jw2C_Xr4ywQYxTYymBb_H3q8CjCmU_jX1vQfFeSZwJ7wriGgonhzj0AOiQoyDrXsk88G9WT2PpbcjpoXq1wnJvibfev7N3AQlAkbdsZ6osNOsg"
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

func NewEmart(searchYear string) *Emart {
	searchYear = strutil.NormalizeSpace(searchYear)

	if searchYear == "" {
		log.Fatalf("검색년도는 빈 문자열을 허용하지 않습니다(검색년도:%s)", searchYear)
	}

	return &Emart{
		name: "이마트",

		cultureBaseUrl: "https://www.cultureclub.emart.com",

		searchYearCode: searchYear,

		searchSmstCode: "",

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
	}
}

func (e *Emart) ScrapeCultureLectures(mainC chan<- []domain.Lecture) error {
	log.Printf("%s 문화센터 강좌 수집을 시작합니다.", e.name)

	// 강좌군이 유효한지 확인한다.
	validLG, err := e.validCultureLectureGroup()
	if err != nil {
		return fmt.Errorf("%s 문화센터 강좌군 검증 중 오류 발생: %v", e.name, err)
	}
	if !validLG {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, 강좌군코드 불일치)", e.name)
	}

	var wait sync.WaitGroup

	c := make(chan *domain.Lecture, 100)

	// 한번에 검색할 강좌 갯수
	const sizeOfLectureToSearch = 20

	var count int64 = 0
	for storeCode, storeName := range e.storeCodeMap {
		// 점포가 유효한지 확인한다.
		validStore, err := e.validCultureLectureStore(storeCode, storeName)
		if err != nil {
			return fmt.Errorf("%s 문화센터 점포 검증 중 오류 발생(점포코드:%s): %v", e.name, storeCode, err)
		}
		if !validStore {
			return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(CSS셀렉터를 확인하세요, 점포코드 불일치:%s)", e.name, storeCode)
		}

		// 불러올 전체 강좌 갯수를 구한다.
		lsrd, err := e.searchCultureLecture(storeCode, e.lectureGroupCodeMap, 0, sizeOfLectureToSearch)
		if err != nil {
			return fmt.Errorf("%s 문화센터(%s) 전체 강좌 갯수 검색 실패: %v", e.name, storeName, err)
		}
		if lsrd.Data.GetClassByFiltering.Total == 0 {
			return fmt.Errorf("%s 문화센터(%s) 강좌를 수집하는 중에 전체 강좌 갯수 추출이 실패하였습니다.", e.name, storeName)
		}

		totalLectureCount := lsrd.Data.GetClassByFiltering.Total

		// 강좌 데이터를 수집한다.
		for index := 0; index < totalLectureCount; {
			wait.Add(1)
			go func(storeCode0, storeName0 string, index0 int) {
				defer wait.Done()

				lsrd0, err := e.searchCultureLecture(storeCode0, e.lectureGroupCodeMap, index0, sizeOfLectureToSearch)
				if err != nil {
					log.Fatalf("%s 문화센터(%s) 강좌 페이지 검색 실패(index:%d)로 종료합니다: %v", e.name, storeName0, index0, err)
				}

				for _, lsrld := range lsrd0.Data.GetClassByFiltering.Data {
					atomic.AddInt64(&count, 1)
					go func(l emartLectureSearchResultLectureData) {
						err := e.extractCultureLecture(storeName0, l, c)
						if err != nil {
							log.Fatalf("%s 문화센터 추출 오류로 종료합니다: %v", e.name, err)
						}
					}(lsrld)
				}
			}(storeCode, storeName, index)

			index += sizeOfLectureToSearch
		}
	}

	wait.Wait()

	var lectureList []domain.Lecture
	for i := int64(0); i < count; i++ {
		lecture := <-c
		if len(lecture.Title) > 0 {
			lectureList = append(lectureList, *lecture)
		}
	}

	log.Printf("%s 문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", e.name, len(lectureList))

	mainC <- lectureList

	return nil
}

func (e *Emart) searchCultureLecture(storeCode string, lectureGroupCodeMap map[string]string, startIndex, size int) (*emartLectureSearchResultData, error) {
	// 불러올 강좌군 코드 목록을 생성한다.
	lectureGroupCodeString := ""
	for code := range lectureGroupCodeMap {
		if lectureGroupCodeString != "" {
			lectureGroupCodeString += ", "
		}
		lectureGroupCodeString += fmt.Sprintf("\"%s\"", code)
	}

	var lsrd emartLectureSearchResultData
	err := e.requestSite(fmt.Sprintf("{\"query\":\"query getClassByFiltering($keyword: String, $filterData: [FilterData], $sortKey: String, $from: Int, $size: Int) {\\n  getClassByFiltering(keyword: $keyword, filterData: $filterData, sortKey: $sortKey, from: $from, size: $size) {\\n    total\\n    data {\\n      PK\\n      SK\\n      instructorId\\n      classId\\n      initialClassId\\n      classStatus\\n      classStatusBO\\n      classStatusTeacher\\n      classFlag\\n      classTitle\\n      classDay\\n      classTime {\\n        startTime\\n        endTime\\n      }\\n      mainCategory {\\n        mainCategoryOrder\\n        subCategoryOrder\\n        categoryCode\\n        categoryName\\n      }\\n      subCategory {\\n        mainCategoryOrder\\n        subCategoryOrder\\n        categoryCode\\n        categoryName\\n      }\\n      mainStoreInfo {\\n        storeName\\n        storeCode\\n        storeCenter\\n      }\\n      storeInfo\\n      classroom\\n      minClassCapacity\\n      classCapacity\\n      classTimes\\n      semesterYear\\n      semester\\n      classOriginalFee\\n      classFee\\n      classMaterialFee\\n      classType\\n      channel {\\n        online\\n        offline\\n      }\\n      classDateInfo {\\n        classStartDate\\n        classEndDate\\n        classClosedDate\\n        classRegisterStartDate\\n        classRegisterEndDate\\n        classCancelStartDate\\n        classCancelEndDate\\n      }\\n      classDetail {\\n        classDetailInfo {\\n          classDetailInfoTitle\\n          classDetailInfoContent\\n        }\\n      }\\n      mainImage {\\n        bucket\\n        region\\n        key\\n      }\\n      categoryImage {\\n        bucket\\n        region\\n        key\\n      }\\n      materialCalculate {\\n        materialFee\\n      }\\n    }\\n  }\\n}\\n\",\"variables\":{\"keyword\":\"\",\"filterData\":[{\"type\":\"mainStoreInfo.storeCode\",\"data\":[\"%s\"]},{\"type\":\"subCategory\",\"data\":[%s]}],\"sortKey\":\"deadline\",\"from\":%d,\"size\":%d}}", storeCode, lectureGroupCodeString, startIndex, size), &lsrd)
	if err != nil {
		return nil, err
	}

	return &lsrd, nil
}

func (e *Emart) extractCultureLecture(storeName string, lsrld emartLectureSearchResultLectureData, c chan<- *domain.Lecture) error {
	// 개강일
	startDate := lsrld.ClassDateInfo.ClassStartDate
	if len(startDate) != 8 {
		return fmt.Errorf("%s 문화센터(%s) 강좌 데이터 파싱이 실패하였습니다(개강일:%s)", e.name, storeName, startDate)
	}
	startDate = fmt.Sprintf("%s-%s-%s", startDate[:4], startDate[4:6], startDate[6:])

	// 시작시간, 종료시간
	startTime := lsrld.ClassTime.StartTime
	endTime := lsrld.ClassTime.EndTime
	if len(startTime) != 4 || len(endTime) != 4 {
		return fmt.Errorf("%s 문화센터(%s) 강좌 데이터 파싱이 실패하였습니다(시작시간:%s, 종료시간:%s)", e.name, storeName, startTime, endTime)
	}
	startTime = fmt.Sprintf("%s:%s", startTime[:2], startTime[2:])
	endTime = fmt.Sprintf("%s:%s", endTime[:2], endTime[2:])

	// 요일
	if len(lsrld.ClassDay) == 0 {
		return fmt.Errorf("%s 문화센터(%s) 강좌 데이터 파싱이 실패하였습니다(요일이 없음)", e.name, storeName)
	}
	dayOfTheWeek := lsrld.ClassDay[0]
	if len(dayOfTheWeek) == 0 {
		return fmt.Errorf("%s 문화센터(%s) 강좌 데이터 파싱이 실패하였습니다(요일:%s)", e.name, storeName, dayOfTheWeek)
	}

	// 강좌횟수
	count := fmt.Sprintf("%d", lsrld.ClassTimes)
	if len(count) == 0 {
		return fmt.Errorf("%s 문화센터(%s) 강좌 데이터 파싱이 실패하였습니다(강좌 횟수:%s)", e.name, storeName, count)
	}

	// 접수상태
	var status = domain.ReceptionStatusUnknown
	switch lsrld.ClassStatus {
	case "접수중":
		status = domain.ReceptionStatusPossible
	case "접수마감", "정원마감":
		status = domain.ReceptionStatusClosed
	case "접수대기":
		status = domain.ReceptionStatusStnadBy
	default:
		return fmt.Errorf("%s 문화센터(%s) 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(%s)", e.name, storeName, lsrld.ClassStatus)
	}

	c <- &domain.Lecture{
		StoreName:      fmt.Sprintf("%s %s", e.name, storeName),
		Group:          "",
		Title:          lsrld.ClassTitle,
		Teacher:        "",
		StartDate:      startDate,
		StartTime:      startTime,
		EndTime:        endTime,
		DayOfTheWeek:   dayOfTheWeek + "요일",
		Price:          fmt.Sprintf("%d", lsrld.ClassFee),
		Count:          count,
		Status:         status,
		DetailPageUrl:  fmt.Sprintf("%s/class/%s", e.cultureBaseUrl, lsrld.ClassID),
		ScrapeExcluded: false,
	}

	return nil
}

func (e *Emart) validCultureLectureStore(storeCode, storeName string) (bool, error) {
	var ssrd emartStoreSearchResultData
	err := e.requestSite("{\"query\":\"query getStoreAreaList($isAll: Boolean!) {\\n  getStoreAreaList(isAll: $isAll) {\\n    PK\\n    area\\n    storeListInfo {\\n      storeName\\n      storeCode\\n      storeCenter\\n    }\\n  }\\n}\\n\",\"variables\":{\"isAll\":false}}", &ssrd)
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

func (e *Emart) validCultureLectureGroup() (bool, error) {
	var lgsrd emartLectureGroupSearchResultData
	err := e.requestSite("{\"query\":\"query getCategoryList {\\n  getCategoryList {\\n    message {\\n      mainCategory {\\n        PK\\n        SK\\n        mainCategoryOrder\\n        subCategoryOrder\\n        categoryCode\\n        categoryName\\n        useFlag\\n        iconFileName\\n      }\\n      subCategory {\\n        PK\\n        SK\\n        mainCategoryOrder\\n        subCategoryOrder\\n        categoryCode\\n        categoryName\\n        useFlag\\n        iconFileName\\n        mainDisplayFlag\\n        iconFilePath {\\n          bucket\\n          filename\\n          key\\n          region\\n        }\\n      }\\n    }\\n  }\\n}\\n\",\"variables\":{}}", &lgsrd)
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

func (e *Emart) requestSite(body string, v interface{}) error {
	clPageUrl := "https://o27tfdumlrbf7jmrvql76qbhsm.appsync-api.ap-northeast-2.amazonaws.com/graphql"

	req, err := http.NewRequest("POST", clPageUrl, bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("http.NewRequest failed: %v", err)
	}

	req.Header.Set("Authorization", emartAuthToken)
	req.Header.Set("origin", e.cultureBaseUrl)
	req.Header.Set("referer", e.cultureBaseUrl)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/101.0.4951.67 Safari/537.36")
	req.Header.Set("x-amz-user-agent", "aws-amplify/3.8.14 js")
	req.Header.Set("x-api-key", emartApiKey)

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("client.Do failed: %v", err)
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("Request failed with Status: %d", res.StatusCode)
	}

	//goland:noinspection GoUnhandledErrorResult
	defer res.Body.Close()

	resBodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("io.ReadAll failed: %v", err)
	}

	err = json.Unmarshal(resBodyBytes, v)
	if err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v", err)
	}

	return nil
}
