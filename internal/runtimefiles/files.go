// Package runtimefiles assembles and validates runtime file services.
package runtimefiles

import (
	"path/filepath"

	"isekai-mcp-gateway/internal/assets"
	"isekai-mcp-gateway/internal/templates"
)

type Files struct {
	Root      string
	Templates *templates.Manager
	Assets    *assets.Service
}

// Load validates all runtime files before opening external services.
func Load(root string) (*Files, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	t, err := templates.New(abs)
	if err != nil {
		return nil, err
	}
	a, err := assets.New(abs)
	if err != nil {
		return nil, err
	}
	return &Files{Root: abs, Templates: t, Assets: a}, nil
}
