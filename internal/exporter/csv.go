package exporter

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

// @@@@@
// csvExporter는 Exporter 인터페이스의 CSV 파일 구현체입니다.
// 외부에 직접 노출하지 않고(소문자) NewCSVExporter 생성자를 통해서만 생성합니다.
type csvExporter struct {
	fileName string // 저장할 CSV 파일의 경로 및 이름
}

// NewCSVExporter는 지정된 파일명으로 CSV 파일을 저장하는 Exporter를 생성합니다.
// 구체 타입(*csvExporter)이 아닌 Exporter 인터페이스를 반환하여,
// 호출자가 CSV 구현의 내부 세부사항에 의존하지 않도록 합니다.
func NewCSVExporter(fileName string) Exporter {
	return &csvExporter{
		fileName: fileName,
	}
}

// Export는 수집된 강좌 목록을 CSV 파일로 저장합니다.
// ScrapeExcluded가 true로 표시된 강좌는 필터링되어 CSV에 포함되지 않습니다.
func (e *csvExporter) Export(lectures []domain.Lecture) error {
	log.Println("수집된 문화센터 강좌 자료를 CSV 파일로 저장합니다.")

	// 파일이 이미 존재하면 덮어씁니다. 실패 시 권한 문제나 경로 오류일 가능성이 높습니다.
	f, err := os.Create(e.fileName)
	if err != nil {
		return fmt.Errorf("CSV 파일 생성 실패: %v", err)
	}

	//goland:noinspection GoUnhandledErrorResult
	defer f.Close()

	// 엑셀(Excel) 등에서 한글이 깨지지 않도록 파일 맨 앞에 UTF-8 BOM(Byte Order Mark)을 삽입합니다.
	// BOM 없이 저장하면 일부 프로그램에서 한글이 깨지는 문제가 발생합니다.
	_, err = f.WriteString("\xEF\xBB\xBF")
	if err != nil {
		return fmt.Errorf("UTF-8 BOM 쓰기 실패: %v", err)
	}

	// csv.Writer는 내부 버퍼를 사용하며, Flush()를 호출해야 파일에 실제로 기록됩니다.
	w := csv.NewWriter(f)

	// CSV 첫 번째 행에 컬럼 헤더를 씁니다. 헤더 순서는 아래 레코드 쓰기 순서와 반드시 일치해야 합니다.
	headers := []string{"점포", "강좌그룹", "강좌명", "강사명", "개강일", "시작시간", "종료시간", "요일", "수강료", "강좌횟수", "접수상태", "상세페이지"}
	if err := w.Write(headers); err != nil {
		return fmt.Errorf("CSV 헤더 쓰기 실패: %v", err)
	}

	// 필터링 단계에서 제외 표시된 강좌(ScrapeExcluded=true)를 건너뛰고,
	// 나머지 강좌를 헤더와 동일한 순서로 한 행씩 CSV에 기록합니다.
	count := 0
	for _, lecture := range lectures {
		if lecture.ScrapeExcluded == true {
			continue
		}

		r := []string{
			lecture.StoreName,
			lecture.Group,
			lecture.Title,
			lecture.Teacher,
			lecture.StartDate,
			lecture.StartTime,
			lecture.EndTime,
			lecture.DayOfTheWeek,
			lecture.Price,
			lecture.Count,
			domain.ReceptionStatusString[lecture.Status],
			lecture.DetailPageUrl,
		}
		if err := w.Write(r); err != nil {
			return fmt.Errorf("CSV 레코드 쓰기 실패: %v", err)
		}
		count++
	}

	// 내부 버퍼에 남아있는 데이터를 파일에 모두 기록합니다.
	// Flush() 후 반드시 Error()로 쓰기 중 발생한 오류를 확인해야 합니다.
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("CSV 버퍼 비우기 실패: %v", err)
	}

	log.Printf("수집된 문화센터 강좌 자료(%d건)를 CSV 파일(%s)로 저장하였습니다.", count, e.fileName)

	return nil
}
