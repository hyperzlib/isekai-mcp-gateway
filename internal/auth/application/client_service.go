package application

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

func (s *Service) validateClient(c *Client, public bool) error {
	// 认证方式缺省为 none；公开注册的客户端不允许持有密钥。
	if c.TokenAuthMethod == "" {
		c.TokenAuthMethod = "none"
	}
	if public && c.TokenAuthMethod != "none" {
		return invalidMetadata("public registration requires token_endpoint_auth_method=none")
	}
	if !slices.Contains([]string{"none", "client_secret_basic", "client_secret_post"}, c.TokenAuthMethod) {
		return invalidMetadata("unsupported token endpoint authentication method")
	}

	// 授权类型仅支持 authorization_code 与 refresh_token，且必须包含前者。
	if len(c.Grants) == 0 {
		c.Grants = []string{"authorization_code"}
	}
	if !slices.Contains(c.Grants, "authorization_code") {
		return invalidMetadata("authorization_code grant is required")
	}
	for _, g := range c.Grants {
		if g != "authorization_code" && g != "refresh_token" {
			return invalidMetadata("unsupported grant type")
		}
	}

	// 仅支持 response_type=code；重复值归一化。
	if len(c.Responses) == 0 {
		c.Responses = []string{"code"}
	}
	c.Responses = slices.Clone(c.Responses)
	slices.Sort(c.Responses)
	c.Responses = slices.Compact(c.Responses)
	if len(c.Responses) != 1 || c.Responses[0] != "code" {
		return invalidMetadata("only response_type=code is supported")
	}

	// 限制重定向地址数量与客户端名称长度，避免元数据膨胀。
	if len(c.Redirects) == 0 || len(c.Redirects) > 16 || len(c.Name) > 256 {
		return invalidMetadata("invalid client metadata size")
	}

	// 重定向地址必须是绝对 URI：HTTPS、回环 HTTP 或公开客户端的私有 scheme。
	for _, raw := range c.Redirects {
		u, err := url.Parse(raw)
		if err != nil || !u.IsAbs() || u.User != nil || u.Fragment != "" {
			return invalidMetadata("invalid redirect URI")
		}
		if u.Scheme == "https" && u.Host != "" {
			continue
		}
		if u.Scheme == "http" && (net.ParseIP(u.Hostname()).IsLoopback() || u.Hostname() == "localhost") {
			continue
		}
		if public && domain.PrivateApplicationScheme(u.Scheme) {
			continue
		}
		return invalidMetadata("redirect URI requires HTTPS, loopback HTTP, or a private application scheme")
	}
	// 归一化并校验请求的 scope，offline_access 必须搭配 refresh_token 授权。
	if c.Scope == "" {
		c.Scope = "mcp:access"
		if public {
			// DCR 缺省允许基础 scope；实际授予范围由授权请求决定。
			basic := BasicScopes()
			if !slices.Contains(c.Grants, "refresh_token") {
				basic = slices.DeleteFunc(basic, func(scope string) bool { return scope == "offline_access" })
			}
			c.Scope += " " + strings.Join(basic, " ")
		}
	}
	scopes, err := NormalizeScopes(strings.Fields(c.Scope), BusinessScopes())
	if err != nil {
		return invalidMetadata("requested scope is not allowed")
	}
	if slices.Contains(scopes, "offline_access") && !slices.Contains(c.Grants, "refresh_token") {
		return invalidMetadata("offline_access requires refresh_token grant")
	}
	c.Scope = strings.Join(scopes, " ")

	if !domain.WebMetadataURL(c.URI, false) {
		return invalidMetadata("invalid client_uri")
	}
	if c.LogoURL != nil && !domain.WebMetadataURL(*c.LogoURL, true) {
		return invalidMetadata("invalid logo_uri")
	}
	c.Fingerprint, _, err = domain.ClientFingerprint(c)
	if err != nil {
		return invalidMetadata("invalid fingerprint metadata")
	}
	return nil
}

