package provider

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
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
	lmStartDateRegex = regexp.MustCompile(`^[0-9]{4}\.[0-9]{2}\.[0-9]{2}`)
	lmStartTimeRegex = regexp.MustCompile(" [0-9]{2}:[0-9]{2}")
	lmEndTimeRegex   = regexp.MustCompile("[0-9]{2}:[0-9]{2}$")
	lmDayOfWeekRegex = regexp.MustCompile(`\([월화수목금토일]`)
	lmPriceRegex     = regexp.MustCompile("[0-9,]{1,8}원$")
	lmCountRegex     = regexp.MustCompile("[0-9]{1,3}회")
)

type Lottemart struct {
	name           string
	cultureBaseUrl string

	searchTermCode string // 검색년도 & 검색시즌 코드
	client         *scraper.Client

	storeCodeMap        map[string]string            // 점포
	lectureGroupCodeMap map[string]map[string]string // 강좌군
}

func NewLottemart(cfg scraper.Config) (*Lottemart, error) {
	searchYear := strutil.NormalizeSpace(cfg.SearchYear)
	searchSeasonCode := strutil.NormalizeSpace(cfg.SearchSeasonCode)

	if searchYear == "" || searchSeasonCode == "" {
		return nil, fmt.Errorf("검색년도 및 검색시즌코드는 빈 문자열을 허용하지 않습니다(검색년도:%s, 검색시즌코드:%s)", searchYear, searchSeasonCode)
	}

	return &Lottemart{
		name: "롯데마트",

		cultureBaseUrl: "https://culture.lottemart.com",

		searchTermCode: fmt.Sprintf("%s0%s", searchYear, searchSeasonCode),
		client:         scraper.NewClient(),

		storeCodeMap: map[string]string{
			"705": "여수점",
		},

		lectureGroupCodeMap: map[string]map[string]string{
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

func (l *Lottemart) Validate(ctx context.Context) error {
	// 강좌군이 유효한지 확인한다.
	validLG, err := l.validCultureLectureGroup(ctx)
	if err != nil {
		return fmt.Errorf("%s 문화센터 강좌군 검증 실패: %v", l.name, err)
	}
	if !validLG {
		return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌군코드 불일치)", l.name)
	}

	// 모든 점포가 유효한지 확인한다.
	for storeCode, storeName := range l.storeCodeMap {
		validStore, err := l.validCultureLectureStore(ctx, storeCode, storeName)
		if err != nil {
			return fmt.Errorf("%s 문화센터 점포 검증 실패(점포코드:%s): %w", l.name, storeCode, err)
		}
		if !validStore {
			return fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(점포코드 불일치:%s)", l.name, storeCode)
		}
	}

	return nil
}

func (l *Lottemart) ScrapeCultureLectures(ctx context.Context) ([]domain.Lecture, error) {

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(5) // HTTP 요청 부하 분산을 위한 동시성 제한

	var lectureList []domain.Lecture
	var mu sync.Mutex

	var count int64 = 0
	for storeCode, storeName := range l.storeCodeMap {
		storeCode, storeName := storeCode, storeName

		// 점포 단위 스크래핑을 백그라운드 태스크로 분리하여 컨텍스트 및 에러 관리를 errgroup 내에 둔다.
		g.Go(func() error {
			// 불러올 전체 페이지 갯수를 구한다.
			_, doc, err := l.cultureLecturePageDocument(groupCtx, 1, storeCode)
			if err != nil {
				return fmt.Errorf("%s 문화센터 강좌를 수집하는 중에 전체 페이지수 확인 문서 로드에 실패하였습니다(점포코드:%s): %w", l.name, storeCode, err)
			}
			pi, exists := doc.Find("tr:last-child").Attr("pageinfo")
			if exists == false {
				return fmt.Errorf("%s 문화센터 강좌를 수집하는 중에 전체 페이지 갯수 추출이 실패하였습니다.", l.name)
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
			piSplit := strings.Split(pi, "|")
			if len(piSplit) != 6 {
				return fmt.Errorf("%s 문화센터 강좌를 수집하는 중에 전체 페이지 갯수 추출이 실패하였습니다.(pageinfo:%s)", l.name, pi)
			}

			totalPageCount, err := strconv.Atoi(piSplit[1])
			if err != nil {
				return fmt.Errorf("페이지수 파싱 오류: %w", err)
			}

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
					clPageUrl, doc, err := l.cultureLecturePageDocument(innerCtx, pageNo, storeCode)
					if err != nil {
						return fmt.Errorf("%s 문화센터(%s) 페이지(pageNo:%d) 조회 실패: %w", l.name, storeName, pageNo, err)
					}

					var pageLectures []domain.Lecture
					var extractErr error
					clSelection := doc.Find("tr")
					clSelection.Each(func(i int, s *goquery.Selection) {
						if extractErr != nil {
							return
						}
						atomic.AddInt64(&count, 1)
						lecture, err := l.extractCultureLecture(innerCtx, clPageUrl, storeCode, storeName, s)
						if err != nil {
							extractErr = fmt.Errorf("%s 문화센터 추출 오류: %w", l.name, err)
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

func (l *Lottemart) getDetailPageURL(classCode string, storeCode string) string {
	return fmt.Sprintf("%s/cu/gus/course/courseinfo/courseview.do?cls_cd=%s&is_category_open=N&search_term_cd=%s&search_str_cd=%s", l.cultureBaseUrl, classCode, l.searchTermCode, storeCode)
}

func (l *Lottemart) extractCultureLecture(ctx context.Context, clPageUrl string, storeCode string, storeName string, s *goquery.Selection) (*domain.Lecture, error) {
	// 강좌의 컬럼 개수를 확인한다.
	ls := s.Find("td")
	if ls.Length() != 5 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌 컬럼 개수 불일치:%d, URL:%s)", l.name, ls.Length(), clPageUrl)
	}

	// 강사명, 형식 : 김준희
	lectureCol2 := strutil.NormalizeSpace(ls.Eq(1).Text())
	// 개강일/요일/시간, 형식 : 2020.12.05(토) 15:20~16:00
	lectureCol3 := strutil.NormalizeSpace(ls.Eq(2).Text())
	// 수강료, 형식 : 12회 80,000원 60,000원
	lectureCol4 := strutil.NormalizeSpace(ls.Eq(3).Text())
	// 접수상태/수강신청, 형식 : 바로신청
	lectureCol5 := strutil.NormalizeSpace(ls.Eq(4).Find("div > div > a.btn-status:last-child").Text())

	// 강좌명
	lts := ls.Eq(0).Find("div.info-txt > a")
	if lts.Length() == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(강좌명 <a> 태그를 찾을 수 없습니다, URL:%s)", l.name, clPageUrl)
	}
	title := strutil.NormalizeSpace(lts.Text())

	// 개강일
	startDate := lmStartDateRegex.FindString(lectureCol3)
	if len(startDate) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol3, clPageUrl)
	}
	startDate = strings.ReplaceAll(startDate, ".", "-")

	// 시작시간, 종료시간
	startTime := strings.TrimSpace(lmStartTimeRegex.FindString(lectureCol3))
	endTime := strings.TrimSpace(lmEndTimeRegex.FindString(lectureCol3))
	if len(startDate) == 0 || len(endTime) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol3, clPageUrl)
	}

	// 요일
	dayOfTheWeek := lmDayOfWeekRegex.FindString(lectureCol3)
	if len(dayOfTheWeek) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol3, clPageUrl)
	}
	dayOfTheWeek = string([]rune(dayOfTheWeek)[1:])

	// 수강료
	price := lmPriceRegex.FindString(lectureCol4)
	if strings.Contains(price, "원") == false {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol4, clPageUrl)
	}

	// 강좌횟수
	count := lmCountRegex.FindString(lectureCol4)
	if len(count) == 0 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(분석데이터:%s, URL:%s)", l.name, lectureCol4, clPageUrl)
	}

	// 접수상태
	var status = domain.ReceptionStatusUnknown
	switch lectureCol5 {
	case "바로신청":
		status = domain.ReceptionStatusPossible
	case "접수마감":
		status = domain.ReceptionStatusClosed
	case "대기자 신청":
		status = domain.ReceptionStatusStandBy
	case "현장문의":
		status = domain.ReceptionStatusVisitInquiry
	case "전화문의":
		status = domain.ReceptionStatusTellInquiry
	case "현장접수":
		status = domain.ReceptionStatusVisitInquiry
	default:
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(지원하지 않는 접수상태입니다(분석데이터:%s, URL:%s)", l.name, lectureCol5, clPageUrl)
	}
	// 상세페이지
	classCode, exists := lts.Attr("onclick")
	if !exists {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지 주소를 찾을 수 없습니다, URL:%s)", l.name, clPageUrl)
	}
	parts := strings.Split(classCode, "'")
	if len(parts) < 3 {
		return nil, fmt.Errorf("%s 문화센터 강좌 데이터 파싱이 실패하였습니다(상세페이지 주소를 찾을 수 없습니다, URL:%s)", l.name, clPageUrl)
	}
	classCode = parts[1]

	if len(title) == 0 {
		return nil, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &domain.Lecture{
		StoreName:      fmt.Sprintf("%s %s", l.name, storeName),
		Group:          "",
		Title:          title,
		Teacher:        lectureCol2,
		StartDate:      startDate,
		StartTime:      startTime,
		EndTime:        endTime,
		DayOfTheWeek:   dayOfTheWeek + "요일",
		Price:          price,
		Count:          count,
		Status:         status,
		DetailPageUrl:  l.getDetailPageURL(classCode, storeCode),
		ScrapeExcluded: false,
	}, nil
}

