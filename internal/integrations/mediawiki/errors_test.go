package mediawiki

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
)

type testTransport func(*http.Request) (*http.Response, error)

func (t testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return t(r) }

type brokenBody struct{ cause error }

func (b brokenBody) Read([]byte) (int, error) { return 0, b.cause }
func (brokenBody) Close() error               { return nil }

func TestWikiRequestCausesAndResponseLimits(t *testing.T) {
	cause := errors.New("secret upstream trace and URL")

	// 覆盖请求错误/超时/状态码/格式错误/超限/读取失败等失败路径
	for _, test := range []struct {
		name         string
		status       int
		body         io.ReadCloser
		transportErr error
		code         string
		retryable    bool
	}{
		{"request", 0, nil, cause, failure.Upstream, false},
		{"timeout", 0, nil, context.DeadlineExceeded, failure.Timeout, true},
		{"status", 503, io.NopCloser(strings.NewReader("secret")), nil, failure.Upstream, true},
		{"malformed", 200, io.NopCloser(strings.NewReader("secret")), nil, failure.Upstream, false},
		{"large", 200, io.NopCloser(strings.NewReader(strings.Repeat(" ", 65537))), nil, failure.Upstream, false},
		{"read", 200, brokenBody{cause}, nil, failure.Upstream, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			// 用自定义 transport 按用例注入上游响应的状态码与响应体
			c := &Client{Config: config.MediaWiki{APIURL: "https://wiki.example/api.php"}, Secret: strings.Repeat("x", 32), HTTP: &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
				if test.transportErr != nil {
					return nil, test.transportErr
				}
				return &http.Response{StatusCode: test.status, Body: test.body, Header: make(http.Header)}, nil
			})}}
			c.Config.Bridge.KeyID = "test"
			_, err := c.Resolve(context.Background(), "subject")
			if err == nil {
				t.Fatal("accepted failed response")
			}
			f := failure.Describe(err)
			if f.Code != test.code || f.Retryable != test.retryable || strings.Contains(err.Error(), "secret") {
				t.Fatalf("invalid failure: %+v", f)
			}
			if test.transportErr != nil && !errors.Is(err, test.transportErr) {
				t.Fatal("lost request cause")
			}
			if test.name == "read" && !errors.Is(err, cause) {
				t.Fatal("lost body read cause")
			}
		})
	}
}
