package failure

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestCausesAndClassification(t *testing.T) {
	// 错误信息中的敏感内容不得出现在 Error() 文本里。
	secret := &mysql.MySQLError{Number: 1064, Message: "secret SELECT password FROM private_table"}
	err := Wrap("auth.read", secret)
	var mysqlError *mysql.MySQLError
	if !errors.Is(err, secret) || !errors.As(err, &mysqlError) || strings.Contains(err.Error(), "secret") {
		t.Fatal("cause was lost or exposed")
	}
	if f := Describe(err); f.Code != Internal || f.Retryable {
		t.Fatal("unknown errors must not opt into retries")
	}
	for _, cause := range []error{driver.ErrBadConn, context.DeadlineExceeded, &mysql.MySQLError{Number: 1213, Message: "secret"}} {
		err := Wrap("auth.lookup", Storage("store.get", cause))
		if Describe(err).Code != Unavailable || HTTPStatus(err) != 503 || !errors.Is(err, cause) {
			t.Fatalf("storage classification lost: %v", err)
		}
	}
	if err := Dependency("oidc.request", fmt.Errorf("wrapper: %w", context.DeadlineExceeded)); HTTPStatus(err) != 504 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("timeout cause lost")
	}
	if err := Wrap("operation", context.Canceled); Describe(err).Code != Canceled || Describe(err).Retryable {
		t.Fatal("canceled request marked retryable")
	}
	if !errors.Is(Wrap("auth", ErrInvalidToken), ErrInvalidToken) {
		t.Fatal("wrapped token error cannot be identified")
	}
}

func TestConcurrentBoundaryAndSafeLogging(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx := WithBoundary(NewRequest(context.Background()))
	err := Storage("store.read", &mysql.MySQLError{Number: 1213, Message: "secret-token SQL https://private/?password=x C:\\private\\file"})
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); Record(ctx, err); Log(ctx, "auth.validate", err) }()
	}
	workers.Wait()
	// 并发记录后，边界只保留首个故障，日志也只写一次。
	if BoundaryFailure(ctx) != err || strings.Count(output.String(), "operation failed") != 1 {
		t.Fatal("boundary or log deduplication failed")
	}
	if !strings.Contains(output.String(), `"mysql_number":1213`) || !strings.Contains(output.String(), RequestID(ctx)) {
		t.Fatal("safe metadata missing")
	}
	for _, forbidden := range []string{"secret-token", "SELECT password", "https://private", "password=x", "private\\\\file"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("unsafe log: %s", output.String())
		}
	}
	if RequestID(NewRequest(ctx)) == RequestID(ctx) {
		t.Fatal("invocation reused transport identity")
	}
}

func TestVerboseLoggingIncludesRawErrorText(t *testing.T) {
	for _, level := range []slog.Level{slog.Level(-8), slog.LevelDebug} {
		var output bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: level})))
		ctx := NewRequest(context.Background())
		Log(ctx, "startup.serve", Wrap("startup.serve", errors.New("dial tcp 127.0.0.1:3306: connection refused")))
		slog.SetDefault(previous)
		if !strings.Contains(output.String(), `"error_text":"dial tcp 127.0.0.1:3306`) {
			t.Fatalf("raw error text missing at %s: %s", level, output.String())
		}
	}
}

func TestBusinessFailureDoesNotHideLaterInternalFailure(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx := NewRequest(context.Background())
	Log(ctx, "auth.bearer", ErrInvalidToken)
	err := Wrap("auth.lookup", Storage("store.get.accounts", errors.New("secret SQL")))
	Log(ctx, "http.write", err)
	Log(ctx, "http.write", err)
	if strings.Count(output.String(), "operation failed") != 2 || !strings.Contains(output.String(), `"level":"ERROR"`) || !strings.Contains(output.String(), `"cause_operation":"store.get.accounts"`) {
		t.Fatalf("later failure was hidden or context lost: %s", output.String())
	}
}
