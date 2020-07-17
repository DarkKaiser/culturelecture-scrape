package lectures

type Lecture struct {
	StoreName      string          // 점포
	Group          string          // 강좌그룹
	Title          string          // 강좌명
	Teacher        string          // 강사명
	StartDate      string          // 개강일(YYYY-MM-DD)
	StartTime      string          // 시작시간(hh:mm) : 24시간 형식
	EndTime        string          // 종료시간(hh:mm) : 24시간 형식
	DayOfTheWeek   string          // 요일
	Price          string          // 수강료
	Count          string          // 강좌횟수
	Status         ReceptionStatus // 접수상태
	DetailPageUrl  string          // 상세페이지
	ScrapeExcluded bool            // 필터링에 걸려서 파일 저장시 제외되는지의 여부(csv 파일에 포함되지 않는다)
}

// 접수상태
type ReceptionStatus uint

// 지원가능한 접수상태 값
const (
	ReceptionStatusUnknown                   ReceptionStatus = iota // 알수없음
	ReceptionStatusPossible                                         // 접수가능
	ReceptionStatusClosed                                           // 접수마감
	ReceptionStatusStnadBy                                          // 대기신청
	ReceptionStatusVisitConsultation                                // 방문상담
	ReceptionStatusVisitFirstComeFirstServed                        // 방문선착순
	ReceptionStatusDayParticipation                                 // 당일참여
)

// 지원가능한 접수상태 문자열
var ReceptionStatusString = []string{"알수없음", "접수가능", "접수마감", "대기신청", "방문상담", "방문선착순", "당일참여"}
