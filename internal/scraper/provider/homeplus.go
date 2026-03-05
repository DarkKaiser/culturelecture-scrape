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
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/PuerkitoBio/goquery"
	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
	"github.com/darkkaiser/notify-server/pkg/strutil"
)

var (
	hpTeacherRegex   = regexp.MustCompile("^(.)*강사")
	hpStartDateRegex = regexp.MustCompile("[0-9]{4}.[0-9]{2}.[0-9]{2} ~")
	hpStartTimeRegex = regexp.MustCompile("[0-9]{2}:[0-9]{2} ~")
	hpEndTimeRegex   = regexp.MustCompile("~ [0-9]{2}:[0-9]{2}")
	hpDayOfWeekRegex = regexp.MustCompile("^[월화수목금토일] ")
	hpPeopleFixRegex = regexp.MustCompile(`\s*\(\d+인 기준\)`)
	hpPriceRegex     = regexp.MustCompile(" [0-9]{1,3}(,[0-9]{3})*원$")
	hpCountRegex     = regexp.MustCompile("^[0-9]{1,3}회")
)

const homeplusLectureSearchPageSize = 20

type Homeplus struct {
	name           string
	cultureBaseUrl string
	client         *scraper.Client

	storeCodeMap        map[string]string // 점포
	lectureGroupCodeMap map[string]string // 강좌군
}

type homeplusStoreSearchResult struct {
	RstCode    int    `json:"RstCode"`
	RstMessage string `json:"RstMessage"`
	Data       struct {
		StoreList []struct {
			StoreAreaName        string      `json:"StoreAreaName"`
			StoreCode            string      `json:"StoreCode"`
			StoreName            string      `json:"StoreName"`
			RegionHQID           int         `json:"RegionHQID"`
			SortingNumber        int         `json:"SortingNumber"`
			RealStoreCode        string      `json:"RealStoreCode"`
			PhoneNumber          string      `json:"PhoneNumber"`
			FaxNumber            interface{} `json:"FaxNumber"`
			ZipCode              string      `json:"ZipCode"`
			Address1             string      `json:"Address1"`
			Address2             string      `json:"Address2"`
			AddressPrevVer       string      `json:"AddressPrevVer"`
			OperaterName         interface{} `json:"OperaterName"`
			OperatorMobileNumber string      `json:"OperatorMobileNumber"`
		} `json:"StoreList"`
		MyStoreList []struct {
			StoreAreaName        interface{} `json:"StoreAreaName"`
			StoreCode            string      `json:"StoreCode"`
			StoreName            string      `json:"StoreName"`
			RegionHQID           int         `json:"RegionHQID"`
			SortingNumber        int         `json:"SortingNumber"`
			RealStoreCode        interface{} `json:"RealStoreCode"`
			PhoneNumber          interface{} `json:"PhoneNumber"`
			FaxNumber            interface{} `json:"FaxNumber"`
			ZipCode              interface{} `json:"ZipCode"`
			Address1             interface{} `json:"Address1"`
			Address2             interface{} `json:"Address2"`
			AddressPrevVer       interface{} `json:"AddressPrevVer"`
			OperaterName         interface{} `json:"OperaterName"`
			OperatorMobileNumber interface{} `json:"OperatorMobileNumber"`
		} `json:"MyStoreList"`
	} `json:"Data"`
}

func NewHomeplus(cfg scraper.Config) (*Homeplus, error) {
	return &Homeplus{
		name: "홈플러스",

		cultureBaseUrl: "https://mschool.homeplus.co.kr",
		client:         scraper.NewClient(),

		storeCodeMap: map[string]string{
			"0035": "광양점",
			"0030": "순천점",
		},

		lectureGroupCodeMap: map[string]string{
			"MH|EL|IF": "Kids 전체",
			"BB":       "Baby 전체",
		},
	}, nil
}