func (s *Service) PersistClient(tx Database, c *Client, state string, expiry time.Time) error {
	if c.TrustLevel == "" {
		c.TrustLevel = domain.DefaultTrust(c.Source)
	}
	d := storedClient{Metadata: *c, Source: c.Source, Fingerprint: c.Fingerprint, SecretHash: c.SecretHash, FetchedAt: c.FetchedAt, ResourceClient: c.ResourceClient}
	return s.Store.Put(tx, "clients", c.ID, d, persistence.Row{ClientID: security.Hash(c.ID), State: state, ExpiresAt: expiry, Fingerprint: c.Fingerprint, ClientName: c.Name, LogoURL: c.LogoURL, TrustLevel: string(c.TrustLevel)})
}

func (s *Service) LoadClient(tx Database, id string, lock bool) (*Client, persistence.Row, error) {
	var d storedClient
	r, err := s.Store.Get(tx, "clients", id, &d, lock)
	if err != nil {
		return nil, r, err
	}
	// active 记录直接放行；pending 记录一旦过期即视为无效。
	if r.State != "active" && r.State != "pending" || r.State == "pending" && !r.ExpiresAt.After(time.Now()) {
		return nil, r, domain.ErrInvalidClient()
	}
	c := d.Metadata
	c.Source, c.Fingerprint, c.SecretHash, c.FetchedAt, c.ResourceClient = d.Source, d.Fingerprint, d.SecretHash, d.FetchedAt, d.ResourceClient
	if err := domain.ClientAllowed(&c); err != nil {
		return &c, r, err
	}
	return &c, r, nil
}

func (s *Service) registryLock(tx Database) error {
	_, err := s.Store.Get(tx, "clients", "@registry", new(struct{}), true)
	return err
}

func (s *Service) Register(ctx context.Context, c *Client) error {
	if !s.Config.Static.Auth.Gateway.Registration.Dynamic {
		return errors.New("dynamic registration is disabled")
	}
	if c.ID != "" {
		return invalidMetadata("client_id is assigned by the server")
	}
	if err := s.validateClient(c, true); err != nil {
		return err
	}
	c.Source = "dcr"
	c.TrustLevel = registrationTrust(c)
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		// 注册表锁保证并发注册时的容量计数一致。
		if err := s.registryLock(tx); err != nil {
			return err
		}
		var rows []persistence.Row
		if err := tx.Table(persistence.Table("clients")).Where("fingerprint = ? AND (state = ? OR (state = ? AND expires_at > ?))", c.Fingerprint, "active", "pending", time.Now().UTC()).Order("CASE WHEN state = 'active' THEN 0 ELSE 1 END, id").Lock().Find(&rows).Error(); err != nil {
			return err
		}
		for _, row := range rows {
			var previous storedClient
			if err := s.Store.OpenRecord(row, &previous, "clients"); err != nil {
				return err
			}
			old := previous.Metadata
			if previous.Source != "dcr" || old.TokenAuthMethod != "none" {
				continue
			}
			if row.LogoURL == nil && c.LogoURL != nil && *c.LogoURL != "" {
				if err := tx.Table(persistence.Table("clients")).Where("id = ? AND logo_url IS NULL", row.ID).Update("logo_url", *c.LogoURL).Error(); err != nil {
					return err
				}
				old.LogoURL = c.LogoURL
			}
			if err := tx.Table(persistence.Table("clients")).Where("id = ?", row.ID).Update("updated_at", time.Now().UTC()).Error(); err != nil {
				return err
			}
			old.Source, old.Fingerprint, old.SecretHash, old.ResourceClient = previous.Source, previous.Fingerprint, previous.SecretHash, previous.ResourceClient
			*c = old
			return nil
		}
		c.ID = security.Random()
		if c.LogoURL == nil {
			empty := ""
			c.LogoURL = &empty
		}
		var count int64
		if err := tx.Table(persistence.Table("clients")).Where("state IN ?", []string{"pending", "active"}).Count(&count).Error(); err != nil {
			return err
		}
		// 容量已满时返回可重试的失败。
		if count >= int64(s.Config.Static.Auth.Gateway.Registration.MaxClients) {
			return &failure.Error{Code: failure.Capacity, Message: "客户端注册容量已满，请稍后重试。", Op: "client.register", Retryable: true}
		}
		return s.PersistClient(tx, c, "pending", time.Now().UTC().Add(24*time.Hour))
	})
}

