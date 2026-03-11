package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewFetcher(t *testing.T) {
	fetcher := NewFetcher()

	if fetcher == nil {
		t.Fatal("NewFetcher()는 nil을 반환해서는 안 됩니다.")
	}

	if fetcher.client == nil {
		t.Fatal("NewFetcher()의 내부 HTTP 클라이언트는 생성되어야 합니다.")
	}

	if fetcher.client.Timeout != defaultTimeout {
		t.Errorf("클라이언트 Timeout 불일치. 기대값: %v, 실제값: %v", defaultTimeout, fetcher.client.Timeout)
	}

	transport, ok := fetcher.client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("생성된 클라이언트의 트랜스포트 계층은 *http.Transport 타입이어야 합니다.")
	}

	// 커넥션 풀 및 주요 타임아웃 세팅 확인 검증
	if transport.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns 불일치. 기대값: 100, 실제값: %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 50 {
		t.Errorf("MaxIdleConnsPerHost 불일치. 기대값: 50, 실제값: %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 50 {
		t.Errorf("MaxConnsPerHost 불일치. 기대값: 50, 실제값: %d", transport.MaxConnsPerHost)
	}
}

func TestFetcher_do(t *testing.T) {
	// 1. 정상 통신 및 헤더 검증용 Mock 서버
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		// User-Agent 에코 모드
		w.Write([]byte(r.Header.Get("User-Agent")))
	}))
	defer ts.Close()

	fetcher := NewFetcher()

	t.Run("기본 User-Agent 반영 확인", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
		res, err := fetcher.do(req)
		if err != nil {
			t.Fatalf("기본 요청 중 예기치 않은 오류 발생: %v", err)
		}
		defer res.Body.Close()

		if req.Header.Get("User-Agent") != defaultUserAgent {
			t.Errorf("요청 객체에 기본 User-Agent가 주입되지 않았습니다. 현재값: %s", req.Header.Get("User-Agent"))
		}
	})

	t.Run("커스텀 User-Agent 유지 확인", func(t *testing.T) {
		customUA := "CustomBrowser/1.0"
		req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
		req.Header.Set("User-Agent", customUA)

		res, err := fetcher.do(req)
		if err != nil {
			t.Fatalf("커스텀 헤더 요청 중 예기치 않은 오류 발생: %v", err)
		}
		defer res.Body.Close()

		if req.Header.Get("User-Agent") != customUA {
			t.Errorf("커스텀 User-Agent가 덮어씌워졌습니다. 현재값: %s", req.Header.Get("User-Agent"))
		}
	})

	t.Run("네트워크 통신 오류 래핑 확인", func(t *testing.T) {
		// 명시적으로 타임아웃을 아주 짧게 제한하여 연결 실패를 유발합니다.
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://192.0.2.1:9999", nil) // 접속 불가 주소
		
		_, err := fetcher.do(req)
		if err == nil {
			t.Fatal("연결 불가능한 주소에 대한 요청이 성공해서는 안 됩니다.")
		}
		
		if !strings.Contains(err.Error(), "대상 서버와의 HTTP 통신을 수행할 수 없습니다") {
			t.Errorf("예상되는 에러 메시지 래핑이 누락되었습니다. 실제 반환된 에러: %v", err)
		}
	})
}

func TestFetcher_FetchBody(t *testing.T) {
	mockBody := "Hello CultureLecture Scraper!"
	
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/200" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockBody))
		} else if r.URL.Path == "/404" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
		}
	}))
	defer ts.Close()

	fetcher := NewFetcher()

	t.Run("정상적인 200 OK 응답 본문 파싱", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/200", nil)
		body, err := fetcher.FetchBody(req)
		
		if err != nil {
			t.Fatalf("정상 응답에서 오류가 발생했습니다: %v", err)
		}
		
		if string(body) != mockBody {
			t.Errorf("응답 데이터 불일치. 기대값: %s, 실제값: %s", mockBody, string(body))
		}
	})

	t.Run("비정상 상태 코드(404) 처리", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/404", nil)
		_, err := fetcher.FetchBody(req)
		
		if err == nil {
			t.Fatal("404 에러 시 FetchBody는 오류를 반환해야 합니다.")
		}
		
		expectedErrMsg := "대상 서버가 유효하지 않은 HTTP 응답 상태를 반환하였습니다"
		if !strings.Contains(err.Error(), expectedErrMsg) {
			t.Errorf("에러 메시지 형식이 일치하지 않습니다. 실제값: %v", err)
		}
	})
}

func TestFetcher_FetchJSON(t *testing.T) {
	type TestStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/valid" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"Jane Doe", "age":30}`))
		} else if r.URL.Path == "/invalid" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"Jane Doe", "age": "thirty-STRING"}`)) // 파싱 오류 유발 (int 필드에 string)
		}
	}))
	defer ts.Close()

	fetcher := NewFetcher()

	t.Run("정상 JSON 파싱 (Unmarshal)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/valid", nil)
		var target TestStruct
		
		err := fetcher.FetchJSON(req, &target)
		if err != nil {
			t.Fatalf("올바른 JSON 파싱 중 오류 발생: %v", err)
		}
		
		if target.Name != "Jane Doe" || target.Age != 30 {
			t.Errorf("JSON 파싱 결과 불일치. 파싱된 데이터: %+v", target)
		}
	})

	t.Run("잘못된 JSON 구조 (Unmarshal 에러)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/invalid", nil)
		var target TestStruct
		
		err := fetcher.FetchJSON(req, &target)
		if err == nil {
			t.Fatal("타입 변환이 불가능한 JSON 응답 시 에러를 반환해야 합니다.")
		}
		
		expectedErrMsg := "수신된 응답 데이터를 JSON 형식으로 파싱하는 과정에서 오류가 발생하였습니다"
		if !strings.Contains(err.Error(), expectedErrMsg) {
			t.Errorf("JSON 파싱 실패 시 기대한 에러 메시지가 포함되지 않았습니다. 실제값: %v", err)
		}
	})
}

func TestFetcher_FetchHTML(t *testing.T) {
	htmlContent := `<html><title>테스트</title><body><div id="target">문화센터 수집 로직</div></body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(htmlContent))
	}))
	defer ts.Close()

	fetcher := NewFetcher()

	t.Run("HTML 정상 goquery 변환 및 DOM 탐색", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
		
		doc, err := fetcher.FetchHTML(req)
		if err != nil {
			t.Fatalf("HTML 구문 파싱 중 오류 발생: %v", err)
		}

		// 반환된 doc 객체가 정상적인지 DOM 구조 탐색을 통해 확인합니다.
		text := doc.Find("#target").Text()
		if text != "문화센터 수집 로직" {
			t.Errorf("goquery를 통한 올바른 DOM 탐색에 실패했습니다. 추출된 텍스트: %s", text)
		}
	})
}