func (h *Homeplus) Validate(ctx context.Context) error {
	// 점포가 유효한지 확인한다.
	validStore, err := h.validCultureLectureStore(ctx)
	if err != nil {
		return fmt.Errorf("%s 문화센터 점포 검증 실패: %v", h.name, err)
	}
	if !validStore {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(점포코드 불일치)", h.name)
	}
	// 강좌군이 유효한지 확인한다.
	validLG, err := h.validCultureLectureGroup(ctx)
	if err != nil {
		return fmt.Errorf("%s 문화센터 강좌군 검증 실패: %v", h.name, err)
	}
	if !validLG {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌군코드 불일치)", h.name)
	}
	return nil
}

func (h *Homeplus) ScrapeCultureLectures(ctx context.Context) ([]domain.Lecture, error) {

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(5) // HTTP 요청 부하 분산을 위한 동시성 제한

	var lectureList []domain.Lecture
	var mu sync.Mutex

	var totalExtractionLectureCount int64 = 0
	for storeCode, storeName := range h.storeCodeMap {
		storeCode, storeName := storeCode, storeName

		// 점포 단위 스크래핑을 백그라운드 태스크로 분리하여 컨텍스트 및 에러 관리를 errgroup 내에 둔다.
		g.Go(func() error {
			// 불러올 전체 강좌 갯수를 구한다.
			_, doc, err := h.cultureLecturePageDocument(groupCtx, 1, storeCode, storeName)
			if err != nil {
				return fmt.Errorf("%s 문화센터 전체 강좌 갯수 확인용 페이지 로드 실패(점포:%s): %w", h.name, storeName, err)
			}
			value := doc.Find("#divTotalCnt").Text()
			if len(value) == 0 {
				return fmt.Errorf("%s 문화센터 강좌를 수집하는 중에 전체 강좌 갯수 추출이 실패하였습니다.", h.name)
			}
			totalLectureCount, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("전체 강좌 갯수 파싱 오류: %w", err)
			}

			// 불러올 전체 페이지 갯수를 구한다.
			totalPageCount := int(math.Ceil(float64(totalLectureCount) / homeplusLectureSearchPageSize))

			// 강좌 데이터를 수집한다. (동일 점포 내에서도 병렬로 요청)
			var innerG *errgroup.Group
			var innerCtx context.Context
			innerG, innerCtx = errgroup.WithContext(groupCtx)
			innerG.SetLimit(10) // 하위 요청(강좌 목록 페이징)에 대한 동시성 제한

			for pageNo := 1; pageNo <= totalPageCount; pageNo++ {
				if err := innerCtx.Err(); err != nil {
					break
				}
				pageNo := pageNo
				innerG.Go(func() error {
					clPageUrl, doc, err := h.cultureLecturePageDocument(innerCtx, pageNo, storeCode, storeName)
					if err != nil {
						return fmt.Errorf("%s 문화센터(%s) 페이지(pageNo:%d) 로드 실패: %w", h.name, storeName, pageNo, err)
					}

					var pageLectures []domain.Lecture
					var extractErr error
					clSelection := doc.Find("li > div.result_info_wrap")
					clSelection.Each(func(i int, s *goquery.Selection) {
						if extractErr != nil {
							return
						}
						atomic.AddInt64(&totalExtractionLectureCount, 1)
						lecture, err := h.extractCultureLecture(innerCtx, clPageUrl, storeName, s)
						if err != nil {
							extractErr = fmt.Errorf("%s 문화센터 강좌 추출 오류: %w", h.name, err)
							return
						}
						if lecture != nil {
							pageLectures = append(pageLectures, *lecture)
						}
					})

					if extractErr != nil {
						return extractErr
					}

					if len(pageLectures) > 0 {
						mu.Lock()
						lectureList = append(lectureList, pageLectures...)
						mu.Unlock()
					}
					return nil
				})
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

func (h *Homeplus) cultureLecturePageDocument(ctx context.Context, pageNo int, storeCode, storeName string) (string, *goquery.Document, error) {
	clPageUrl := fmt.Sprintf("%s/Lecture/GetSearchResult", h.cultureBaseUrl)

	form := url.Values{}
	form.Set("page", strconv.Itoa(pageNo))
	form.Set("pageSize", strconv.Itoa(homeplusLectureSearchPageSize))

	var paramIdx = 0
	h.setLectureSearchParam(form, paramIdx, "", storeName, storeCode, "")
	for lectureGroupCode, lectureGroupName := range h.lectureGroupCodeMap {
		paramIdx++
		h.setLectureSearchParam(form, paramIdx, "", lectureGroupName, "", lectureGroupCode)
	}
	form.Set("word", "")
	form.Set("sort", "1")

	reqBody := strings.NewReader(form.Encode())
	req, err := http.NewRequestWithContext(ctx, "POST", clPageUrl, reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("http.NewRequestWithContext failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	doc, err := h.client.FetchGoQuery(req)
	if err != nil {
		return "", nil, fmt.Errorf("%s 문화센터 요청 실패: %w", h.name, err)
	}

	return clPageUrl, doc, nil
}

func (h *Homeplus) setLectureSearchParam(form url.Values, paramIdx int, id, txt, storeCode, lectureGroupCode string) {
	prefix := fmt.Sprintf("prm[%d]", paramIdx)
	form.Set(prefix+"[Id]", id)
	form.Set(prefix+"[Txt]", txt)
	form.Set(prefix+"[Data][StoreCode]", storeCode)
	form.Set(prefix+"[Data][LectureTarget]", lectureGroupCode)
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

func (h *Homeplus) getDetailPageURL(lectureMasterID string) string {
	return fmt.Sprintf("%s/Lecture/GetLectureDetail?lectureMasterID=%s", h.cultureBaseUrl, lectureMasterID)
}

func (h *Homeplus) extractCultureLecture(ctx context.Context, clPageUrl string, storeName string, s *goquery.Selection) (*domain.Lecture, error) {
	// 강좌 그룹
	title1 := strutil.NormalizeSpace(s.Find("div.title_1").Text())
	// 강좌명
	title2 := strutil.NormalizeSpace(s.Find("div.title_2").Text())
	// 요일/시간, 형식 : 일 14:20 ~ 15:00
	info4 := strutil.NormalizeSpace(s.Find("div.info_4").Text())

	ls := s.Find("div.info_5")
	if ls.Length() != 3 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 컬럼 개수 불일치:%d, URL:%s)", h.name, ls.Length(), clPageUrl)
	}
	// 강좌횟수/수강료, 형식 : 1회 6,000원
	info5Idx0 := strutil.NormalizeSpace(ls.Eq(0).Text())
	// 개강일, 형식 : 2023.08.20 ~ 2023.08.20
	info5Idx1 := strutil.NormalizeSpace(ls.Eq(1).Text())
	// 강사명, 형식 : 신혜정 강사
	info5Idx2 := strutil.NormalizeSpace(ls.Eq(2).Text())

	// 강좌그룹
	if len(title1) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 그룹명이 빈 문자열입니다, URL:%s)", h.name, clPageUrl)
	}
	group := title1

	// 강좌명
	if len(title2) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌명이 빈 문자열입니다, URL:%s)", h.name, clPageUrl)
	}
	title := title2

	// 강사
	teacher := strutil.NormalizeSpace(hpTeacherRegex.FindString(info5Idx2))
	if len(teacher) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5Idx2, clPageUrl)
	}

	// 개강일
	startDate := strutil.NormalizeSpace(hpStartDateRegex.FindString(info5Idx1))
	if len(startDate) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5Idx1, clPageUrl)
	}
	startDate = strings.ReplaceAll(startDate[:len(startDate)-2], ".", "-")

	// 시작시간, 종료시간
	startTime := hpStartTimeRegex.FindString(info4)
	endTime := hpEndTimeRegex.FindString(info4)
	if len(startTime) == 0 || len(endTime) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info4, clPageUrl)
	}
	startTime = strutil.NormalizeSpace(startTime[:len(startTime)-1])
	endTime = strutil.NormalizeSpace(endTime[1:])

	// 요일
	dayOfTheWeek := strutil.NormalizeSpace(hpDayOfWeekRegex.FindString(info4))
	if len(dayOfTheWeek) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info4, clPageUrl)
	}

	// 수강료

	// '1회 7,000원 (2인 기준)' => '1회 7,000원'
	info5Idx0 = strutil.NormalizeSpace(hpPeopleFixRegex.ReplaceAllString(info5Idx0, ""))

	price := strutil.NormalizeSpace(hpPriceRegex.FindString(info5Idx0))
	if len(price) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5Idx0, clPageUrl)
	}

	// 강좌횟수
	count := strutil.NormalizeSpace(hpCountRegex.FindString(info5Idx0))
	if len(count) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", h.name, info5Idx0, clPageUrl)
	}

	// 접수상태
	classCartImgUrl, exists := s.Find("button.btn_class_cart > img").Attr("src")
	if exists == false {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(접수상태 추출이 실패하였습니다, URL:%s)", h.name, clPageUrl)
	}
	classCartStatus := strutil.NormalizeSpace(s.Find("button.btn_class_cart > span:last-child").Text())

	var status = domain.ReceptionStatusUnknown
	switch classCartImgUrl {
	case "/images/ico/icon_cart_3.png":
		if classCartStatus == "대기" {
			status = domain.ReceptionStatusStandBy
		} else if classCartStatus == "강의 장바구니 담기" {
			status = domain.ReceptionStatusPossible
		} else {
			return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", h.name, classCartImgUrl, clPageUrl)
		}
	case "/images/ico/icon_cart_4.png":
		if classCartStatus == "마감" {
			status = domain.ReceptionStatusClosed
		} else if classCartStatus == "방문" {
			status = domain.ReceptionStatusVisitConsultation
		} else if classCartStatus == "문의" {
			status = domain.ReceptionStatusVisitInquiry
		} else {
			return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", h.name, classCartImgUrl, clPageUrl)
		}
	default:
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", h.name, classCartImgUrl, clPageUrl)
	}

	// 상세페이지로 이동하기 위한 LectureMasterID를 구한다.
	idSelection := s.Find("input[name=LectureMasterID]")
	lectureMasterId, exists := idSelection.Attr("value")
	if exists == false {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지로 이동하기 위해 필요한 [ LectureMasterID ] 값이 비어 있습니다, URL:%s)", h.name, clPageUrl)
	}

	// 빈 강좌명 필터링 (기존 for루프 역할 이동)
	if len(title) == 0 {
		return nil, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &domain.Lecture{
		StoreName:      fmt.Sprintf("%s %s", h.name, storeName),
		Group:          group,
		Title:          title,
		Teacher:        teacher,
		StartDate:      startDate,
		StartTime:      startTime,
		EndTime:        endTime,
		DayOfTheWeek:   fmt.Sprintf("%s요일", dayOfTheWeek),
		Price:          price,
		Count:          count,
		Status:         status,
		DetailPageUrl:  h.getDetailPageURL(lectureMasterId),
		ScrapeExcluded: false,
	}, nil
}

