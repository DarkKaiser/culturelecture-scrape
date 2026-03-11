package domain

// Lecture 문화센터 강좌 하나의 정보를 담는 도메인 구조체입니다.
// 각 수집기(Provider)가 사이트에서 파싱한 데이터를 이 구조체로 변환하여 필터링과 CSV 저장의 공통 단위로 사용합니다.
type Lecture struct {
	// ------------------------------------------------------------------
	// 1. 강좌 원본 데이터
	// ------------------------------------------------------------------

	// [기본 정보]
	StoreName     string // 점포 이름 (예: "이마트 여수")
	Category      string // 강좌 그룹 (예: "Kids & Children")
	Title         string // 강좌명
	Instructor    string // 강사명
	DetailPageURL string // 강좌 상세페이지 URL

	// [일정 및 강의 정보]
	StartDate    string // 개강일 (YYYY-MM-DD 형식)
	StartTime    string // 시작시간 (hh:mm, 24시간 형식)
	EndTime      string // 종료시간 (hh:mm, 24시간 형식)
	Weekday      string // 요일 (예: "월요일")
	Price        string // 수강료 (수집기마다 형식이 다를 수 있음. 예: "60,000원" 또는 "60000")
	SessionCount string // 강좌횟수 (예: "12회")

	// [상태 정보]
	Status ReceptionStatus // 현재 접수 가능 여부

	// ------------------------------------------------------------------
	// 2. 애플리케이션 내부 제어 상태
	// ------------------------------------------------------------------

	// ScrapeExcluded가 true이면 필터링 조건에 의해 제외 대상으로 표시된 강좌입니다.
	// 데이터 삭제가 아니라 플래그만 설정하여, CSV 내보내기 단계에서 해당 강좌를 건너뜁니다.
	ScrapeExcluded bool
}

// ReceptionStatus 강좌의 접수 상태를 나타내는 열거형 타입입니다.
type ReceptionStatus uint

// 강좌 접수 상태 상수입니다.
const (
	ReceptionStatusUnknown            ReceptionStatus = iota // 알수없음: 파싱 불가 등 예외 상황
	ReceptionStatusPlanned                                   // 접수예정: 아직 접수 시작 전
	ReceptionStatusPossible                                  // 접수가능: 현재 온라인 접수 가능
	ReceptionStatusClosed                                    // 접수마감: 정원 또는 기간 마감
	ReceptionStatusStandBy                                   // 대기신청: 정원 초과, 대기자 등록 가능
	ReceptionStatusOnsiteConsultation                        // 방문상담: 센터 방문 후 상담 필요
	ReceptionStatusOnsiteFCFS                                // 방문선착순: 방문하여 선착순 신청
	ReceptionStatusOnsiteInquiry                             // 현장문의: 현장에서 직접 문의 필요
	ReceptionStatusPhoneInquiry                              // 전화문의: 전화로 문의 필요
	ReceptionStatusWalkIn                                    // 당일참여: 당일 현장 참여 방식
	ReceptionStatusMax                                       // 상태값 개수 (배열 크기 정의용 센티넬)
)

// ReceptionStatusStrings 각 열거형 상태값에 대응하는 한글 문자열을 저장한 배열입니다.
// CSV 등 외부 파일에 상태를 기록할 때 내부 인덱스를 이 배열의 한글 값으로 변환하여 사용합니다.
var ReceptionStatusStrings = [ReceptionStatusMax]string{"알수없음", "접수예정", "접수가능", "접수마감", "대기신청", "방문상담", "방문선착순", "현장문의", "전화문의", "당일참여"}
