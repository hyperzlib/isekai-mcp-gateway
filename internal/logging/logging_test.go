package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/config"
)

func TestRoutingAndAppend(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(map[bool]string{false: "combined", true: "split"}[split], func(t *testing.T) {
			cfg := config.Defaults().App.Logging
			cfg.File.Enabled, cfg.File.Split = true, split
			cfg.File.Path = filepath.Join(t.TempDir(), "nested", "log.jsonl")
			cfg.File.ErrorPath = filepath.Join(filepath.Dir(cfg.File.Path), "error.jsonl")
			var out, stderr bytes.Buffer
			level := new(slog.LevelVar)
			for i := 0; i < 2; i++ {
				logger, closeLogs, err := New(cfg, level, &out, &stderr)
				if err != nil {
					t.Fatal(err)
				}
				logger = logger.With("request_id", "req_test").WithGroup("operation")
				logger.Info("normal", "value", 1)
				logger.Warn("warning")
				logger.Error("failure")
				logger.Debug("filtered")
				if err := closeLogs(); err != nil {
					t.Fatal(err)
				}
			}
			if strings.Count(out.String(), "level=INFO") != 2 || strings.Contains(out.String(), "warning") || strings.Contains(out.String(), "filtered") {
				t.Fatalf("stdout: %s", &out)
			}
			if strings.Count(stderr.String(), "level=WARN") != 2 || strings.Count(stderr.String(), "level=ERROR") != 2 || strings.Contains(stderr.String(), "normal") {
				t.Fatalf("stderr: %s", &stderr)
			}
			check := func(path string, count int, failureFile bool) {
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSpace(string(b)), "\n")
				if len(lines) != count {
					t.Fatalf("%s: got %d lines", path, len(lines))
				}
				for _, line := range lines {
					var entry map[string]any
					if err := json.Unmarshal([]byte(line), &entry); err != nil {
						t.Fatal(err)
					}
					if entry["request_id"] != "req_test" {
						t.Fatal(entry)
					}
					if split && (entry["level"] != "INFO") != failureFile {
						t.Fatal(entry)
					}
				}
			}
			if split {
				check(cfg.File.Path, 2, false)
				check(cfg.File.ErrorPath, 4, true)
			} else {
				check(cfg.File.Path, 6, false)
			}
		})
	}
}

func TestOpenFailureAndDynamicLevel(t *testing.T) {
	cfg := config.Defaults().App.Logging
	var out, stderr bytes.Buffer
	level := new(slog.LevelVar)
	logger, closeLogs, err := New(cfg, level, &out, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer closeLogs()
	logger.Debug("hidden")
	level.Set(slog.LevelDebug)
	logger.Debug("visible")
	if strings.Contains(out.String(), "hidden") || !strings.Contains(out.String(), "visible") {
		t.Fatal(out.String())
	}
	cfg.File.Enabled = true
	cfg.File.Path = t.TempDir()
	if _, _, err := New(cfg, level, &out, &stderr); err == nil {
		t.Fatal("expected directory open failure")
	}
}
