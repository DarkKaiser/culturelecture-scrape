package scraper

import (
	"context"
	"fmt"
	"log"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// SearchCriteria 사용자가 설정한 검색 조건(연도, 시즌)을 하나로 묶은 구조체입니다.
// 이마트, 홈플러스, 롯데마트 스크래퍼가 모두 동일한 기준으로 강좌를 수집할 수 있도록 각 스크래퍼에 공통으로 전달됩니다.
type SearchCriteria struct {
	// SearchYear 수집 대상 연도를 나타냅니다. (예: "2025")
	SearchYear string

	// SearchSeason 수집 대상 시즌을 사람이 읽기 쉬운 한글 명칭으로 나타냅니다. (예: "봄", "여름", "가을", "겨울")
	SearchSeason string

	// SearchSeasonCode 일부 API(예: 롯데마트)가 한글 대신 숫자 코드로 시즌을 요구하기 때문에
	// SearchSeason을 숫자로 변환한 값입니다. (봄:1 / 여름:2 / 가을:3 / 겨울:4)
	SearchSeasonCode string
}

// Scraper 각 마트(이마트, 홈플러스, 롯데마트) 수집기가 공통으로 제공해야 할 기능들을 정의하는 인터페이스입니다.
type Scraper interface {
	// Name 각 스크래퍼가 로깅, 오류 추적 시 자신을 식별할 수 있도록 고유한 명칭을 반환합니다.
	// (예: "이마트", "홈플러스", "롯데마트")
	Name() string

	// Validate 본격적인 강좌 수집 전에 설정이 올바른지 확인합니다.
	// 예를 들어, 점포 코드가 실제로 존재하는지, API 토큰이 유효한지 등을 검사합니다.
	// 설정에 문제가 있으면 error를 반환하여 잘못된 설정으로 수집이 진행되는 것을 사전에 방지합니다.
	Validate(ctx context.Context) error

	// Scrape 해당 마트의 문화센터 서버에 접속하여 강좌 목록을 가져옵니다.
	// 수집된 강좌들은 []domain.Lecture 타입으로 반환되며, 수집 실패 시 error를 반환합니다.
	Scrape(ctx context.Context) ([]domain.Lecture, error)
}

// Scrape 전달받은 수집기 목록을 사용하여 모든 마트의 강좌를 수집한 뒤, 하나로 합쳐 반환합니다.
// 수집은 두 단계로 진행됩니다.
//
//  1. 검증: 모든 수집기의 설정이 올바른지 먼저 확인합니다. 단 하나라도 문제가 있으면 수집을 시작하지 않습니다.
//  2. 수집: 검증이 통과된 수집기들을 동시에 실행하여 강좌 목록을 빠르게 가져옵니다.
//     수집 중 어느 한 곳에서 오류가 발생하면 나머지 수집도 함께 중단됩니다.
func Scrape(ctx context.Context, scrapers []Scraper) ([]domain.Lecture, error) {
	// -----------------------------------------------------------------------
	// 1단계: 사전 검증
	//
	// 잘못된 설정(점포 코드, API 토큰 등)으로 수집이 진행되면 강좌 데이터가 누락될 수 있습니다.
	// 이를 막기 위해 수집 전에 모든 수집기의 설정을 동시에 검증하며, 단 하나라도 실패하면
	// 즉시 에러를 반환하고 수집을 시작하지 않습니다.
	// -----------------------------------------------------------------------

	// valGroup은 모든 수집기의 검증 고루틴을 하나로 묶어 관리합니다.
	// 어느 하나라도 오류를 반환하면 valCtx가 즉시 취소되어 나머지 검증 작업도 중단됩니다.
	valGroup, valCtx := errgroup.WithContext(ctx)

	for _, sc := range scrapers {
		// 고루틴 클로저는 루프 변수를 직접 참조하므로, 루프가 끝나면 의도하지 않은 값을 가질 수 있습니다.
		// 로컬 변수에 복사해 각 고루틴이 자신만의 올바른 수집기 인스턴스를 참조하도록 합니다.
		sc := sc

		valGroup.Go(func() error {
			if err := sc.Validate(valCtx); err != nil {
				return err
			}
			return nil
		})
	}

	// 모든 검증 고루틴이 끝날 때까지 기다립니다.
	// 오류가 있으면 수집 단계로 넘어가지 않고 즉시 반환합니다.
	if err := valGroup.Wait(); err != nil {
		return nil, err
	}

	// -----------------------------------------------------------------------
	// 2단계: 병렬 수집
	//
	// 검증이 통과된 모든 수집기를 동시에 실행하여 강좌 목록을 빠르게 가져옵니다.
	// -----------------------------------------------------------------------

	// 최종 결과를 누적할 슬라이스입니다. 각 수집기가 가져온 강좌들이 여기에 모입니다.
	var lectures []domain.Lecture

	// 여러 수집기가 동시에 실행되므로, 같은 슬라이스에 동시에 쓰기를 시도하면 데이터가 손상될 수 있습니다.
	// Mutex를 사용해 한 번에 하나의 수집기만 슬라이스에 쓸 수 있도록 순서를 보장합니다.
	var mu sync.Mutex

	// scrapeGroup은 모든 수집기의 고루틴을 하나로 묶어 관리합니다.
	// 어느 하나라도 오류를 반환하면 scrapeCtx가 즉시 취소되어 나머지 수집 작업도 중단됩니다.
	scrapeGroup, scrapeCtx := errgroup.WithContext(ctx)

	for _, sc := range scrapers {
		// 고루틴 클로저는 루프 변수를 직접 참조하므로, 루프가 끝나면 의도하지 않은 값을 가질 수 있습니다.
		// 로컬 변수에 복사해 각 고루틴이 자신만의 올바른 수집기 인스턴스를 참조하도록 합니다.
		sc := sc

		scrapeGroup.Go(func() error {
			log.Printf("%s 문화센터 강좌 수집을 시작합니다.", sc.Name())

			// 해당 마트의 서버에서 강좌 목록을 가져옵니다.
			batch, err := sc.Scrape(scrapeCtx)
			if err != nil {
				log.Printf("%s 강좌 수집 중 오류가 발생하였습니다: %v", sc.Name(), err)

				return fmt.Errorf("%s 강좌 수집 중 오류가 발생하였습니다: %w", sc.Name(), err)
			}

			log.Printf("%s 문화센터 강좌 수집이 완료되었습니다. (수집 건수: %d건)", sc.Name(), len(batch))

			// 수집이 완료된 강좌 목록을 공유 슬라이스에 추가합니다.
			// Mutex로 잠근 후 추가하기 때문에 다른 수집기와 동시에 실행되어도 데이터가 뒤섞이지 않습니다.
			mu.Lock()
			lectures = append(lectures, batch...)
			mu.Unlock()

			return nil
		})
	}

	// 모든 수집 고루틴이 끝날 때까지 기다립니다.
	// 오류가 있으면 지금까지 수집된 강좌와 함께 반환합니다.
	if err := scrapeGroup.Wait(); err != nil {
		return lectures, err
	}

	log.Printf("문화센터 강좌 수집이 모두 완료되었습니다. (수집된 총 강좌 수: %d개)", len(lectures))

	return lectures, nil
}
