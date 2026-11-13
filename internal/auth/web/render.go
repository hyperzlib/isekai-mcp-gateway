package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/justinas/nosurf"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
)

type scopeView struct {
	Name, Label, Description string
	Checked, Force           bool
}

type entryView = domain.ManagementEntry

// apiKeyView 是 API Key 管理页的列表条目。
type apiKeyView struct {
	ID, Name, Scope, CreatedAt, ExpiresAt, Status string
}

// ttlView 是创建表单的有效期选项；Disabled 由 maxTtlSeconds 决定。
type ttlView struct {
	Name, Value string
	Days        int
	Disabled    bool
}

type pageData struct {
	SiteTitle, SiteIconURL                      string
	Title, Mode, CSRF, Message                  string
	Interaction, ClientName, Redirect, Username string
	ClientLogo                                  string
	LogoURL                                     *string
	TrustLevel                                  domain.TrustLevel
	Countdown                                   int
	Scopes                                      []scopeView
	Entries                                     []entryView
	// 登录态与导航：LoggedIn 控制头像/登出，AvatarURL 缺失时用首字母占位。
	LoggedIn, KeyEnabled                  bool
	AvatarURL, Initial, ReturnTo, Summary string
	// Wide 让管理页使用宽面板与顶部页签布局。
	Wide bool
	// RetryURL 非空时错误页渲染“重试”按钮（POST 回发起登录的端点）。
	RetryURL                  string
	APIKeys                   []apiKeyView
	CreatedKey, CreatedName   string
	AllowPermanent            bool
	MaxPerUser, MaxTTLSeconds int
	TTLOptions                []ttlView
}

// applySiteData 填充各页面共用的站点品牌信息，并返回登录页可放行的上游授权来源
func (s *Service) applySiteData(p *pageData) (authorizationOrigin string) {
	p.SiteTitle = config.DefaultSiteTitle
	if s.Service != nil && s.Upstream != nil {
		if bundle := s.Upstream.Get(); bundle != nil {
			if bundle.Config != nil {
				p.SiteTitle, p.SiteIconURL = bundle.Config.App.Site.Title, bundle.Config.App.Site.IconURL
				p.KeyEnabled = bundle.Config.Static.Auth.Gateway.APIKeys.Enabled
			}
			if bundle.OidcClient != nil {
				authorizationOrigin = bundle.OidcClient.AuthorizationOrigin()
			}
		}
	}
	if strings.TrimSpace(p.SiteTitle) == "" {
		p.SiteTitle = config.DefaultSiteTitle
	}
	return authorizationOrigin
}

// LoginState 探测浏览器会话，供布局导航与首页渲染登录/登出与账号信息。
func (s *Service) LoginState(w http.ResponseWriter, r *http.Request) (loggedIn bool, username, avatar string) {
	return s.loginState(w, r)
}

// loginState 探测浏览器会话，供布局导航渲染登录/登出与账号信息。
func (s *Service) loginState(w http.ResponseWriter, r *http.Request) (loggedIn bool, username, avatar string) {
	if s.Service == nil || s.Store == nil {
		return false, "", ""
	}
	session, err := s.browserSession(r, s.browser(w, r))
	if err != nil {
		return false, "", ""
	}
	username = session.Identity.DisplayName
	if username == "" {
		username = session.Identity.Username
	}
	return true, username, session.Identity.AvatarURL
}

// applyLoginState 注入导航所需的登录态；已登录但缺少头像时回退到配置的默认头像。
func (s *Service) applyLoginState(p *pageData, w http.ResponseWriter, r *http.Request) {
	loggedIn, username, avatar := s.loginState(w, r)
	if !loggedIn {
		// 探测失败时保留调用方显式提供的登录态（如账户页已登录视图）。
		if !p.LoggedIn {
			return
		}
	} else {
		p.LoggedIn, p.Username, p.AvatarURL = true, username, avatar
	}
	if p.Initial == "" && p.Username != "" {
		p.Initial = string([]rune(p.Username)[:1])
	}
	if p.AvatarURL == "" && s.Service != nil && s.Upstream != nil {
		if bundle := s.Upstream.Get(); bundle != nil && bundle.Config != nil {
			p.AvatarURL = bundle.Config.App.Auth.OidcClient.DefaultAvatar
		}
	}
}

