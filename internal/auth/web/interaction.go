package web

import (
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"isekai-mcp-gateway/internal/auth/application"
)

func (s *Service) Interaction(w http.ResponseWriter, r *http.Request) {
	// 交互事务必须属于当前浏览器且仍处于可处理状态
	id, browser := chi.URLParam(r, "uid"), s.browser(w, r)
	a, row, err := s.InteractionInfo(r.Context(), id)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	if a.Browser != browser {
		s.flowError(w, r, &application.FlowError{Code: "INVALID_INTERACTION", Message: "本次授权不属于当前浏览器会话，请在发起连接的浏览器中继续，或返回应用重新发起连接。", Status: 400})
		return
	}
	if row.State == "consumed" || row.State == "code" || row.State == "consented" {
		s.flowError(w, r, &application.FlowError{Code: "INVALID_INTERACTION", Message: "本次授权已完成，请返回应用查看连接结果。如需再次授权，请从应用重新发起连接。", Status: 400})
		return
	}

	// 处理表单动作：取消授权、或在登录态发起登录
	if r.Method == "POST" {
		if err = r.ParseForm(); err != nil {
			s.flowError(w, r, &application.FlowError{Code: "INVALID_REQUEST", Message: "提交的内容无效，请重试。", Status: 400, Cause: err})
			return
		}
		if r.PostForm.Get("action") == "cancel" {
			err = s.CancelInteraction(r.Context(), id, browser)
			if err != nil {
				s.flowError(w, r, err)
				return
			}
			s.AuthError(w, r, a.Request.RedirectURI, a.Request.State, "access_denied", "user cancelled authorization")
			return
		}
		if r.PostForm.Get("action") == "login" && row.State == "login" {
			s.beginLogin(w, r, id, browser)
			return
		}
	}
	c, err := s.ClientInfo(r.Context(), a.Request.ClientID)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	// 客户端 logo 仅在非空时展示，否则模板回退到默认 MCP Logo。
	clientLogo := ""
	if c.LogoURL != nil {
		clientLogo = strings.TrimSpace(*c.LogoURL)
	}
	view := pageData{Title: "应用请求连接", Interaction: id, ClientName: c.Name, ClientLogo: clientLogo, LogoURL: c.LogoURL, TrustLevel: c.TrustLevel, Redirect: a.Request.RedirectURI}
	// 未绑定账号时先渲染登录页
	if a.AccountID == "" {
		view.Mode = "login"
		s.render(w, r, view)
		return
	}
	session, err := s.LoadBrowserSession(r.Context(), a.SessionID, browser)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	view.Username = session.Identity.DisplayName
	if view.Username == "" {
		view.Username = session.Identity.Username
	}
	trusted, err := s.IsTrusted(r.Context(), a.AccountID, c)
	if err != nil {
		s.flowError(w, r, err)
		return
	}

	// 未信任客户端时展示信任确认，并驱动倒计时
	if !trusted {
		if r.Method == "POST" && r.PostForm.Get("action") == "trust" {
			if err = s.ConfirmTrust(r.Context(), a, c); err != nil {
				s.flowError(w, r, err)
				return
			}
			http.Redirect(w, r, r.URL.Path, 303)
			return
		}
		if a.TrustAfter.IsZero() {
			err = s.BeginTrustCountdown(r.Context(), a, c)
			if err != nil {
				s.flowError(w, r, err)
				return
			}
		}
		view.Mode = "trust"
		view.Countdown = int(math.Max(0, math.Ceil(time.Until(a.TrustAfter).Seconds())))
		s.render(w, r, view)
		return
	}

	// 已信任时处理同意提交，未提交则渲染同意页
	if r.Method == "POST" && r.PostForm.Get("action") == "consent" {
		if err = s.Consent(r.Context(), a, c, expandConsentScopes(r.PostForm["scope"], a.Request.Scopes)); err != nil {
			s.flowError(w, r, err)
			return
		}
		http.Redirect(w, r, "/oauth/authorize/callback?id="+url.QueryEscape(id), 303)
		return
	}
	view.Mode = "consent"
	view.Scopes = consentScopeViews(a.Request.Scopes, application.BusinessScopes(), s.Upstream.Get().Config.App.Auth.Consent.DefaultSelectedScopes)
	s.render(w, r, view)
}
