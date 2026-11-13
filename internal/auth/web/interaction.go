package web

import (
	"context"
	"errors"
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

	// 处理表单动作：取消授权、发起登录、沿用/绑定登录态、切换账号
	if r.Method == "POST" {
		if err = r.ParseForm(); err != nil {
			s.flowError(w, r, &application.FlowError{Code: "INVALID_REQUEST", Message: "提交的内容无效，请重试。", Status: 400, Cause: err})
			return
		}
		action := r.PostForm.Get("action")
		switch {
		case action == "cancel":
			if err = s.CancelInteraction(r.Context(), id, browser); err != nil {
				s.flowError(w, r, err)
				return
			}
			s.AuthError(w, r, a.Request.RedirectURI, a.Request.State, "access_denied", "user cancelled authorization")
			return
		case action == "switch" || action == "switch_account":
			// 切换账号：退回 login 状态并清除浏览器登录态，自刷新到未登录视图
			if err = s.ResetInteraction(r.Context(), id, browser); err != nil {
				s.flowError(w, r, err)
				return
			}
			s.cookie(w, "gateway_session", "", -1)
			http.Redirect(w, r, r.URL.Path, 303)
			return
		case row.State == "login" && action == "login":
			s.beginLogin(w, r, id, browser)
			return
		case row.State == "login" && action == "change":
			// 更改授权：仅绑定会话，自刷新进入完整 consent 页
			s.attachAndRefresh(w, r, id, browser)
			return
		case row.State == "login" && action == "continue":
			s.continueInteraction(w, r, id, browser)
			return
		case row.State == "authenticated" && action == "trust":
			s.confirmTrust(w, r, id, browser)
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

	// login 阶段渲染统一账户页：无会话显示登录入口；有会话按客户端信任
	// 与可沿用授权分流到账户页或直接进入 consent。
	if row.State == "login" {
		session, sessErr := s.browserSession(r, browser)
		if sessErr != nil {
			if !errors.Is(sessErr, application.ErrNoSession) {
				s.flowError(w, r, sessErr)
				return
			}
			view.Mode = "account"
			s.render(w, r, view)
			return
		}
		trusted, err := s.IsTrusted(r.Context(), session.AccountID, c)
		if err != nil {
			s.flowError(w, r, err)
			return
		}
		if trusted {
			// 可沿用授权时渲染账户页提供“继续/更改授权”，否则绑定会话
			// 后自刷新，由状态机直接渲染 consent 页。
			g, err := s.ReusableGrant(r.Context(), session, c, a.Resource, a.Request.Scopes)
			if err != nil && !errors.Is(err, application.ErrNoReusableGrant) {
				s.flowError(w, r, err)
				return
			}
			if g != nil {
				view.Mode, view.Summary = "account", scopeSummary(strings.Join(g.Scopes, " "))
				s.render(w, r, view)
				return
			}
			s.attachAndRefresh(w, r, id, browser)
			return
		}
		// 不受信客户端：先展示账户页，由“继续”进入 trust 确认。
		view.Mode = "account"
		s.render(w, r, view)
		return
	}

	// 已绑定账号时加载会话；trust/consent 渲染与提交保持原有状态机。
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

	// 已信任时处理同意提交，未提交则渲染同意页（预勾选上次授权 scopes）。
	if r.Method == "POST" && r.PostForm.Get("action") == "consent" {
		if err = s.Consent(r.Context(), a, c, expandConsentScopes(r.PostForm["scope"], a.Request.Scopes)); err != nil {
			s.flowError(w, r, err)
			return
		}
		http.Redirect(w, r, "/oauth/authorize/callback?id="+url.QueryEscape(id), 303)
		return
	}
	view.Mode = "consent"
	view.Scopes = consentScopeViews(a.Request.Scopes, application.BusinessScopes(), s.previousOrDefault(r.Context(), session, c, a.Resource))
	s.render(w, r, view)
}

// previousOrDefault 返回上次授权 scopes 作为同意页预勾选，缺失时回退配置默认。
func (s *Service) previousOrDefault(ctx context.Context, session *application.Session, c *application.Client, resource string) []string {
	if previous := s.LastGrantScopes(ctx, session.AccountID, c, resource); previous != nil {
		return previous
	}
	if bundle := s.Upstream.Get(); bundle != nil && bundle.Config != nil {
		return bundle.Config.App.Auth.Consent.DefaultSelectedScopes
	}
	return nil
}

// attachAndRefresh 把当前会话绑定到交互后自刷新，交回 GET 状态机渲染。
func (s *Service) attachAndRefresh(w http.ResponseWriter, r *http.Request, id, browser string) {
	session, err := s.browserSession(r, browser)
	if err != nil {
		if errors.Is(err, application.ErrNoSession) {
			http.Redirect(w, r, r.URL.Path, 303)
			return
		}
		s.flowError(w, r, err)
		return
	}
	if err = s.AttachSession(r.Context(), id, session); err != nil {
		s.flowError(w, r, err)
		return
	}
	http.Redirect(w, r, r.URL.Path, 303)
}

// continueInteraction 处理账户页的“继续”：可沿用授权时一键直达回调，
// 否则仅绑定会话进入 trust/consent 状态机。
func (s *Service) continueInteraction(w http.ResponseWriter, r *http.Request, id, browser string) {
	session, err := s.browserSession(r, browser)
	if err != nil {
		if errors.Is(err, application.ErrNoSession) {
			s.beginLogin(w, r, id, browser)
			return
		}
		s.flowError(w, r, err)
		return
	}
	a, _, err := s.InteractionInfo(r.Context(), id)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	c, err := s.ClientInfo(r.Context(), a.Request.ClientID)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	// trust 校验前置：不受信的客户端即使有历史授权也先走 trust 页。
	trusted, err := s.IsTrusted(r.Context(), session.AccountID, c)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	if !trusted {
		s.attachAndRefresh(w, r, id, browser)
		return
	}
	if err = s.ReuseGrant(r.Context(), id, session, c); err != nil {
		// 没有可沿用授权时退化为普通绑定，避免停在错误页。
		if errors.Is(err, application.ErrNoReusableGrant) {
			s.attachAndRefresh(w, r, id, browser)
			return
		}
		s.flowError(w, r, err)
		return
	}
	http.Redirect(w, r, "/oauth/authorize/callback?id="+url.QueryEscape(id), 303)
}

// confirmTrust 处理信任确认提交，成功后自刷新进入 consent。
func (s *Service) confirmTrust(w http.ResponseWriter, r *http.Request, id, browser string) {
	a, _, err := s.InteractionInfo(r.Context(), id)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	c, err := s.ClientInfo(r.Context(), a.Request.ClientID)
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	if err = s.ConfirmTrust(r.Context(), a, c); err != nil {
		s.flowError(w, r, err)
		return
	}
	http.Redirect(w, r, r.URL.Path, 303)
}
