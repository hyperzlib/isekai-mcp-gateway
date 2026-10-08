package domain

import (
	"slices"
	"strings"
)

// BasicScopes returns a copy so callers cannot mutate the shared classification.
func BasicScopes() []string          { return []string{"openid", "offline_access", "profile", "email"} }
func IsBasicScope(scope string) bool { return slices.Contains(BasicScopes(), scope) }

// BusinessScopes returns the fixed business permissions this gateway supports.
func BusinessScopes() []string {
	return []string{"mcp:access", "wiki:read", "wiki:edit", "wiki:create", "wiki:upload", "wiki:move", "wiki:delete"}
}
func SplitScopes(scopes []string) (basic, custom []string) {
	// 将 scope 分为基础权限与自定义权限两组，并去重。
	for _, scope := range scopes {
		if IsBasicScope(scope) {
			if !slices.Contains(basic, scope) {
				basic = append(basic, scope)
			}
		} else if !slices.Contains(custom, scope) {
			custom = append(custom, scope)
		}
	}
	return
}

// ConsentScopes combines requested basic permissions with all configured custom permissions.
func ConsentScopes(requested, allowed []string) []string {
	basic, _ := SplitScopes(requested)
	_, custom := SplitScopes(allowed)
	return append(basic, custom...)
}
func RequestedScopesAllowed(requested, registered []string) bool {
	basic, _ := SplitScopes(requested)
	return Subset(basic, registered)
}

// RefreshScopes changes only the basic subset; custom grants are inherited verbatim.
func RefreshScopes(previous, requested []string) ([]string, error) {
	basic, _ := SplitScopes(requested)
	oldBasic, custom := SplitScopes(previous)
	if !Subset(basic, oldBasic) {
		return nil, ErrInvalidScope()
	}
	return append(basic, custom...), nil
}

func NormalizeScopes(raw []string, allowed []string) ([]string, error) {
	var result []string
	add := func(s string) {
		if !slices.Contains(result, s) {
			result = append(result, s)
		}
	}

	// 逐个校验入参 scope 是否被允许，并去重累积。
	for _, s := range raw {
		if !slices.Contains(allowed, s) && !IsBasicScope(s) {
			return nil, ErrInvalidScope()
		}
		add(s)
	}

	// mcp:access 为必需的基础权限。
	add("mcp:access")
	// wiki:create 隐含 wiki:edit；任何非只读的 wiki 权限都隐含 wiki:read。
	if slices.Contains(result, "wiki:create") {
		add("wiki:edit")
	}
	for _, s := range result {
		if strings.HasPrefix(s, "wiki:") && s != "wiki:read" {
			add("wiki:read")
		}
	}

	// 展开隐含权限后重新校验，确保未引入越权范围。
	for _, s := range result {
		if !slices.Contains(allowed, s) && !IsBasicScope(s) {
			return nil, ErrInvalidScope()
		}
	}
	return result, nil
}

func Subset(a, b []string) bool {
	for _, s := range a {
		if !slices.Contains(b, s) {
			return false
		}
	}
	return true
}
