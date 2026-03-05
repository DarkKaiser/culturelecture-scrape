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
	DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/101.0.4951.67 Safari/537.36"
	DefaultTimeout   = 30 * time.Second
)

// Client는 스크래퍼에서 공통으로 사용하는 HTTP 클라이언트 기능을 제공합니다.
type Client struct {
	httpClient *http.Client
}

// NewClient는 기본 설정이 적용된 새로운 Client를 생성합니다.
func NewClient() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 50
	transport.MaxConnsPerHost = 50
	transport.IdleConnTimeout = 90 * time.Second

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   DefaultTimeout,
		},
	}
}

// DoRequest는 공통 헤더를 설정하고 HTTP 요청을 수행합니다.
func (c *Client) DoRequest(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP 요청 실패: %w", err)
	}

	return res, nil
}

// FetchBody는 요청을 수행하고 바디를 읽어 반환합니다.
func (c *Client) FetchBody(req *http.Request) ([]byte, error) {
	res, err := c.DoRequest(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// TCP 커넥션 재사용을 위해 남은 Body 데이터를 끝까지 비움
		_, _ = io.Copy(io.Discard, res.Body)
		return nil, fmt.Errorf("요청 실패 (상태 코드: %d)", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("바디 읽기 실패: %w", err)
	}

	return body, nil
}

// FetchJSON은 요청을 수행하고 응답 바디를 JSON으로 파싱합니다.
func (c *Client) FetchJSON(req *http.Request, v interface{}) error {
	body, err := c.FetchBody(req)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("JSON 파싱 실패: %w", err)
	}

	return nil
}

// FetchGoQuery는 요청을 수행하고 응답 바디를 goquery.Document로 파싱합니다.
func (c *Client) FetchGoQuery(req *http.Request) (*goquery.Document, error) {
	bodyBytes, err := c.FetchBody(req)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("goquery.NewDocumentFromReader 실패: %w", err)
	}

	return doc, nil
}
