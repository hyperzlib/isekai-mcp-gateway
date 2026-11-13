package application

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

// 明文 key 形如 "sk-<id段><secret段>"：id 段参与索引查询，secret 段仅存哈希。
const (
	apiKeyPrefix   = "sk-"
	apiKeyIDLen    = 16
	apiKeySecretLo = 24
)

// ErrNoReusableGrant 表示 (账号, 客户端, 资源) 下没有可沿用的活跃授权。
var ErrNoReusableGrant = errors.New("no reusable grant")

// permanentExpiry 是永久 key 在非空 expires_at 列中的远期哨兵时间。
func permanentExpiry() time.Time { return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC) }

func isPermanentExpiry(t time.Time) bool { return t.Year() >= 9999 }

func (s *Service) apiKeys() config.APIKeys { return s.Config.Static.Auth.Gateway.APIKeys }

func (s *Service) APIKeysEnabled() bool { return s.apiKeys().Enabled }

// CreateAPIKey 生成明文 key 并快照当前会话身份落库；明文仅在返回中出现一次。
func (s *Service) CreateAPIKey(ctx context.Context, session *Session, name string, scopes []string, expires time.Time) (string, *domain.APIKey, error) {
	if !s.APIKeysEnabled() {
		return "", nil, &FlowError{Code: "API_KEYS_DISABLED", Message: "API Key 功能未启用。", Status: 404}
	}
	cfg := s.apiKeys()
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 || strings.ContainsAny(name, "\r\n") {
		return "", nil, &FlowError{Code: "INVALID_REQUEST", Message: "API Key 名称无效。", Status: 400}
	}
	// scope 必须是已知业务范围子集，且恒含 mcp:access。
	for _, scope := range scopes {
		if !containsScope(domain.BusinessScopes(), scope) {
			return "", nil, domain.ErrInvalidScope()
		}
	}
	scopes, err := domain.NormalizeScopes(scopes, domain.BusinessScopes())
	if err != nil {
		return "", nil, err
	}
	now := time.Now().UTC()
	// 永久 key 使用远期哨兵时间；TTL 受配置上限约束。
	if isPermanentExpiry(expires) {
		if !cfg.AllowPermanent {
			return "", nil, &FlowError{Code: "INVALID_REQUEST", Message: "不允许创建永久 API Key。", Status: 400}
		}
		expires = permanentExpiry()
	} else {
		if !expires.After(now) || int64(expires.Sub(now).Seconds()) > int64(cfg.MaxTTLSeconds) {
			return "", nil, &FlowError{Code: "INVALID_REQUEST", Message: "API Key 有效期超出允许范围。", Status: 400}
		}
	}
	// 会话必须仍然有效，身份快照才有意义。
	if session == nil || session.Identity.UserID <= 0 {
		return "", nil, ErrNoSession
	}
	db := s.Store.Database().WithContext(ctx)
	var total int64
	if err := db.Table(persistence.Table("api_keys")).Where("account_id = ? AND state = ? AND expires_at > ?", session.AccountID, "active", now).Count(&total).Error(); err != nil {
		return "", nil, failure.Wrap("apikey.count", err)
	}
	if total >= int64(cfg.MaxPerUser) {
		return "", nil, &FlowError{Code: "API_KEY_LIMIT", Message: "API Key 数量已达上限，请先删除不再使用的密钥。", Status: 400}
	}
	keyID, secret := security.Random()[:apiKeyIDLen], security.Random()
	digest := sha256.Sum256([]byte(secret))
	key := domain.APIKey{ID: keyID, AccountID: session.AccountID, Name: name, Scopes: scopes, Identity: session.Identity, SecretHash: hex.EncodeToString(digest[:])}
	if err := s.Store.Create(db, "api_keys", keyID, key, persistence.Row{AccountID: session.AccountID, State: "active", ExpiresAt: expires}); err != nil {
		return "", nil, failure.Wrap("apikey.create", err)
	}
	return apiKeyPrefix + keyID + secret, &key, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, accountID string) ([]domain.APIKey, []persistence.Row, error) {
	var rows []persistence.Row
	q := s.Store.Database().WithContext(ctx).Table(persistence.Table("api_keys")).Where("account_id = ? AND state = ?", accountID, "active")
	if err := q.Order("updated_at DESC").Find(&rows).Error(); err != nil {
		return nil, nil, failure.Wrap("apikey.list", err)
	}
	keys, kept := []domain.APIKey{}, []persistence.Row{}
	for _, row := range rows {
		var key domain.APIKey
		if err := s.Store.OpenRecord(row, &key, "api_keys"); err != nil {
			return nil, nil, failure.Wrap("apikey.open", err)
		}
		keys = append(keys, key)
		kept = append(kept, row)
	}
	return keys, kept, nil
}

