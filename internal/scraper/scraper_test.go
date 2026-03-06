package scraper_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
	"github.com/darkkaiser/culturelecture-scrape/internal/scraper"
)

// MockScraper는 테스트를 위해 Scraper 인터페이스를 흉내 내는 Mock 객체입니다.
// 각 테스트 시나리오에 맞게 ValidateFunc와 ScrapeFunc를 주입하여 동작을 제어합니다.
type MockScraper struct {
	ValidateFunc func(ctx context.Context) error
	ScrapeFunc   func(ctx context.Context) ([]domain.Lecture, error)
}

func (m *MockScraper) Validate(ctx context.Context) error {
	if m.ValidateFunc != nil {
		return m.ValidateFunc(ctx)
	}
	return nil
}

func (m *MockScraper) Scrape(ctx context.Context) ([]domain.Lecture, error) {
	if m.ScrapeFunc != nil {
		return m.ScrapeFunc(ctx)
	}
	return nil, nil
}

// TestScrape_Success는 모든 수집기가 정상적으로 동작하여
// 수집된 강좌들이 하나의 슬라이스로 잘 합쳐지는지 확인합니다.
func TestScrape_Success(t *testing.T) {
	t.Parallel()

	// 1번 수집기가 수집할 임의의 강좌 2개
	lectures1 := []domain.Lecture{
		{StoreName: "이마트 여수", Title: "강좌1"},
		{StoreName: "이마트 여수", Title: "강좌2"},
	}

	// 2번 수집기가 수집할 임의의 강좌 1개
	lectures2 := []domain.Lecture{
		{StoreName: "홈플러스 여수", Title: "강좌3"},
	}

	scrapers := []scraper.Scraper{
		&MockScraper{
			// 검증은 문제없이 통과
			ValidateFunc: func(ctx context.Context) error { return nil },
			// 수집 시 lectures1 반환
			ScrapeFunc: func(ctx context.Context) ([]domain.Lecture, error) { return lectures1, nil },
		},
		&MockScraper{
			ValidateFunc: func(ctx context.Context) error { return nil },
			ScrapeFunc:   func(ctx context.Context) ([]domain.Lecture, error) { return lectures2, nil },
		},
	}

	ctx := context.Background()

	// Action
	results, err := scraper.Scrape(ctx, scrapers)

	// Assert
	if err != nil {
		t.Fatalf("예상치 못한 에러가 발생했습니다: %v", err)
	}

	// 총 3개(2+1)의 강좌가 수집되어야 함
	if len(results) != 3 {
		t.Errorf("예상되는 강좌 수: %d, 실제: %d", 3, len(results))
	}

	// 결과에는 lectures1과 lectures2의 요소가 모두 포함되어야 합니다.
	// 병렬 수집이므로 순서는 보장되지 않으므로, Title로 포함 여부를 확인합니다.
	foundMap := make(map[string]bool)
	for _, l := range results {
		foundMap[l.Title] = true
	}

	expectedTitles := []string{"강좌1", "강좌2", "강좌3"}
	for _, title := range expectedTitles {
		if !foundMap[title] {
			t.Errorf("결과에 필요한 강좌가 누락되었습니다: %s", title)
		}
	}
}

