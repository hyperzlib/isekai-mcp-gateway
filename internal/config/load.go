package config

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"isekai-mcp-gateway/internal/failure"
)

func Decode(path string, v any) error {
	// 依据文件名给出更具体的错误字段名。
	field := "configuration"
	switch filepath.Base(path) {
	case "app.json", "app.example.json":
		field = "app"
	case "server.json", "server.example.json":
		field = "server"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return &failure.Error{Code: "CONFIG_READ_FAILED", Message: "configuration file could not be read", Op: "config.read", Field: field, Cause: err}
	}
	// 严格解码：未知字段视为错误，避免拼写错误被静默忽略。
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		return &failure.Error{Code: "CONFIG_DECODE_FAILED", Message: "configuration JSON is invalid", Op: "config.decode", Field: field, Cause: err}
	}
	// 首个对象解码后必须立即到达 EOF，否则存在多余内容。
	if err = d.Decode(new(any)); err != io.EOF {
		return &failure.Error{Code: "CONFIG_DECODE_FAILED", Message: "configuration must contain exactly one JSON object", Op: "config.decode", Field: field, Cause: err}
	}
	return nil
}

// Reload reuses the original static configuration; changing server.json needs restart.
func Load(dir string, static *ServerFile) (*Config, error) {
	c := Defaults()
	c.Dir = dir
	if static == nil {
		if err := Decode(filepath.Join(dir, "server.json"), &c.Static); err != nil {
			return nil, err
		}
	} else {
		c.Static = *static
	}
	if err := Decode(filepath.Join(dir, "app.json"), &c.App); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}
