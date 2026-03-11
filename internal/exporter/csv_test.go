package exporter

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkkaiser/culturelecture-scrape/internal/domain"
)

func TestCsvExporter_Export(t *testing.T) {
	tempDir := t.TempDir()

	// 테스트용 가짜(Mock) 강좌 데이터 세팅
	dummyLectures := []domain.Lecture{
		{
			StoreName:      "테스트점",
			Category:       "어린이",
			Title:          "재미있는 코딩",
			Instructor:     "김코딩",
			StartDate:      "2026-05-01",
			StartTime:      "14:00",
			EndTime:        "15:00",
			Weekday:        "월요일",
			Price:          "50000",
			SessionCount:   "4회",
			Status:         domain.ReceptionStatusPossible,
			DetailPageURL:  "http://test.com/1",
			ScrapeExcluded: false, // 포함되어야 함
		},
		{
			StoreName:      "테스트점2",
			Category:       "성인",
			Title:          "필터링 강좌",
			Instructor:     "이필터",
			StartDate:      "2026-05-02",
			StartTime:      "10:00",
			EndTime:        "11:00",
			Weekday:        "화요일",
			Price:          "10000",
			SessionCount:   "1회",
			Status:         domain.ReceptionStatusClosed,
			DetailPageURL:  "http://test.com/2",
			ScrapeExcluded: true, // 제외(필터링)되어야 함 -> CSV에 안 쓰여야 함
		},
	}

	tests := []struct {
		name        string
		filename    string
		lectures    []domain.Lecture
		wantErr     bool
		errContains string
		validate    func(t *testing.T, filePath string)
	}{
		{
			name:     "성공: 정상적인 데이터 쓰기 (필터링 적용 확인 및 BOM 검증)",
			filename: filepath.Join(tempDir, "success_test.csv"),
			lectures: dummyLectures,
			wantErr:  false,
			validate: func(t *testing.T, filePath string) {
				// 파일 존재 확인
				data, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("생성된 CSV 파일을 읽을 수 없습니다: %v", err)
				}

				// 1. BOM(Byte Order Mark)이 최상단에 올바르게 삽입되었는지 검증
				if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
					t.Errorf("UTF-8 BOM이 파일 첫 부분에 존재하지 않습니다.")
				}

				// 2. CSV 내용 파싱 검증
				// BOM 뒤부터 읽기 위해 슬라이싱
				reader := csv.NewReader(strings.NewReader(string(data[3:])))
				records, err := reader.ReadAll()
				if err != nil {
					t.Fatalf("생성된 파일이 올바른 CSV 형식이 아닙니다: %v", err)
				}

				// 예상 라인 수: 헤더(1줄) + 정상데이터(ScrapeExcluded=false인 1줄) = 총 2줄
				if len(records) != 2 {
					t.Errorf("저장된 CSV 레코드 수 불일치. 기댓값=2, 실젯값=%d (ScrapeExcluded 필터 동작 오류 가능성)", len(records))
				}

				// 헤더 검증
				expectedHeader := []string{"점포", "강좌그룹", "강좌명", "강사명", "개강일", "시작시간", "종료시간", "요일", "수강료", "강좌횟수", "접수상태", "상세페이지"}
				for i, h := range records[0] {
					if expectedHeader[i] != h {
						t.Errorf("헤더 불일치: 기댓값=%s, 실젯값=%s", expectedHeader[i], h)
					}
				}

				// 데이터 row 검증 (domain.ReceptionStatusStrings 매핑 정상 동작 확인 포함)
				if records[1][0] != "테스트점" || records[1][2] != "재미있는 코딩" || records[1][10] != "접수가능" {
					t.Errorf("저장된 데이터 열 내용이 예상과 다릅니다: %v", records[1])
				}
			},
		},
		{
			name:     "성공: 강좌 데이터가 0건일 때 쓰기 (헤더만 정상 저장되는지 확인)",
			filename: filepath.Join(tempDir, "empty_test.csv"),
			lectures: []domain.Lecture{},
			wantErr:  false,
			validate: func(t *testing.T, filePath string) {
				data, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("파열을 읽을 수 없습니다: %v", err)
				}
				reader := csv.NewReader(strings.NewReader(string(data[3:])))
				records, _ := reader.ReadAll()

				// 예상: 헤더(1줄)만 존재해야 함
				if len(records) != 1 {
					t.Errorf("빈 배열 저장 시 헤더만 기록되어야 합니다. 실젯값: %d", len(records))
				}
			},
		},
		{
			name:     "성공: 모든 데이터가 필터링(ScrapeExcluded=true)되었을 때 쓰기",
			filename: filepath.Join(tempDir, "all_filtered_test.csv"),
			lectures: []domain.Lecture{
				dummyLectures[1], // 필터링 강좌만 포함
				dummyLectures[1],
			},
			wantErr: false,
			validate: func(t *testing.T, filePath string) {
				data, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("파일을 읽을 수 없습니다: %v", err)
				}
				reader := csv.NewReader(strings.NewReader(string(data[3:])))
				records, _ := reader.ReadAll()

				// 예상: 헤더(1줄)만 존재해야 함
				if len(records) != 1 {
					t.Errorf("모든 레코드가 스킵되었으므로 헤더만 기록되어야 합니다. 실젯값: %d", len(records))
				}
			},
		},
		{
			name:        "실패: 잘못된 파일 경로 (파일 생성 실패)",
			filename:    filepath.Join(tempDir, "not_exist_dir", "error_test.csv"), // 존재하지 않는 상위 디렉토리
			lectures:    dummyLectures,
			wantErr:     true,
			errContains: "파일 입출력 오류: 지정된 경로에 CSV 파일을 생성할 수 없습니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := NewCSV(tt.filename)

			err := exporter.Export(tt.lectures)

			// 에러 발생 유무 검증
			if (err != nil) != tt.wantErr {
				t.Fatalf("Export() error = %v, wantErr %v", err, tt.wantErr)
			}

			// 특정 에러 문자열 포함 여부 검증
			if tt.wantErr && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Export() 에러 메시지 불일치\nExpected contain: %v\nGot: %v", tt.errContains, err.Error())
				}
			}

			// 성공 시 내부 데이터 검증 로직 실행
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, tt.filename)
			}
		})
	}
}