// TestScrape_ValidateError는 여러 개 중 단 하나의 수집기라도 Validate에서 에러를 뱉으면,
// Scrape 함수가 전혀 실행되지 않고 즉시 해당 에러를 반환하는지 검증합니다.
func TestScrape_ValidateError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("검증 실패 오류")

	var scrapeCallCount int32

	scrapers := []scraper.Scraper{
		&MockScraper{ // 정상 수집기
			ValidateFunc: func(ctx context.Context) error { return nil },
			ScrapeFunc: func(ctx context.Context) ([]domain.Lecture, error) {
				atomic.AddInt32(&scrapeCallCount, 1)
				return []domain.Lecture{{Title: "정상"}}, nil
			},
		},
		&MockScraper{ // 실패 수집기 (이곳에서 에러 발생)
			ValidateFunc: func(ctx context.Context) error { return expectedErr },
			ScrapeFunc: func(ctx context.Context) ([]domain.Lecture, error) {
				atomic.AddInt32(&scrapeCallCount, 1)
				return nil, nil
			},
		},
	}

	ctx := context.Background()
	results, err := scraper.Scrape(ctx, scrapers)

	if err == nil {
		t.Fatal("에러가 반환되어야 하지만, nil이 반환되었습니다.")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("기대하는 에러: %v, 실제 에러: %v", expectedErr, err)
	}
	if results != nil {
		t.Errorf("검증 실패 시 결과는 nil이어야 하지만, %v가 반환되었습니다.", results)
	}

	// 중요한 검증 포인트: Validate에서 에러가 났으므로 ScrapeFunc는 단 한 번도 호출되지 않아야 합니다.
	if atomic.LoadInt32(&scrapeCallCount) > 0 {
		t.Errorf("Validate에서 에러가 발생했음에도 ScrapeFunc가 호출되었습니다.")
	}
}

// TestScrape_ScrapeError는 설정(Validate)은 모두 정상이지만,
// 데이터를 가져오는 중(Scrape) 오류가 발생했을 때 프로그램이 멈추지 않고 반환하는지 확인합니다.
func TestScrape_ScrapeError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("데이터 수집 네트워크 오류")

	scrapers := []scraper.Scraper{
		&MockScraper{
			ValidateFunc: func(ctx context.Context) error { return nil },
			ScrapeFunc: func(ctx context.Context) ([]domain.Lecture, error) {
				return []domain.Lecture{{Title: "1번"}}, nil
			},
		},
		&MockScraper{
			ValidateFunc: func(ctx context.Context) error { return nil },
			ScrapeFunc: func(ctx context.Context) ([]domain.Lecture, error) {
				return nil, expectedErr // 여기서 에러 반환
			},
		},
	}

	ctx := context.Background()
	_, err := scraper.Scrape(ctx, scrapers)

	if err == nil {
		t.Fatal("수집 중 에러가 반환되어야 하지만, nil입니다.")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("기대하는 에러: %v, 실제 에러: %v", expectedErr, err)
	}
}

// TestScrape_EmptyScrapers는 수집기 목록이 아예 비어있을 때
// 에러 없이 빈 배열을 잘 반환하는지 확인합니다.
func TestScrape_EmptyScrapers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	results, err := scraper.Scrape(ctx, []scraper.Scraper{})

	if err != nil {
		t.Fatalf("빈 수집기 목록일 때 에러가 나면 안 됩니다. 실제 에러: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("결과는 빈 슬라이스여야 하지만, 크기가 %d입니다.", len(results))
	}
}

// TestScrape_ContextCancellation는 작업 도중에 외부(호출자)에서 문맥(Context)을 취소할 경우,
// 수집 작업이 타임아웃/취소 처리에 의해 무한히 대기하지 않고 즉시 리턴하는지를 확인합니다.
func TestScrape_ContextCancellation(t *testing.T) {
	t.Parallel()

	// 50ms라는 아주 짧은 시간 뒤에 자동으로 취소되는 컨텍스트 생성
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	scrapers := []scraper.Scraper{
		&MockScraper{
			ValidateFunc: func(ctx context.Context) error { return nil },
			ScrapeFunc: func(ctx context.Context) ([]domain.Lecture, error) {
				// 취소될 때까지 무한히 블로킹되는 함수 흉내
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(10 * time.Second): // 실제 타임아웃보다 긴 시간
					return []domain.Lecture{{Title: "결코 도달할 수 없는 곳"}}, nil
				}
			},
		},
	}

	_, err := scraper.Scrape(ctx, scrapers)

	if err == nil {
		t.Fatal("Context 취소가 발생하면 에러가 반환되어야 합니다.")
	}

	// 컨텍스트에 의한 취소/만료 에러인지 확인
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("예상되는 Context 에러 대신 다른 에러가 발생했습니다: %v", err)
	}
}
