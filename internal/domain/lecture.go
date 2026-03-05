package domain

// @@@@@
// Lecture는 문화센터 강좌 하나의 정보를 담는 도맩인 구조체입니다.
// 각 수집기(Provider)가 사이트에서 파싱한 데이터를 이 구조체로 변환하여
// 필터링과 CSV 저장의 공통 단위로 사용합니다.
type Lecture struct {
	StoreName    string // 점포 이름 (예: "이마트 여수")
	Group        string // 강좌 그룹 (예: "Kids & Children"). 제공하지 않는 수집기 기준 빈 문자열일 수 있음.
	Title        string // 강좌명
	Teacher      string // 강사명. 제공하지 않는 수집기 기준 빈 문자열일 수 있음.
	StartDate    string // 개강일 (YYYY-MM-DD 형식)
	StartTime    string // 시작시간 (hh:mm, 24시간 형식)
	EndTime      string // 종료시간 (hh:mm, 24시간 형식)
	DayOfTheWeek string // 요일 (예: "월요일")
	Price        string // 수강료 (수집기마다 형식이 다를 수 있음. 예: "60,000원" 또는 "60000")
	Count        string // 강좌횟수 (예: "12횟")

	// Status는 현재 접수 가능 여부를 나타내며, CSV 저장 시 영문 문자열로 변환됩니다.
	Status        ReceptionStatus
	DetailPageUrl string // 수집기마다 다를 수 있는 강좌 상세페이지 URL

	// ScrapeExcluded가 true이면 필터링 조건에 의해 제외 대상으로 표시된 강좌입니다.
	// 데이터 삭제가 아니라 플래그만 설정하여, CSV 내보내기 단계에서 해당 강좌를 건너끁니다.
	ScrapeExcluded bool
}

// ReceptionStatus는 강좌의 접수 상태를 나타내는 열거형 타입입니다.
// uint 기반으로 iota를 사용하여 각 상태값을 0부터 순서대로 할당합니다.
// CSV 저장 시에는 ReceptionStatusString 배열을 통해 한글 수자열로 변환합니다.
type ReceptionStatus uint

// 강좌 접수 상태 상수입니다.
// ReceptionStatusMax는 상태값의 주의 개수를 나타내며, 배열 크기 정의에 사용됩니다.
// 새로운 상태를 추가할 때는 ReceptionStatusMax 앞에 삽입하고
// ReceptionStatusString 에도 맞차 한글 이름을 추가해야 합니다.
const (
	ReceptionStatusUnknown                   ReceptionStatus = iota // 알수없음: 파싱 불가 등 예외 상황
	ReceptionStatusPlanned                                          // 접수예정: 아직 접수 시작 전
	ReceptionStatusPossible                                         // 접수가능: 현재 온라인 접수 가능
	ReceptionStatusClosed                                           // 접수마감: 정원 또는 기간 마감
	ReceptionStatusStandBy                                          // 대기신청: 정원 초과, 대기자 등록 가능
	ReceptionStatusVisitConsultation                                // 방문상담: 센터 방문 후 상담 필요
	ReceptionStatusVisitFirstComeFirstServed                        // 방문선착순: 방문하여 선착순 신청
	ReceptionStatusVisitInquiry                                     // 현장문의: 현장에서 직접 문의 필요
	ReceptionStatusTellInquiry                                      // 전화문의: 전화로 문의 필요
	ReceptionStatusDayParticipation                                 // 당일참여: 당일 현장 참여 방식
	ReceptionStatusMax                                              // 상태값 개수 (배열 크기 정의용 센티넬)
)

// ReceptionStatusString은 ReceptionStatus 정수를 인덱스로 배열에서 CSV 출력용 한글 문자열을 조회합니다.
// ReceptionStatusMax를 배열 크기로 사용하면 새 상태 추가 시 컴파일러가 배열 크기 불일치를 자동으로 감지합니다.
var ReceptionStatusString = [ReceptionStatusMax]string{"알수없음", "접수예정", "접수가능", "접수마감", "대기신청", "방문상담", "방문선착순", "현장문의", "전화문의", "당일참여"}
