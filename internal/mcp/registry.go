package gatewaymcp

import (
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Definition struct {
	Kind     string
	Name     string
	Key      string
	Register func(*mcp.Server)
}

func Register(server *mcp.Server, definitions []Definition) error {
	// 第一遍预检：在真正注册前发现冲突，避免注册到一半才失败。
	names, keys := map[string]bool{}, map[string]bool{}
	for _, d := range definitions {
		name := d.Kind + ":" + d.Name
		key := d.Kind + ":" + d.Key
		// 名称必填；同名或同 Key（Key 非空时）都视为冲突。
		if d.Name == "" || names[name] || d.Key != "" && keys[key] {
			return errors.New("duplicate or empty MCP definition")
		}
		names[name], keys[key] = true, true
	}

	// 预检通过后逐个注册，此时可安全执行各自的副作用。
	for _, d := range definitions {
		d.Register(server)
	}
	return nil
}
