package web

import (
	"slices"

	"isekai-mcp-gateway/internal/auth/domain"
)

// basic is a consent form alias, never a protocol or persisted scope.

// consentScopeViews 将请求与允许的 scope 组装为同意页视图，基础 scope 合并为一项
func consentScopeViews(requested, allowed, defaults []string) []scopeView {
	var views []scopeView
	for _, scope := range domain.ConsentScopes(requested, allowed) {
		// 所有基础 scope 聚合为单个“基础信息”选项，去重后只展示一次
		if domain.IsBasicScope(scope) {
			if !slices.ContainsFunc(views, func(v scopeView) bool { return v.Name == "basic" }) {
				views = append(views, scopeView{
					Name:        "basic",
					Label:       "基础信息",
					Checked:     true,
					Force:       true,
					Description: "用于连接你的账号，并读取你的基本资料，例如显示名称和邮箱。选择保持登录后，应用可以在较长时间内继续使用，无需你反复登录。",
				})
			}
			continue
		}

		if scope == "mcp:access" {
			views = append(views, scopeView{
				Name:        scope,
				Label:       scopeLabel(scope),
				Description: scopeDescription(scope),
				Checked:     true,
				Force:       true,
			})
			continue
		}

		views = append(views, scopeView{
			Name:        scope,
			Label:       scopeLabel(scope),
			Description: scopeDescription(scope),
			Checked:     slices.Contains(defaults, scope),
		})
	}
	return views
}

// expandConsentScopes 将同意提交视为已同意本次请求的基础 scope；自定义权限取自表单。
func expandConsentScopes(selected, requested []string) []string {
	expanded, _ := domain.SplitScopes(requested)
	for _, scope := range selected {
		if scope != "basic" && !domain.IsBasicScope(scope) && !slices.Contains(expanded, scope) {
			expanded = append(expanded, scope)
		}
	}
	return expanded
}
