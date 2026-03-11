package exporter

import "github.com/darkkaiser/culturelecture-scrape/internal/domain"

// Exporter 수집된 강좌 데이터를 특정 형식의 파일로 내보내는 기능을 정의하는 인터페이스입니다.
type Exporter interface {
	// Export 필터링된 강좌 목록을 받아 지정된 형식의 파일로 저장합니다.
	Export(lectures []domain.Lecture) error
}
