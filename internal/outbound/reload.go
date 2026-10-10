package outbound

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
)

// Watch polls portable file contents, handling editors that atomically replace
// app.json. Failed candidates never change the published snapshot or bundles.
func (m *Manager) Watch(ctx context.Context, c *config.Config, level *slog.LevelVar) {
	path := filepath.Join(c.Dir, "app.json")
	last, _ := os.ReadFile(path)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var candidate []byte
	readFailed := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 同一轮读取失败只记录一次，避免刷屏。
			b, err := os.ReadFile(path)
			if err != nil {
				if !readFailed {
					failure.Log(ctx, "config.read", failure.Wrap("config.read", err))
					readFailed = true
				}
				continue
			}
			readFailed = false
			if bytes.Equal(b, last) {
				candidate = nil
				continue
			}
			if !bytes.Equal(b, candidate) {
				candidate = b
				continue
			}

			// Two stable observations debounce a partial write.
			last, candidate = b, nil
			cfg, err := config.Load(c.Dir, &c.Static)
			if err != nil {
				failure.Log(ctx, "config.reload", failure.Wrap("config.reload", err))
				continue
			}
			// 密钥类变更影响已颁发凭据，必须重启而非热重载。
			if !bytes.Equal(cfg.Secrets.Cookie, c.Secrets.Cookie) || !bytes.Equal(cfg.Secrets.Storage, c.Secrets.Storage) || cfg.Secrets.Signing != c.Secrets.Signing || cfg.Secrets.DBPassword != c.Secrets.DBPassword {
				slog.Warn("static environment changes require restart; previous snapshot retained")
				continue
			}
			// Bindings must retain the same Wiki site while existing Grants live.
			if cfg.App.MediaWiki.SiteID != m.Get().Config.App.MediaWiki.SiteID {
				slog.Warn("siteId change requires restart; previous snapshot retained")
				continue
			}
			// 构建新 bundle 设 30 秒上限，失败时保留旧快照。
			currentLogging := m.Get().Config.App.Logging
			if cfg.App.Logging.ConsoleFormat != currentLogging.ConsoleFormat || cfg.App.Logging.File != currentLogging.File {
				slog.Warn("logging output changes require restart; previous snapshot retained")
				continue
			}
			reloadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			bundle, err := Build(reloadCtx, cfg)
			cancel()
			if err != nil {
				failure.Log(ctx, "outbound.reload", failure.Wrap("outbound.reload", err))
				continue
			}
			m.Publish(bundle)
			SetLogLevel(level, cfg.App.Logging.Level)
			slog.Info("app configuration reloaded")
		}
	}
}

// SetLogLevel 将配置中的级别名映射到 slog 级别，未知值回退为 info。
func SetLogLevel(level *slog.LevelVar, name string) {
	switch name {
	case "trace":
		level.Set(slog.Level(-8))
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}
}
