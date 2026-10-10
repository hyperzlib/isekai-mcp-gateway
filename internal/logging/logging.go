// Package logging routes records to console streams and optional JSON Lines files.
package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"isekai-mcp-gateway/internal/config"
)

type handler struct {
	normal, failures         slog.Handler
	fileNormal, fileFailures slog.Handler
}

func (h *handler) target(level slog.Level) (slog.Handler, slog.Handler) {
	if level >= slog.LevelWarn {
		return h.failures, h.fileFailures
	}
	return h.normal, h.fileNormal
}
func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	a, b := h.target(level)
	return a.Enabled(ctx, level) || (b != nil && b.Enabled(ctx, level))
}
func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	a, b := h.target(r.Level)
	var err error
	if a.Enabled(ctx, r.Level) {
		err = a.Handle(ctx, r)
	}
	if b != nil && b.Enabled(ctx, r.Level) {
		if fileErr := b.Handle(ctx, r); fileErr != nil {
			// slog callers ignore handler errors; report file failures on the console.
			fallback := slog.NewRecord(r.Time, slog.LevelError, "log file write failed", 0)
			fallback.Add("error", fileErr.Error())
			_ = h.failures.Handle(ctx, fallback)
			err = errors.Join(err, fileErr)
		}
	}
	return err
}
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.normal, c.failures = h.normal.WithAttrs(attrs), h.failures.WithAttrs(attrs)
	if h.fileNormal != nil {
		c.fileNormal = h.fileNormal.WithAttrs(attrs)
		c.fileFailures = h.fileFailures.WithAttrs(attrs)
	}
	return &c
}
func (h *handler) WithGroup(name string) slog.Handler {
	c := *h
	c.normal, c.failures = h.normal.WithGroup(name), h.failures.WithGroup(name)
	if h.fileNormal != nil {
		c.fileNormal = h.fileNormal.WithGroup(name)
		c.fileFailures = h.fileFailures.WithGroup(name)
	}
	return &c
}

// New uses the process working directory for relative file paths.
func New(cfg config.Logging, level slog.Leveler, stdout, stderr io.Writer) (*slog.Logger, func() error, error) {
	opts := &slog.HandlerOptions{Level: level}
	console := func(w io.Writer) slog.Handler {
		if cfg.ConsoleFormat == "json" {
			return slog.NewJSONHandler(w, opts)
		}
		return slog.NewTextHandler(w, opts)
	}
	h := &handler{normal: console(stdout), failures: console(stderr)}
	var files []*os.File
	closeFiles := func() error {
		var err error
		for _, f := range files {
			err = errors.Join(err, f.Close())
		}
		return err
	}
	open := func(path string) (slog.Handler, error) {
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
		return slog.NewJSONHandler(f, opts), nil
	}
	if cfg.File.Enabled {
		if cfg.File.Split {
			a, err := filepath.Abs(cfg.File.Path)
			if err != nil {
				return nil, closeFiles, err
			}
			b, err := filepath.Abs(cfg.File.ErrorPath)
			if err != nil {
				return nil, closeFiles, err
			}
			samePath := a == b || (runtime.GOOS == "windows" && strings.EqualFold(a, b))
			if first, err := os.Stat(a); err == nil {
				if second, err := os.Stat(b); err == nil {
					samePath = samePath || os.SameFile(first, second)
				}
			}
			if samePath {
				return nil, closeFiles, errors.New("split log files must use different paths")
			}
		}
		var err error
		h.fileNormal, err = open(cfg.File.Path)
		if err == nil {
			h.fileFailures = h.fileNormal
			if cfg.File.Split {
				h.fileFailures, err = open(cfg.File.ErrorPath)
			}
		}
		if err != nil {
			_ = closeFiles()
			return nil, func() error { return nil }, err
		}
	}
	return slog.New(h), closeFiles, nil
}