func (h *Homeplus) validCultureLectureStore(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/Store/GetStoreList", h.cultureBaseUrl), nil)
	if err != nil {
		return false, fmt.Errorf("http.NewRequestWithContext failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	var storeSearchResult homeplusStoreSearchResult
	err = h.client.FetchJSON(req, &storeSearchResult)
	if err != nil {
		return false, err
	}

	for storeCode, storeName := range h.storeCodeMap {
		foundStore := false
		for _, elem := range storeSearchResult.Data.StoreList {
			if storeCode == elem.StoreCode && storeName == elem.StoreName {
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

func (h *Homeplus) validCultureLectureGroup(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/Lecture/Search", h.cultureBaseUrl), nil)
	if err != nil {
		return false, fmt.Errorf("http.NewRequestWithContext failed: %w", err)
	}

	doc, err := h.client.FetchGoQuery(req)
	if err != nil {
		return false, err
	}

	for lectureGroupCode, lectureGroupName := range h.lectureGroupCodeMap {
		lectureGroupSelection := doc.Find(fmt.Sprintf("section.search_body div.menu_depth_2_wrap ul.tree_menu_2 > li.depth_2 > ul.depth_3 > li:first-child > button[data-lecture-target='%s']", lectureGroupCode))
		if lectureGroupSelection.Length() != 1 {
			return false, nil
		}

		val := lectureGroupSelection.Text()
		if strutil.NormalizeSpace(val) != lectureGroupName {
			return false, nil
		}
	}

	return true, nil
}