// imageOrigins 汇总页面需要加载的远程图片来源，供 CSP img-src 白名单使用
func imageOrigins(p *pageData) string {
	images := "'self'"
	origins := map[string]bool{}
	addImageOrigin := func(raw string) {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return
		}
		origin := u.Scheme + "://" + u.Host
		if !origins[origin] {
			origins[origin] = true
			images += " " + origin
		}
	}
	addLogo := func(raw *string) {
		if raw == nil || *raw == "" || !domain.WebMetadataURL(*raw, true) {
			return
		}
		addImageOrigin(*raw)
	}
	if p.SiteIconURL != "" && config.ValidSiteIcon(p.SiteIconURL) && !strings.HasPrefix(p.SiteIconURL, "/") {
		addImageOrigin(p.SiteIconURL)
	}
	addLogo(p.LogoURL)
	// IDP 头像与远程默认头像按需放行；/assets/ 本地路径无需放行。
	if p.AvatarURL != "" && !strings.HasPrefix(p.AvatarURL, "/") {
		addImageOrigin(p.AvatarURL)
	}
	for _, entry := range p.Entries {
		addLogo(entry.LogoURL)
	}
	return images
}

func (s *Service) render(w http.ResponseWriter, r *http.Request, p pageData) {
	p.CSRF = nosurf.Token(r)
	if r.Method == http.MethodGet && (p.Mode == "grants" || p.Mode == "clients" || p.Mode == "apikeys") {
		if cookie, err := r.Cookie("gateway_removal_notice"); err == nil {
			if name, err := base64.RawURLEncoding.DecodeString(cookie.Value); err == nil && len(name) <= 512 && utf8.Valid(name) {
				p.Message = string(name)
			}
			http.SetCookie(w, &http.Cookie{Name: "gateway_removal_notice", Path: r.URL.Path, MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
		}
	}
	authorizationOrigin := s.applySiteData(&p)
	// 探测浏览器登录态，供布局导航展示头像与登出入口
	s.applyLoginState(&p, w, r)
	p.ReturnTo = ""
	if p.Mode == "login" && p.Interaction == "" && r.URL.Query().Get("backto") == "home" {
		p.ReturnTo = "/"
	}
	// Ordinary form POSTs serialize Origin as null under no-referrer.
	// same-origin preserves CSRF origin checks without disclosing OAuth URLs
	// to the upstream identity provider or the downstream client.
	w.Header().Set("Referrer-Policy", "same-origin")
	// 按页面用途逐步放开 form-action 白名单，默认仅允许自身
	sources := "'self'"
	// 登录表单与未登录账户页会把浏览器重定向到上游授权端点
	if p.Mode == "login" || (p.Mode == "account" && !p.LoggedIn) {
		origin := authorizationOrigin
		if origin != "" {
			sources += " " + origin
		}
	}
	// Consent and cancellation may redirect through Gateway to the validated
	// client callback. Browsers can enforce form-action across that chain.
	if destination := formDestination(p.Redirect); destination != "" {
		sources += " " + destination
	}
	w.Header().Set("Content-Security-Policy", "img-src "+imageOrigins(&p)+"; default-src 'self'; frame-ancestors 'none'; form-action "+sources+"; base-uri 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	var body bytes.Buffer
	if err := s.Pages.RenderHTML(&body, p.Mode, p); err != nil {
		failure.Log(r.Context(), "page.render", failure.Wrap("page.render", err))
		http.Error(w, "Unable to render page", http.StatusInternalServerError)
		return
	}
	if _, err := body.WriteTo(w); err != nil {
		failure.Log(r.Context(), "page.write", failure.Wrap("page.write", err))
	}
}

// Redirect is supplied only from an already validated authorization request.
// formDestination 从回调地址中提取可安全放入 CSP form-action 的来源
func formDestination(raw string) string {
	// 拒绝含用户信息、片段或空白/引号等可注入字符的地址
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || strings.ContainsAny(raw, "'\"; \t\r\n") {
		return ""
	}
	// Web 回调取 scheme 与 host，自定义 scheme 取 scheme 前缀
	if (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	if u.IsAbs() && domain.PrivateApplicationScheme(u.Scheme) {
		return u.Scheme + ":"
	}
	return ""
}

func (s *Service) pageError(w http.ResponseWriter, message string, status int) {
	s.pageErrorContext(context.Background(), w, message, status)
}

func (s *Service) pageErrorContext(ctx context.Context, w http.ResponseWriter, message string, status int) {
	s.pageErrorRetry(nil, w, message, status, "")
}

// pageErrorRetry 渲染错误页；retry 非空时附带“重试”按钮（POST 回发起登录的端点）。
func (s *Service) pageErrorRetry(r *http.Request, w http.ResponseWriter, message string, status int, retry string) {
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// 错误页同样展示站点品牌，避免整块品牌信息缺失
	p := pageData{Title: "操作未完成", Mode: "error", Message: message, RetryURL: retry}
	if r != nil && retry != "" {
		p.CSRF = nosurf.Token(r)
	}
	s.applySiteData(&p)
	// 错误页的重试表单提交到本站端点，CSP 仅需放行品牌图标
	w.Header().Set("Content-Security-Policy", "img-src "+imageOrigins(&p)+"; default-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	var body bytes.Buffer
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	if err := s.Pages.RenderHTML(&body, "error", p); err != nil {
		op := "page.render_error"
		var id string
		if r != nil {
			id = w.Header().Get("X-Request-ID")
		}
		failure.Log(ctx, op, failure.Wrap(op, err), "response_request_id", id)
		http.Error(w, "Unable to render page", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(status)
	if _, err := body.WriteTo(w); err != nil {
		op := "page.write"
		var id string
		if r != nil {
			id = w.Header().Get("X-Request-ID")
		}
		failure.Log(ctx, op, failure.Wrap(op, err), "response_request_id", id)
	}
}

func scopeLabel(scope string) string {
	labels := map[string]string{
		"mcp:access":     "连接到本服务",
		"wiki:read":      "查看 Wiki 内容",
		"wiki:edit":      "编辑页面",
		"wiki:create":    "新建页面",
		"wiki:upload":    "上传文件",
		"wiki:move":      "重命名或移动页面",
		"wiki:delete":    "删除页面",
		"offline_access": "保持登录状态",
		"openid":         "读取账号标识",
		"profile":        "读取显示名称",
		"email":          "读取邮箱",
	}
	if label := labels[scope]; label != "" {
		return label
	}
	return scope
}

// scopeSummary turns stored scope names into text ordinary users can read.
func scopeSummary(raw string) string {
	names := strings.Fields(raw)
	labels := make([]string, 0, len(names))
	for _, name := range names {
		labels = append(labels, scopeLabel(name))
	}
	return strings.Join(labels, "、")
}

func scopeDescription(scope string) string {
	descriptions := map[string]string{
		"mcp:access":     "连接本服务所需的基础权限。",
		"wiki:read":      "查看 Wiki 页面内容。",
		"wiki:create":    "新建 Wiki 页面。",
		"wiki:edit":      "修改已有 Wiki 页面。",
		"wiki:move":      "重命名或移动 Wiki 页面。",
		"wiki:delete":    "删除 Wiki 页面。",
		"wiki:upload":    "上传文件到 Wiki。",
		"offline_access": "在你保持登录期间维持连接。",
		"openid":         "读取你的账号标识。",
		"profile":        "读取你的显示名称等资料。",
		"email":          "读取你的邮箱地址。",
	}
	return descriptions[scope]
}
