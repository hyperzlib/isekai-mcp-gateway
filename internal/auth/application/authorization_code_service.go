package application

import (
	"context"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

func (s *Service) CreateAuthRequest(ctx context.Context, r *domain.AuthorizationRequest, _ string) (*Interaction, error) {
	// 授权请求必须携带请求上下文中的资源与浏览器信息。
	info, ok := ctx.Value(RequestKey{}).(RequestInfo)
	if !ok {
		return nil, domain.ErrInvalidRequest()
	}
	c, _, err := s.LoadClient(s.Store.Database().WithContext(ctx), r.ClientID, false)
	if err != nil {
		return nil, lookupError(err, domain.ErrInvalidClient())
	}
	a := &Interaction{ID: security.Random(), Request: *r, Resource: info.Resource, Browser: info.Browser, ClientFingerprint: c.Fingerprint}
	err = s.Store.Create(s.Store.Database().WithContext(ctx), "interactions", a.ID, a, persistence.Row{ClientID: security.Hash(c.ID), State: "login", ExpiresAt: time.Now().UTC().Add(Seconds(s.Config.Static.Auth.Gateway.InteractionTTL))})
	return a, err
}

func (s *Service) LoadInteraction(db Database, id string, lock bool) (*Interaction, persistence.Row, error) {
	var a Interaction
	r, err := s.Store.Get(db, "interactions", id, &a, lock)
	if err != nil {
		return nil, r, lookupError(err, &FlowError{Code: "INVALID_INTERACTION", Message: "找不到本次授权请求，链接可能已被清理。请返回应用重新发起连接。", Status: 400, Cause: domain.ErrInvalidGrant()})
	}
	// 交互已过期或被取消即视为无效。
	if !r.ExpiresAt.After(time.Now()) {
		return nil, r, &FlowError{Code: "INVALID_INTERACTION", Message: "本次授权请求已过期，请返回应用重新发起连接。", Status: 400, Cause: domain.ErrInvalidGrant()}
	}
	if r.State == "cancelled" {
		return nil, r, &FlowError{Code: "INVALID_INTERACTION", Message: "本次授权已取消，请返回应用重新发起连接。", Status: 400, Cause: domain.ErrInvalidGrant()}
	}
	c, _, err := s.LoadClient(db, a.Request.ClientID, false)
	if err != nil {
		return nil, r, lookupError(err, &FlowError{Code: "INVALID_INTERACTION", Message: "本次授权使用的应用注册已失效，请返回应用重新注册连接。", Status: 400, Cause: domain.ErrInvalidClient()})
	}
	// 客户端指纹变化说明客户端已重新注册，旧交互失效。
	if c.Fingerprint != a.ClientFingerprint {
		return nil, r, &FlowError{Code: "INVALID_INTERACTION", Message: "应用的连接配置已变更，本次授权无法继续。请返回应用重新发起连接。", Status: 400, Cause: domain.ErrInvalidClient()}
	}
	return &a, r, nil
}

func (s *Service) AuthRequestByID(ctx context.Context, id string) (*Interaction, error) {
	a, _, err := s.LoadInteraction(s.Store.Database().WithContext(ctx), id, false)
	return a, err
}

func (s *Service) AuthRequestByCode(ctx context.Context, code string) (*Interaction, error) {
	var c Code
	r, err := s.Store.Get(s.Store.Database().WithContext(ctx), "authorization_codes", code, &c, false)
	if err != nil {
		return nil, lookupError(err, domain.ErrInvalidGrant())
	}
	// 授权码必须处于活跃状态且未过期。
	if r.State != "active" || !r.ExpiresAt.After(time.Now()) {
		return nil, domain.ErrInvalidGrant()
	}
	a, _, err := s.LoadInteraction(s.Store.Database().WithContext(ctx), c.InteractionID, false)
	if err != nil {
		return nil, err
	}
	if !a.DoneFlag {
		return nil, domain.ErrInvalidGrant()
	}
	// 换取令牌的请求必须指向同一资源，防止跨资源使用授权码。
	info, _ := ctx.Value(RequestKey{}).(RequestInfo)
	if info.Resource != a.Resource {
		return nil, domain.ErrInvalidGrant().WithDescription("resource mismatch")
	}
	return a, nil
}

func (s *Service) SaveAuthCode(ctx context.Context, id, code string) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		a, row, err := s.LoadInteraction(tx, id, true)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		// 只有已同意授权的交互才能生成授权码。
		if row.State != "consented" || !a.DoneFlag {
			return domain.ErrInvalidGrant()
		}
		row.State = "code"
		if err = s.Store.Put(tx, "interactions", id, a, row); err != nil {
			return err
		}
		return s.Store.Create(tx, "authorization_codes", code, Code{InteractionID: id}, persistence.Row{AccountID: a.AccountID, ClientID: security.Hash(a.Request.ClientID), GrantID: a.GrantID, State: "active", ExpiresAt: time.Now().UTC().Add(Seconds(s.Config.Static.Auth.Gateway.CodeTTL))})
	})
}

func (s *Service) DeleteAuthRequest(context.Context, string) error {
	// Issuance already consumed the interaction atomically. Retain the record
	// until its TTL to reject racing exchanges and keep provider callbacks safe.
	return nil
}