// DeleteAPIKey 按存储行 id 吊销 key；仅允许吊销属于该账号的记录。
func (s *Service) DeleteAPIKey(ctx context.Context, accountID, rowID string) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		var row persistence.Row
		if e := tx.Table(persistence.Table("api_keys")).Where("id = ? AND account_id = ?", rowID, accountID).Lock().Take(&row).Error(); e != nil {
			return lookupError(e, ErrInvalidToken)
		}
		if row.State != "active" {
			return nil
		}
		return tx.Table(persistence.Table("api_keys")).Where("id = ?", row.ID).Update("state", "revoked").Error()
	})
}

// ValidateAPIKey 解析明文 sk- key 并校验行状态、有效期与 secret 哈希，
// 返回可直接用于 MCP 调用链的 principal。
func (s *Service) ValidateAPIKey(ctx context.Context, raw string) (*Principal, error) {
	if !s.APIKeysEnabled() || len(raw) < len(apiKeyPrefix)+apiKeyIDLen+apiKeySecretLo || len(raw) > 256 || !strings.HasPrefix(raw, apiKeyPrefix) {
		return nil, ErrInvalidToken
	}
	db := s.Store.Database().WithContext(ctx)
	key, row, err := s.loadAPIKeyByID(db, raw[len(apiKeyPrefix):len(apiKeyPrefix)+apiKeyIDLen])
	if err != nil {
		return nil, err
	}
	secret := raw[len(apiKeyPrefix)+apiKeyIDLen:]
	digest := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(digest[:])), []byte(key.SecretHash)) != 1 {
		return nil, ErrInvalidToken
	}
	var account Account
	if _, err := s.Store.Get(db, "accounts", key.AccountID, &account, false); err != nil {
		return nil, lookupError(err, ErrInvalidToken)
	}
	if !containsScope(key.Scopes, "mcp:access") || key.Identity.UserID <= 0 {
		return nil, ErrInvalidToken
	}
	return &Principal{
		Issuer: s.Config.Static.Auth.Gateway.Issuer, AccountID: key.AccountID,
		OAuthClientID: "api-key", GrantID: key.ID, WikiUserID: key.Identity.UserID,
		Scopes: key.Scopes, ExpiresAt: row.ExpiresAt, AuthKind: domain.AuthKindAPIKey,
	}, nil
}

// loadAPIKeyByID 按 id 段检索并校验行状态与有效期。
func (s *Service) loadAPIKeyByID(db Database, keyID string) (*domain.APIKey, persistence.Row, error) {
	if len(keyID) != apiKeyIDLen {
		return nil, persistence.Row{}, ErrInvalidToken
	}
	var key domain.APIKey
	row, err := s.Store.Get(db, "api_keys", keyID, &key, false)
	if err != nil {
		return nil, row, lookupError(err, ErrInvalidToken)
	}
	if row.State != "active" || !row.ExpiresAt.After(time.Now()) {
		return nil, row, ErrInvalidToken
	}
	return &key, row, nil
}

// APIKeyFor 校验 principal 引用的 key 仍处于活跃状态且未过期，
// 供 MCP 长连接与 wiki 调用链复用。
func (s *Service) APIKeyFor(ctx context.Context, keyID string) (*domain.APIKey, persistence.Row, error) {
	db := s.Store.Database().WithContext(ctx)
	key, row, err := s.loadAPIKeyByID(db, keyID)
	if err != nil {
		return nil, row, err
	}
	var account Account
	if _, err := s.Store.Get(db, "accounts", key.AccountID, &account, false); err != nil {
		return nil, row, lookupError(err, ErrInvalidToken)
	}
	return key, row, nil
}

// RefreshAPIKeyIdentity 重新解析账号的 wiki 身份并回写 key 快照，
// 供 wiki 侧凭据失效时重取。
func (s *Service) RefreshAPIKeyIdentity(ctx context.Context, keyID string) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		var row persistence.Row
		if e := tx.Table(persistence.Table("api_keys")).Where("id = ? AND state = ?", security.Hash(keyID), "active").Lock().Take(&row).Error(); e != nil {
			return lookupError(e, ErrInvalidToken)
		}
		var key domain.APIKey
		if e := s.Store.OpenRecord(row, &key, "api_keys"); e != nil {
			return failure.Wrap("apikey.open", e)
		}
		var account Account
		if _, e := s.Store.Get(tx, "accounts", key.AccountID, &account, false); e != nil {
			return lookupError(e, ErrInvalidToken)
		}
		b := s.Upstream.Get()
		if b == nil {
			return failure.Dependency("apikey.refresh", errors.New("outbound unavailable"))
		}
		identity, e := b.Bridge.Resolve(ctx, account.Subject)
		if e != nil {
			return e
		}
		identity.AvatarURL = key.Identity.AvatarURL
		key.Identity = identity
		return s.Store.Put(tx, "api_keys", keyID, key, persistence.Row{AccountID: key.AccountID, State: row.State, ExpiresAt: row.ExpiresAt})
	})
}

// containsScope 避免 slices.Contains 在热路径外再多一层包装，保持可读。
func containsScope(scopes []string, scope string) bool {
	for _, v := range scopes {
		if v == scope {
			return true
		}
	}
	return false
}