func (l *Lottemart) cultureLecturePageDocument(ctx context.Context, pageNo int, storeCode string) (string, *goquery.Document, error) {
	clPageUrl := fmt.Sprintf("%s/cu/gus/course/courseinfo/searchList.do", l.cultureBaseUrl)

	paramArrCatCd := ""
	paramSearchCatCd := ""
	for _, v := range l.lectureGroupCodeMap {
		for lectureGroupCode := range v {
			if paramSearchCatCd != "" {
				paramSearchCatCd += ","
			}
			paramSearchCatCd += lectureGroupCode

			if paramArrCatCd != "" {
				paramArrCatCd += "&"
			}
			paramArrCatCd += fmt.Sprintf("arr_cat_cd=%s", lectureGroupCode)
		}
	}
	reqBody := bytes.NewBufferString(fmt.Sprintf("currPageNo=%d&search_list_type=&search_str_cd=%s&search_order_gbn=&search_reg_status=&is_category_open=Y&from_fg=&cls_cd=&fam_no=&wish_typ=&search_term_cd=%s&search_day_fg=&search_cls_nm=&search_cat_cd=%s&search_opt_cd=&search_tit_cd=&%s", pageNo, storeCode, l.searchTermCode, paramSearchCatCd, paramArrCatCd))

	req, err := http.NewRequestWithContext(ctx, "POST", clPageUrl, reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("http.NewRequestWithContext failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	resBodyBytes, err := l.client.FetchBody(req)
	if err != nil {
		return "", nil, fmt.Errorf("%s 문화센터 요청 실패: %w", l.name, err)
	}

	// 실제 불러온 데이터는 '<table>' 태그가 포함되어 있지 않고 '<tr>', '<td>'만 있는 형태!!
	// 이 형태에서 goquery.NewDocumentFromReader() 함수를 호출하면 '<tr>', '<td>' 태그가 모두 사라지므로 '<table>' 태그를 강제로 붙여준다.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table>" + string(resBodyBytes) + "</table>"))
	if err != nil {
		return "", nil, fmt.Errorf("goquery.NewDocumentFromReader failed: %v", err)
	}

	return clPageUrl, doc, nil
}

func (l *Lottemart) validCultureLectureStore(ctx context.Context, storeCode, storeName string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/cu/branch/main.do?search_str_cd=%s", l.cultureBaseUrl, storeCode), nil)
	if err != nil {
		return false, fmt.Errorf("http.NewRequestWithContext failed: %w", err)
	}

	doc, err := l.client.FetchGoQuery(req)
	if err != nil {
		return false, err
	}

	vSelection := doc.Find("#contents div.branch_main-wrap div.branch_info-area > div.branch_spot-area > h3")
	if vSelection.Length() != 1 || strutil.NormalizeSpace(vSelection.Text()) != storeName {
		return false, nil
	}

	return true, nil
}

func (l *Lottemart) validCultureLectureGroup(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/cu/gus/course/courseinfo/courselist.do", l.cultureBaseUrl), nil)
	if err != nil {
		return false, fmt.Errorf("http.NewRequestWithContext failed: %w", err)
	}

	doc, err := l.client.FetchGoQuery(req)
	if err != nil {
		return false, err
	}

	for lectureGroupsID, v := range l.lectureGroupCodeMap {
		lectureGroupsIDSelection := doc.Find(fmt.Sprintf("#%s", lectureGroupsID))
		if lectureGroupsIDSelection.Length() != 1 {
			return false, nil
		}

		for lectureGroupCode, lectureGroupName := range v {
			if lectureGroupName == "" {
				continue
			}

			lectureGroupSelection := lectureGroupsIDSelection.Parent().Parent().Parent().Find(fmt.Sprintf("dd > ul > li > div > input[value='%s']", lectureGroupCode))
			if lectureGroupSelection.Length() != 1 || strutil.NormalizeSpace(lectureGroupSelection.Parent().Text()) != lectureGroupName {
				return false, nil
			}
		}
	}

	return true, nil
}
