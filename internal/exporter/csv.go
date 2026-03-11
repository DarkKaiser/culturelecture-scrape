package exporter

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// csvExporter Exporter 인터페이스의 CSV 파일 구현체입니다.
type csvExporter struct {
	filename string // 저장할 CSV 파일의 경로 및 이름
}

// 컴파일 타임에 인터페이스 구현 여부를 검증합니다.
var _ Exporter = (*csvExporter)(nil)

// NewCSV 지정된 파일명으로 CSV 파일을 저장하는 Exporter를 생성합니다.
func NewCSV(filename string) Exporter {
	return &csvExporter{
		filename: filename,
	}
}

// Export 수집된 강좌 목록을 CSV 파일로 저장합니다.
func (e *csvExporter) Export(lectures []domain.Lecture) error {
	log.Println("수집된 강좌 데이터를 CSV 포맷으로 내보내는 작업을 시작합니다.")

	// 파일이 이미 존재하면 덮어씁니다. 실패 시 권한 문제나 경로 오류일 가능성이 높습니다.
	f, err := os.Create(e.filename)
	if err != nil {
		return fmt.Errorf("파일 입출력 오류: 지정된 경로에 CSV 파일을 생성할 수 없습니다. 상세 오류: %v", err)
	}
	defer f.Close()

	// 엑셀 등에서 한글이 깨지지 않도록 파일 맨 앞에 UTF-8 BOM(Byte Order Mark)을 삽입합니다.
	// BOM 없이 저장하면 일부 프로그램에서 한글이 깨지는 문제가 발생합니다.
	_, err = f.WriteString("\xEF\xBB\xBF")
	if err != nil {
		return fmt.Errorf("인코딩 오류: 파일 최상단에 UTF-8 BOM을 기록하는 데 실패하였습니다. 상세 오류: %v", err)
	}

	// 파일 쓰기 속도를 최적화하기 위해, 데이터를 디스크에 한 줄씩 쓰지 않고 메모리에 모아두었다가
	// 한 번에 기록(버퍼링)하는 Writer 객체를 생성합니다.
	w := csv.NewWriter(f)

	// CSV 첫 번째 행에 컬럼 헤더를 씁니다. 헤더 순서는 아래 레코드 쓰기 순서와 반드시 일치해야 합니다.
	headers := []string{"점포", "강좌그룹", "강좌명", "강사명", "개강일", "시작시간", "종료시간", "요일", "수강료", "강좌횟수", "접수상태", "상세페이지"}
	if err := w.Write(headers); err != nil {
		return fmt.Errorf("파일 쓰기 오류: CSV 파일의 헤더(열 제목)를 기록하는 데 실패하였습니다. 상세 오류: %v", err)
	}

	// 필터링 단계에서 제외 표시된 강좌(ScrapeExcluded=true)는 건너뛰고,
	// 나머지 강좌를 헤더와 동일한 순서로 한 행씩 CSV 파일에 기록합니다.
	savedCount := 0
	for _, lecture := range lectures {
		if lecture.ScrapeExcluded == true {
			continue
		}

		r := []string{
			lecture.StoreName,
			lecture.Category,
			lecture.Title,
			lecture.Instructor,
			lecture.StartDate,
			lecture.StartTime,
			lecture.EndTime,
			lecture.Weekday,
			lecture.Price,
			lecture.SessionCount,
			domain.ReceptionStatusStrings[lecture.Status],
			lecture.DetailPageURL,
		}
		if err := w.Write(r); err != nil {
			return fmt.Errorf("파일 쓰기 오류: 강좌 데이터 레코드를 CSV 파일에 기록하는 데 실패하였습니다. 상세 오류: %v", err)
		}

		savedCount++
	}

	// 내부 버퍼에 남아있는 데이터를 파일에 모두 기록합니다.
	// Flush() 후 반드시 Error()로 쓰기 중 발생한 오류를 확인해야 합니다.
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("파일 쓰기 오류: 메모리 버퍼에 남은 데이터를 CSV 파일로 최종 출력하는 데 실패하였습니다. 상세 오류: %v", err)
	}

	log.Printf("총 %d건의 강좌 데이터를 CSV 파일(%s)에 성공적으로 저장하였습니다.", savedCount, e.filename)

	return nil
}