func (s *Service) SeedClients(ctx context.Context) error {
	db := s.Store.Database().WithContext(ctx)
	if err := s.Store.Put(db, "clients", "@registry", struct{}{}, persistence.Row{State: "registry", ExpiresAt: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		return err
	}
	configured := map[string]bool{}
	for _, v := range s.Config.Static.Auth.Gateway.Registration.Clients {
		configured[v.ID] = true
		c := &Client{ID: v.ID, Name: v.Name, URI: v.URI, LogoURL: v.LogoURL, Redirects: v.Redirects, Grants: v.Grants, Responses: v.Responses, TokenAuthMethod: v.AuthMethod, Scope: v.Scope, Source: "pre", ResourceClient: v.ResourceClient}
		if secret := s.Config.Secrets.ClientSecrets[v.ID]; secret != "" {
			c.SecretHash = security.Hash(secret)
		}
		if c.TokenAuthMethod != "" && c.TokenAuthMethod != "none" && c.SecretHash == "" {
			return errors.New("confidential client secret is missing")
		}
		if err := s.validateClient(c, false); err != nil {
			return err
		}
		if err := s.updateClient(ctx, c, "active"); err != nil {
			return err
		}
	}
	var rows []persistence.Row
	if err := db.Table(persistence.Table("clients")).Where("state = ?", "active").Find(&rows).Error(); err != nil {
		return err
	}
	// 清理配置中已移除的预注册客户端，并吊销其关联授权。
	for _, r := range rows {
		var d storedClient
		if err := s.Store.OpenRecord(r, &d, "clients"); err != nil {
			return err
		}
		if d.Source == "pre" && !configured[d.Metadata.ID] {
			if err := db.Transaction(func(tx Database) error {
				if err := tx.Table(persistence.Table("clients")).Where("id = ?", r.ID).Update("state", "disabled").Error(); err != nil {
					return err
				}
				return tx.Table(persistence.Table("grants")).Where("client_id = ?", r.ClientID).Update("state", "revoked").Error()
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) updateClient(ctx context.Context, c *Client, state string) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		if err := s.registryLock(tx); err != nil {
			return err
		}
		var previous storedClient
		previousRow, err := s.Store.Get(tx, "clients", c.ID, &previous, true)
		if err != nil && !errors.Is(err, ErrRecordNotFound) {
			return err
		}
		exists := err == nil
		if exists {
			c.TrustLevel = domain.TrustLevel(previousRow.TrustLevel)
			c.LogoURL = previousRow.LogoURL
		} else {
			c.TrustLevel = registrationTrust(c)
			if c.LogoURL == nil {
				empty := ""
				c.LogoURL = &empty
			}
		}
		if exists && previous.Source != c.Source {
			return errors.New("client registration source cannot change")
		}
		// 指纹变化表示客户端配置被篡改，需吊销旧授权与信任记录。
		if exists && previous.Fingerprint != c.Fingerprint {
			if c.Source == "cimd" {
				c.TrustLevel = registrationTrust(c)
			}
			if err := tx.Table(persistence.Table("grants")).Where("client_id = ?", security.Hash(c.ID)).Update("state", "revoked").Error(); err != nil {
				return err
			}
			if err := tx.Table(persistence.Table("client_trusts")).Where("client_id = ?", security.Hash(c.ID)).Delete(&persistence.Row{}).Error(); err != nil {
				return err
			}
		}
		// 新建客户端时同样受容量上限约束。
		if !exists {
			var n int64
			if err := tx.Table(persistence.Table("clients")).Where("state IN ?", []string{"active", "pending"}).Count(&n).Error(); err != nil {
				return err
			}
			if n >= int64(s.Config.Static.Auth.Gateway.Registration.MaxClients) {
				return &failure.Error{Code: failure.Capacity, Message: "客户端注册容量已满，请稍后重试。", Op: "client.register", Retryable: true}
			}
		}
		return s.PersistClient(tx, c, state, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
	})
}

func (s *Service) GetClientByClientID(ctx context.Context, id string) (*Client, error) {
	if id == "" || len(id) > 2048 {
		return nil, domain.ErrInvalidClient()
	}
	// CIMD 客户端以 URL 作为标识，需在开关开启时拉取并缓存其元数据。
	if strings.HasPrefix(id, "https://") {
		if !s.Config.Static.Auth.Gateway.Registration.CIMD {
			return nil, domain.ErrInvalidClient().WithDescription("CIMD is disabled")
		}
		cached, _, err := s.LoadClient(s.Store.Database().WithContext(ctx), id, false)
		if err != nil && !errors.Is(err, ErrRecordNotFound) && failure.IsInfrastructure(err) {
			return nil, failure.Wrap("client.cache", err)
		}
		// 命中 5 分钟内的缓存则直接返回，避免频繁拉取上游元数据。
		if cached != nil && cached.FetchedAt.After(time.Now().Add(-5*time.Minute)) {
			return cached, err
		}
		b, err := s.FetchMetadata(ctx, id, s.Config.Static.Auth.Gateway)
		if err != nil {
			return nil, err
		}
		// 拉取到的文档中 client_id 必须与 URL 标识一致，防止冒充他人客户端。
		var c Client
		if err = json.Unmarshal(b, &c); err != nil || c.ID != id {
			return nil, domain.ErrInvalidClient().WithDescription("invalid CIMD document or client_id")
		}
		c.Source, c.FetchedAt = "cimd", time.Now().UTC()
		if err = s.validateClient(&c, true); err != nil {
			return nil, err
		}
		if err = s.updateClient(ctx, &c, "active"); err != nil {
			return nil, err
		}
		if err := domain.ClientAllowed(&c); err != nil {
			return nil, err
		}
		return &c, nil
	}
	c, _, err := s.LoadClient(s.Store.Database().WithContext(ctx), id, false)
	if errors.Is(err, ErrRecordNotFound) {
		// Idle pre-registered records may be reclaimed. Their immutable config
		// can recreate the registration when the client next connects.
		for _, v := range s.Config.Static.Auth.Gateway.Registration.Clients {
			if v.ID != id {
				continue
			}
			c = &Client{ID: v.ID, Name: v.Name, URI: v.URI, LogoURL: v.LogoURL, Redirects: v.Redirects, Grants: v.Grants, Responses: v.Responses, TokenAuthMethod: v.AuthMethod, Scope: v.Scope, Source: "pre", ResourceClient: v.ResourceClient}
			if secret := s.Config.Secrets.ClientSecrets[id]; secret != "" {
				c.SecretHash = security.Hash(secret)
			}
			if err = s.validateClient(c, false); err == nil {
				err = s.updateClient(ctx, c, "active")
			}
			break
		}
	}
	if err != nil {
		var oauth *domain.OAuthError
		if errors.As(err, &oauth) && oauth.Code == "unauthorized_client" {
			return nil, err
		}
		return nil, lookupError(err, domain.ErrInvalidClient())
	}
	// 刷新活跃时间，避免预注册客户端被空闲回收。
	if err = s.Store.Database().WithContext(ctx).Table(persistence.Table("clients")).Where("id = ?", security.Hash(id)).Update("updated_at", time.Now().UTC()).Error(); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) AuthorizeClientIDSecret(ctx context.Context, id, secret string) error {
	c, _, err := s.LoadClient(s.Store.Database().WithContext(ctx), id, false)
	if err != nil {
		return lookupError(err, domain.ErrInvalidClient())
	}
	// 使用常量时间比较，防止通过响应时间侧信道推测密钥。
	if c.TokenAuthMethod == "none" || c.SecretHash == "" || subtle.ConstantTimeCompare([]byte(c.SecretHash), []byte(security.Hash(secret))) != 1 {
		return domain.ErrInvalidClient()
	}
	return nil
}

func registrationTrust(c *Client) domain.TrustLevel {
	if c.Source == "dcr" || c.Source == "cimd" {
		for _, raw := range c.Redirects {
			u, err := url.Parse(raw)
			if err == nil && u.Scheme != "http" && u.Scheme != "https" && !strings.Contains(u.Scheme, ".") {
				return domain.TrustBlocked
			}
		}
	}
	return domain.DefaultTrust(c.Source)
}
