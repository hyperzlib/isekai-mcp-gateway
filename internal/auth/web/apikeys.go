package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
)

// APIKeys 渲染 API Key 管理页：列表 + 创建表单 + 一次性明文 key 展示。
func (s *Service) APIKeys(w http.ResponseWriter, r *http.Request) {
	// 功能未启用时整个入口按 404 处理。
	if !s.APIKeysEnabled() {
		s.pageErrorContext(r.Context(), w, "API Key 功能未启用。", 404)
		return
	}
	session, err := s.browserSession(r, s.browser(w, r))
	if err != nil {
		if !errors.Is(err, application.ErrNoSession) {
			s.flowError(w, r, err)
			return
		}
		http.Redirect(w, r, "/auth/login", 303)
		return
	}
	if r.Method == "POST" {
		if err = r.ParseForm(); err != nil {
			s.pageErrorContext(r.Context(), w, "请求无效，请重试。", 400)
			return
		}
		switch r.PostForm.Get("action") {
		case "create":
			s.createAPIKey(w, r, session)
		case "delete":
			if err = s.DeleteAPIKey(r.Context(), session.AccountID, r.PostForm.Get("id")); err != nil {
				s.flowError(w, r, err)
				return
			}
			redirectAfterRemoval(w, r)
		default:
			s.pageErrorContext(r.Context(), w, "请求无效，请重试。", 400)
		}
		return
	}
	s.renderAPIKeys(w, r, session, pageData{})
}

func (s *Service) renderAPIKeys(w http.ResponseWriter, r *http.Request, session *application.Session, p pageData) {
	p.Mode, p.Title, p.Wide = "apikeys", "API Keys", true
	p.MaxPerUser = s.Config.Static.Auth.Gateway.APIKeys.MaxPerUser
	p.MaxTTLSeconds = s.Config.Static.Auth.Gateway.APIKeys.MaxTTLSeconds
	p.AllowPermanent = s.Config.Static.Auth.Gateway.APIKeys.AllowPermanent
	keys, rows, err := s.ListAPIKeys(r.Context(), session.AccountID)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	p.APIKeys = []apiKeyView{}
	for i, key := range keys {
		keyPreview := "sk-" + key.ID[:6]
		p.APIKeys = append(p.APIKeys, apiKeyView{
			ID: rows[i].ID, Name: key.Name, Scope: keyPreview + "... · " + scopeSummary(strings.Join(key.Scopes, " ")),
			CreatedAt: rows[i].UpdatedAt.UTC().Format("2006-01-02 15:04"),
			ExpiresAt: expiryText(rows[i].ExpiresAt), Status: rows[i].State,
		})
	}
	// scope 勾选复用同意页视图：mcp:access 恒含且强制。
	p.Scopes = consentScopeViews(domain.BusinessScopes(), domain.BusinessScopes(), s.defaultKeyScopes())
	// 有效期选项按 maxTtlSeconds 与 allowPermanent 裁剪。
	p.TTLOptions = s.ttlOptions()
	s.render(w, r, p)
}

func (s *Service) defaultKeyScopes() []string {
	bundle := s.Upstream.Get()
	if bundle == nil || bundle.Config == nil {
		return nil
	}
	var defaults []string
	for _, scope := range bundle.Config.App.Auth.Consent.DefaultSelectedScopes {
		if !domain.IsBasicScope(scope) {
			defaults = append(defaults, scope)
		}
	}
	return defaults
}

func (s *Service) ttlOptions() []ttlView {
	maxDays := s.Config.Static.Auth.Gateway.APIKeys.MaxTTLSeconds / 86400
	options := []ttlView{{Name: "7 天", Value: "7", Days: 7}, {Name: "30 天", Value: "30", Days: 30}, {Name: "90 天", Value: "90", Days: 90}, {Name: "365 天", Value: "365", Days: 365}}
	for i := range options {
		options[i].Disabled = options[i].Days > maxDays
	}
	return options
}

// createAPIKey 处理创建表单并渲染一次性明文 key 结果页。
func (s *Service) createAPIKey(w http.ResponseWriter, r *http.Request, session *application.Session) {
	name := r.PostForm.Get("name")
	scopes := expandConsentScopes(r.PostForm["scope"], domain.BusinessScopes())
	expires, err := s.parseExpiry(r)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	plaintext, _, err := s.CreateAPIKey(r.Context(), session, name, scopes, expires)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	// 明文 key 只在本响应中出现一次，落库仅存哈希。
	s.renderAPIKeys(w, r, session, pageData{CreatedKey: plaintext, CreatedName: name})
}

// parseExpiry 把表单中的有效期选择换算为过期时间。
func (s *Service) parseExpiry(r *http.Request) (time.Time, error) {
	now := time.Now().UTC()
	switch r.PostForm.Get("expiry") {
	case "permanent":
		if !s.Config.Static.Auth.Gateway.APIKeys.AllowPermanent {
			return time.Time{}, &application.FlowError{Code: "INVALID_REQUEST", Message: "不允许创建永久 API Key。", Status: 400}
		}
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), nil
	case "custom":
		date, err := time.ParseInLocation("2006-01-02", r.PostForm.Get("customDate"), time.UTC)
		if err != nil {
			return time.Time{}, &application.FlowError{Code: "INVALID_REQUEST", Message: "自定义日期无效。", Status: 400}
		}
		expires := date.AddDate(0, 0, 1)
		if !expires.After(now) || int64(expires.Sub(now).Seconds()) > int64(s.Config.Static.Auth.Gateway.APIKeys.MaxTTLSeconds) {
			return time.Time{}, &application.FlowError{Code: "INVALID_REQUEST", Message: "自定义有效期超出允许范围。", Status: 400}
		}
		return expires, nil
	}
	// 预设天数选项统一走天数换算，未知取值直接拒绝。
	value := r.PostForm.Get("expiry")
	n := 0
	for _, c := range value {
		if c < '0' || c > '9' {
			return time.Time{}, &application.FlowError{Code: "INVALID_REQUEST", Message: "API Key 有效期无效。", Status: 400}
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 || n > s.Config.Static.Auth.Gateway.APIKeys.MaxTTLSeconds/86400 {
		return time.Time{}, &application.FlowError{Code: "INVALID_REQUEST", Message: "API Key 有效期无效。", Status: 400}
	}
	return now.Add(time.Duration(n) * 24 * time.Hour), nil
}

func expiryText(t time.Time) string {
	if t.Year() >= 9999 {
		return "永久"
	}
	return t.UTC().Format("2006-01-02 15:04")
}
