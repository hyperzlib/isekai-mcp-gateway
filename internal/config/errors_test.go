package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/failure"
)

func TestConfigurationErrorsPreserveSafeContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(path, []byte(`{"private-secret-key": "secret-value"}`), 0600); err != nil {
		t.Fatal(err)
	}
	// 未知字段必须报错，但不能把字段名或值泄漏到错误文本中。
	err := Decode(path, new(AppFile))
	if err == nil || failure.Describe(err).Field != "app" || strings.Contains(err.Error(), "secret") {
		t.Fatal("unknown field was exposed or context lost")
	}
	if err := os.WriteFile(path, []byte(`{"secret":`), 0600); err != nil {
		t.Fatal(err)
	}
	// 解析错误需保留可展开的底层 cause，同时避免泄漏内容。
	err = Decode(path, new(AppFile))
	if err == nil || strings.Contains(err.Error(), "secret") || errors.Unwrap(err) == nil {
		t.Fatal("parse cause was dropped or exposed")
	}
	// A syntax error from a complete invalid token retains typed location data.
	if err := os.WriteFile(path, []byte(`{"configVersion": ?}`), 0600); err != nil {
		t.Fatal(err)
	}
	err = Decode(path, new(AppFile))
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) || syntax.Offset == 0 {
		t.Fatal("JSON syntax location was lost")
	}
}
