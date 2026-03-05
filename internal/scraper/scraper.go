package scraper

import (
	"context"
	"fmt"
	"log"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

type Config struct {
	SearchYear       string
	SearchSeason     string
	SearchSeasonCode string
	EmartAuthToken   string
}

type Scraper interface {
	Validate(ctx context.Context) error
	ScrapeCultureLectures(ctx context.Context) ([]domain.Lecture, error)
}

func Scrape(ctx context.Context, scrapers []Scraper) ([]domain.Lecture, error) {
	// 1. 사전 검증 단계 (Validation Phase)
	g_val, valCtx := errgroup.WithContext(ctx)
	for _, sc := range scrapers {
		sc := sc // Closure 이슈 방지
		g_val.Go(func() error {
			if err := sc.Validate(valCtx); err != nil {
				return err
			}
			return nil
		})
	}
	if err := g_val.Wait(); err != nil {
		return nil, err
	}

	// 2. 데이터 수집 단계 (Scraping Phase)
	var lectures []domain.Lecture
	var mu sync.Mutex

	g, groupCtx := errgroup.WithContext(ctx)

	for _, sc := range scrapers {
		sc := sc // Closure 이슈 방지
		g.Go(func() error {
			result, err := sc.ScrapeCultureLectures(groupCtx)
			if err != nil {
				log.Printf("스크래핑 작업 중 오류 발생: %v", err)
				return fmt.Errorf("수집 작업 중 오류 발생: %w", err)
			}

			mu.Lock()
			lectures = append(lectures, result...)
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return lectures, err
	}

	log.Printf("문화센터 강좌 수집이 완료되었습니다. 총 %d개의 강좌가 수집되었습니다.", len(lectures))
	return lectures, nil
}
