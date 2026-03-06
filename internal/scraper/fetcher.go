package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	// defaultTimeout HTTP 요청의 최대 허용 시간입니다.
	// TCP 연결부터 응답 바디를 모두 다 읽을 때까지 전체 과정에 적용됩니다.
	defaultTimeout = 30 * time.Second

	// defaultUserAgent HTTP 요청 시 자동으로 설정되는 User-Agent 헤더 값입니다.
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/101.0.4951.67 Safari/537.36"
)

// Fetcher 스크래퍼에서 공통으로 사용하는 HTTP 통신 도우미 객체입니다.
// 응답 형식에 따른 파싱 함수(FetchJSON, FetchGoQuery 등)를 제공하여 각 수집기(Provider)가 동일한 방식으로 HTTP 요청을 수행하도록 돕습니다.
type Fetcher struct {
	client *http.Client
}

// NewFetcher 기본 설정이 적용된 새로운 Fetcher를 생성합니다.
func NewFetcher() *Fetcher {
	// http.DefaultTransport의 안전한 복사본을 만듭니다.
	// 전역 기본값을 직접 수정하면 프로그램 전체의 다른 HTTP 요청에도 영향을 주기 때문에
	// Clone()을 통해 기본 설정을 안전하게 상속받으면서 독립적으로 커스터마이징합니다.
	transport := http.DefaultTransport.(*http.Transport).Clone()

	// 1. 커넥션 풀(Connection Pool) 설정
	// 반복적인 HTTP 요청 시 발생하는 고비용의 3-Way Handshake 및 TLS 세션 협상 오버헤드를 줄이기 위해,
	// 대상 호스트와의 TCP 커넥션을 지속(Keep-Alive)하고 재사용하도록 커넥션 풀 크기를 최적화합니다.
	transport.MaxIdleConns = 100       // 전체 호스트를 합산한 최대 유휴(Idle) 커넥션 수
	transport.MaxIdleConnsPerHost = 50 // 호스트당 최대 유휴 커넥션 수
	transport.MaxConnsPerHost = 50     // 호스트당 전체 동시 허용 커넥션 수 (유휴 + 활성)

	// 2. 타임아웃 및 정리 설정
	transport.IdleConnTimeout = 90 * time.Second       // 유휴 커넥션이 닫히기 전까지 풀에 유지할 최대 시간
	transport.TLSHandshakeTimeout = 10 * time.Second   // TLS 핸드셰이크를 실패로 판단할 때까지의 최대 대기 시간
	transport.ResponseHeaderTimeout = 15 * time.Second // 요청을 보낸 후 응답 헤더를 받기까지 기다릴 최대 시간

	return &Fetcher{
		client: &http.Client{
			Transport: transport,

			// 앞선 설정들이 과정별(연결, 헤더 수신 등)로 세분화된 타임아웃이라면,
			// 이 설정은 "요청 시작부터 전체 내용을 다 받을 때까지" 걸리는 총 허용 시간을 의미합니다.
			Timeout: defaultTimeout,
		},
	}
}

// do HTTP 요청 전송을 전담하는 내부 래퍼(Wrapper) 메서드입니다.
// 헤더 주입 등 모든 HTTP 요청에 공통으로 필요한 전처리 작업을 중앙 집중화하여 수행합니다.
func (f *Fetcher) do(req *http.Request) (*http.Response, error) {
	// 호출 측에서 User-Agent를 명시하지 않은 경우, 기본값을 할당하여 차단을 방지합니다.
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}

	res, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("대상 서버와의 HTTP 통신을 수행할 수 없습니다: %w", err)
	}

	// 주의: 물리적 통신 성공만 보장하며, 응답 상태 코드(예: 404, 500)에 대한 검증은 호출자에게 책임 위임
	return res, nil
}

// FetchBody HTTP 요청을 실행하고, 응답 코드가 200 OK인 경우 본문(Body) 데이터를 읽어 반환합니다.
func (f *Fetcher) FetchBody(req *http.Request) ([]byte, error) {
	res, err := f.do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// TCP 커넥션 풀을 효율적으로 재사용하기 위해 남은 Body 데이터를 끝까지 비워냅니다.
		_, _ = io.Copy(io.Discard, res.Body)

		return nil, fmt.Errorf("대상 서버가 유효하지 않은 HTTP 응답 상태를 반환하였습니다 (상태 코드: %d)", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("대상 서버의 응답 스트림을 수신하는 과정에서 오류가 발생하였습니다: %w", err)
	}

	return body, nil
}

// FetchJSON HTTP 요청을 실행하고, JSON 응답을 지정된 구조체(v)에 파싱하여 채웁니다.
func (f *Fetcher) FetchJSON(req *http.Request, v any) error {
	body, err := f.FetchBody(req)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("수신된 응답 데이터를 JSON 형식으로 파싱하는 과정에서 오류가 발생하였습니다: %w", err)
	}

	return nil
}

// FetchHTML HTTP 요청을 실행하고, HTML 응답을 goquery.Document로 파싱하여 반환합니다.
func (f *Fetcher) FetchHTML(req *http.Request) (*goquery.Document, error) {
	body, err := f.FetchBody(req)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("수신된 HTML 문서를 DOM 구조로 파싱하는 과정에서 오류가 발생하였습니다: %w", err)
	}

	return doc, nil
}
