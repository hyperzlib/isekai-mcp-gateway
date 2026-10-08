package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/outbound"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

type loginSnapshot struct {
	App              config.AppFile
	OidcClientSecret string
	BridgeSecret     string
}

func (s *Service) CompleteLogin(ctx context.Context, state, code, browser string, denied bool) (*LoginResult, error) {
	var transaction LoginTransaction
	// Consume before external calls. A failed exchange requires a new login,
	// never a replay of the same state or upstream code.
	err := s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		row, err := s.Store.Get(tx, "login_transactions", state, &transaction, true)
		if err != nil {
			return lookupError(err, &FlowError{Code: "INVALID_LOGIN_TRANSACTION", Message: "登录事务已过期、已使用或属于其他浏览器。", Status: 400})
		}
		// 事务必须处于活跃状态、未过期且属于发起登录的同一浏览器。
		if row.State != "active" || !row.ExpiresAt.After(time.Now()) || transaction.Browser != browser {
			return &FlowError{Code: "INVALID_LOGIN_TRANSACTION", Message: "登录事务已过期、已使用或属于其他浏览器。", Status: 400}
		}
		return tx.Table(persistence.Table("login_transactions")).Where("id = ?", row.ID).Update("state", "used").Error()
	})
	if err != nil {
		return nil, loginError("login.consume", err)
	}
	// 用户取消或缺少授权码时直接结束流程。
	if denied || code == "" {
		return nil, &FlowError{Message: "身份服务取消了登录，请重新发起。", Status: 400}
	}
	var snap loginSnapshot
	if err := json.Unmarshal(transaction.Snapshot, &snap); err != nil {
		return nil, &FlowError{Message: "无法恢复登录配置。", Status: 500, Cause: failure.Wrap("login.snapshot", err)}
	}
	c := &config.Config{App: snap.App, Secrets: config.Secrets{OidcClientSecret: snap.OidcClientSecret, BridgeSecret: snap.BridgeSecret}}
	// 限制整个上游登录调用的总时长，避免请求长时间挂起。
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	b, err := outbound.Build(ctx, c)
	if err != nil {
		return nil, loginError("login.discovery", err)
	}
	subject, err := b.OidcClient.Authenticate(ctx, code, transaction.Verifier, transaction.Nonce)
	if err != nil {
		return nil, loginError("login.authenticate", err)
	}
	identity, err := b.Bridge.Resolve(ctx, subject)
	if err != nil {
		// 身份服务返回的业务错误直接透传其错误码与说明。
		var failure *domain.IdentityError
		if errors.As(err, &failure) {
			return nil, &FlowError{Code: failure.Code, Message: failure.Message, Status: 403, Cause: err}
		}
		return nil, loginError("login.identity", err)
	}
	accountID, sessionID := "", security.Random()
	err = s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		// Stable digest avoids duplicate accounts when multiple callbacks race.
		accountID = security.Hash(snap.App.Auth.OidcClient.Issuer + "\x00" + subject)
		account := Account{ID: accountID, Issuer: snap.App.Auth.OidcClient.Issuer, Subject: subject, SiteID: snap.App.MediaWiki.SiteID, WikiUserID: identity.UserID, Username: identity.Username, DisplayName: identity.DisplayName}
		var previous Account
		ar, e := s.Store.Get(tx, "accounts", accountID, &previous, true)
		if e != nil && !errors.Is(e, ErrRecordNotFound) {
			return e
		}
		// 已停用账号拒绝登录。
		if e == nil && ar.State != "active" {
			return &FlowError{Code: "ACCOUNT_DISABLED", Message: "账号已停用，请联系管理员。", Status: 403}
		}
		// 同一站点下 Wiki 用户发生变更，说明账号被复用，需吊销旧授权。
		if e == nil && previous.SiteID == account.SiteID && previous.WikiUserID != identity.UserID {
			if e = tx.Table(persistence.Table("grants")).Where("account_id = ?", accountID).Update("state", "revoked").Error(); e != nil {
				return e
			}
		}
		expires := time.Now().UTC().Add(Seconds(s.Config.Static.Auth.Gateway.GrantTTL))
		if e = s.Store.Put(tx, "accounts", accountID, account, persistence.Row{AccountID: accountID, State: "active", ExpiresAt: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)}); e != nil {
			return e
		}
		if e = s.Store.Create(tx, "sessions", sessionID, Session{ID: sessionID, AccountID: accountID, SiteID: account.SiteID, Browser: browser, Identity: identity}, persistence.Row{AccountID: accountID, State: "active", ExpiresAt: expires}); e != nil {
			return e
		}
		// 若登录由某个连接交互发起，则回填账号与会话并推进状态机。
		if transaction.InteractionID != "" {
			a, row, e := s.LoadInteraction(tx, transaction.InteractionID, true)
			if e != nil {
				return e
			}
			if a.Browser != browser || row.State != "login" {
				return &FlowError{Code: "INVALID_INTERACTION", Message: "这次连接已失效，请重新发起。", Status: 400}
			}
			a.AccountID, a.SessionID, a.AuthTime = accountID, sessionID, time.Now().UTC()
			row.AccountID, row.State = accountID, "authenticated"
			if e = s.Store.Put(tx, "interactions", a.ID, a, row); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return nil, loginError("login.persist", err)
	}

	return &LoginResult{SessionID: sessionID, InteractionID: transaction.InteractionID}, nil
}

func (s *Service) BeginLogin(ctx context.Context, interactionID, browser string) (string, error) {
	b := s.Upstream.Get()
	// state、nonce 与 PKCE verifier 各自独立随机生成。
	state, nonce, verifier := security.Random(), security.Random(), security.Random()
	snapshot, err := json.Marshal(loginSnapshot{App: b.Config.App, OidcClientSecret: b.Config.Secrets.OidcClientSecret, BridgeSecret: b.Config.Secrets.BridgeSecret})
	if err != nil {
		return "", &FlowError{Message: "无法创建登录事务。", Status: 500, Cause: failure.Wrap("login.snapshot", err)}
	}
	t := LoginTransaction{ID: state, InteractionID: interactionID, Browser: browser, Nonce: nonce, Verifier: verifier, Snapshot: snapshot}
	err = s.Store.Create(s.Store.Database().WithContext(ctx), "login_transactions", state, t, persistence.Row{State: "active", ExpiresAt: time.Now().UTC().Add(Seconds(s.Config.Static.Auth.Gateway.InteractionTTL))})
	if err != nil {
		return "", &FlowError{Message: "无法保存登录事务。", Status: 500, Cause: failure.Wrap("login.create", err)}
	}
	return b.OidcClient.AuthorizationURL(state, nonce, verifier), nil
}

func loginError(op string, err error) error {
	var flow *FlowError
	if errors.As(err, &flow) {
		return flow
	}
	f := failure.Describe(failure.Wrap(op, err))
	status := failure.HTTPStatus(f)
	// 非基础设施的 500 降级为 400，避免把内部错误暴露给浏览器。
	if !failure.IsInfrastructure(f) && status == 500 {
		status = 400
	}
	if f.Code == failure.Unavailable {
		status = 502
	}
	return &FlowError{Code: f.Code, Message: f.Message, Status: status, Cause: f}
}
