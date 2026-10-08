package config

import (
	"crypto/tls"
	"errors"
	"net/url"
	"path/filepath"

	"isekai-mcp-gateway/internal/failure"
)

func (c *Config) validateTLS() error {
	settings := &c.Static.Server.TLS
	if !settings.Enabled {
		return nil
	}
	// 启用 TLS 时对外地址必须为 HTTPS。
	u, _ := url.Parse(c.Static.Server.PublicURL)
	if u.Scheme != "https" {
		return errors.New("server.publicUrl must use HTTPS when server.tls.enabled is true")
	}
	if settings.CertFile == "" || settings.KeyFile == "" {
		return errors.New("server.tls.certFile and keyFile are required for HTTPS")
	}
	// 相对路径按配置目录解析，便于随部署目录整体迁移。
	for _, path := range []*string{&settings.CertFile, &settings.KeyFile} {
		if !filepath.IsAbs(*path) {
			*path = filepath.Join(c.Dir, *path)
		}
	}
	if _, err := tls.LoadX509KeyPair(settings.CertFile, settings.KeyFile); err != nil {
		return &failure.Error{Code: "CONFIG_TLS_INVALID", Message: "server.tls: unable to load certificate and matching private key", Op: "config.tls", Field: "server.tls", Cause: err}
	}
	return nil
}
