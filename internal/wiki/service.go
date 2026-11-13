package wiki

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/integrations/mediawiki"
	"isekai-mcp-gateway/internal/security"
	"isekai-mcp-gateway/internal/templates"
	"slices"
	"strings"
	"sync"
	"time"
)

type Service struct {
	Auth      *application.Service
	Repo      *Repository
	sites     sync.Map
	pageCache readCache
	Text      templates.TextRenderer
}

func (s *Service) Render(key string, data any, fallback string) string {
	if s.Text == nil {
		return fallback
	}
	var b bytes.Buffer

	// 渲染失败、为空或超过 8192 字节时回退到静态文本。
	if x := s.Text.RenderText(&b, key, data); x != nil || b.Len() == 0 || b.Len() > 8192 {
		return fallback
	}
	return b.String()
}
func (s *Service) DescribeError(err error) error {
	f := failure.Describe(err)
	copy := *f
	copy.Message = s.Render("errors/"+f.Code+".md", Object{"Message": f.Message}, s.Render("errors/error.md", Object{"Message": f.Message}, f.Message))
	return &copy
}

type Call struct {
	Principal   *application.Principal
	Credentials mediawiki.Credentials
	Client      *mediawiki.Client
	Config      *config.Config
}

func errCode(code, message string) error { return failure.New(code, message, "wiki", nil) }
func detail(code, message string, d Object) error {
	return &failure.Error{Code: code, Message: message, Op: "wiki", Details: d}
}
func (s *Service) Call(ctx context.Context, extra *mcp.RequestExtra, scope string) (*Call, error) {
	if extra == nil || extra.TokenInfo == nil {
		return nil, application.ErrAuthenticationRequired
	}
	// principal 必须存在且 token 未过期。
	p, ok := extra.TokenInfo.Extra["principal"].(*application.Principal)
	if !ok || p == nil || !p.ExpiresAt.After(time.Now()) {
		return nil, application.ErrInvalidToken
	}

	// mcp:access 是基础作用域，调用声明的 scope 也需全部具备。
	if !slices.Contains(p.Scopes, "mcp:access") || scope != "" && !slices.Contains(p.Scopes, scope) {
		return nil, detail("INSUFFICIENT_SCOPE", "Authorize the required scope.", Object{"required_scopes": []string{scope}})
	}
	db := s.Auth.Store.Database().WithContext(ctx)

	// 凭据来源分派：OAuth principal 经授权链校验，API key principal 复核 key 记录。
	var (
		credentials mediawiki.Credentials
		accountID   string
		siteID      string
	)
	if p.AuthKind == domain.AuthKindAPIKey {
		key, _, e := s.Auth.APIKeyFor(ctx, p.GrantID)
		if e != nil {
			return nil, e
		}
		if key.Identity.UserID != p.WikiUserID {
			return nil, application.ErrInvalidToken
		}
		credentials = mediawiki.Credentials{UserID: p.WikiUserID, Token: key.Identity.UserToken}
		accountID = key.AccountID
	} else {
		g, _, e := s.Auth.ActiveGrant(db, p.GrantID, false)
		if e != nil {
			return nil, e
		}
		accountID = g.AccountID

		// 校验 grant 对应的会话仍存在，且会话身份与 token 声明的用户一致。
		var session application.Session
		if _, e = s.Auth.Store.Get(db, "sessions", g.SessionID, &session, false); e != nil {
			return nil, e
		}
		if session.Identity.UserID != p.WikiUserID {
			return nil, application.ErrInvalidToken
		}
		credentials = mediawiki.Credentials{UserID: p.WikiUserID, Token: session.Identity.UserToken}
		// 会话绑定站点优先，缺失时回退到账户站点。
		siteID = session.SiteID
	}

	// 上游连接必须在运行期可用。
	b := s.Auth.Upstream.Get()
	if b == nil {
		return nil, errCode("SERVICE_UNAVAILABLE", "Wiki connection is unavailable.")
	}
	if e := s.rememberSite(ctx, b.Bridge); e != nil {
		return nil, e
	}
	call := &Call{p, credentials, b.Bridge, b.Config}
	var account application.Account
	if _, e := s.Auth.Store.Get(db, "accounts", accountID, &account, false); e != nil {
		return nil, e
	}
	if siteID == "" {
		siteID = account.SiteID
	}
	if siteID != "" {
		if e := s.site(call, siteID); e != nil {
			return nil, e
		}
	}
	// API key 凭据是创建时的身份快照，wiki 鉴权失败时重取一次并重试。
	if p.AuthKind == domain.AuthKindAPIKey {
		if e := s.refreshCredentialsIfNeeded(ctx, call); e != nil {
			return nil, e
		}
	}
	return call, nil
}

// bridgeAuthError 判定错误是否来自 wiki 侧的用户凭据校验失败。
func bridgeAuthError(err error) bool {
	var apiErr *mediawiki.APIError
	return errors.As(err, &apiErr) && strings.HasPrefix(apiErr.Code, "isekai-servicebridge-")
}

// refreshCredentialsIfNeeded 用轻量请求探测凭据有效性；Bridge 拒绝时
// 重新解析身份并重试一次，避免快照凭据失效后持续报错。
func (s *Service) refreshCredentialsIfNeeded(ctx context.Context, call *Call) error {
	if _, err := call.Client.Query(ctx, call.Credentials, url.Values{"action": {"query"}, "meta": {"userinfo"}}); err == nil || !bridgeAuthError(err) {
		return nil
	}
	if err := s.Auth.RefreshAPIKeyIdentity(ctx, call.Principal.GrantID); err != nil {
		return nil // 重取失败不阻断本次调用，由具体工具按原错误反馈
	}
	key, _, err := s.Auth.APIKeyFor(ctx, call.Principal.GrantID)
	if err != nil {
		return err
	}
	call.Credentials = mediawiki.Credentials{UserID: call.Principal.WikiUserID, Token: key.Identity.UserToken}
	if _, err := call.Client.Query(ctx, call.Credentials, url.Values{"action": {"query"}, "meta": {"userinfo"}}); err != nil && bridgeAuthError(err) {
		return err
	}
	return nil
}

// owner 摘要绑定站点与用户，用于隔离各站点的持久化对象。
func (c *Call) owner(site string) string {
	return security.Hash(c.Principal.Issuer + "\x00" + c.Principal.AccountID + "\x00" + site + fmt.Sprint(c.Principal.WikiUserID))
}

// principalKey 不含站点，用于判定跨站点的确认归属。
func (c *Call) principalKey() string {
	return security.Hash(c.Principal.Issuer + "\x00" + c.Principal.AccountID + "\x00" + c.Principal.OAuthClientID + "\x00" + c.Principal.GrantID)
}

// require 逐个校验调用所需的 scope。
func require(c *Call, scopes ...string) error {
	for _, v := range scopes {
		if !slices.Contains(c.Principal.Scopes, v) {
			return detail("INSUFFICIENT_SCOPE", "Authorize the required scope.", Object{"required_scopes": scopes})
		}
	}
	return nil
}
func (s *Service) site(c *Call, id string) error {
	if c.Client.Config.SiteID == id {
		return nil
	}

	// 切换到会话原始站点对应的客户端，代际缺失时无法响应。
	v, ok := s.sites.Load(id)
	if !ok {
		return errCode("SITE_GENERATION_UNAVAILABLE", "The session's original Wiki connection is unavailable.")
	}
	c.Client = v.(*mediawiki.Client)
	return nil
}

// positive 将 0 视为默认值，并限制结果落在 [1,max]。
func positive(v, def, max int) (int, error) {
	if v == 0 {
		v = def
	}
	if v < 1 || v > max {
		return 0, errCode("INVALID_REQUEST", "Limit is outside the supported range.")
	}
	return v, nil
}
func object(v any) Object {
	if x, ok := v.(map[string]any); ok {
		return x
	}
	return Object{}
}
func number(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	}
	return 0
}
func str(v any) string { x, _ := v.(string); return x }
func pageParams(p PageRef) (map[string][]string, error) {
	// page_id 与 title 必须二选一，且 page_id 不能为负。
	if (p.PageID > 0) == (p.Title != "") || p.PageID < 0 {
		return nil, errCode("INVALID_REQUEST", "Supply exactly one page_id or title.")
	}
	if p.PageID > 0 {
		return map[string][]string{"pageids": {fmt.Sprint(p.PageID)}}, nil
	}

	// 标题禁止含分隔符与控制字符，长度上限 1024。
	if strings.ContainsAny(p.Title, "|\x00\r\n") || len(p.Title) > 1024 {
		return nil, errCode("INVALID_REQUEST", "Invalid page title.")
	}
	return map[string][]string{"titles": {p.Title}}, nil
}

type cursorData struct {
	Tool, Owner, Filter string
	Continue            Object
	Offset              int
	Version             int64
}

// cursor 将载荷与 HMAC-SHA256 签名拼接为不透明游标。
func cursor(v cursorData) string {
	b, _ := json.Marshal(v)
	mac := hmac.New(sha256.New, cursorSecret())
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// 未配置存储密钥时使用进程内随机密钥，重启后旧游标会全部失效。
var fallbackCursorKey = []byte(security.Random())

func cursorSecret() []byte {
	if c := config.GetConfig(); c != nil && len(c.Secrets.Storage) > 0 {
		return c.Secrets.Storage
	}
	return fallbackCursorKey
}

// readCursor 校验签名与上下文，返回可信的游标数据。
func readCursor(raw, tool, owner, filter string) (cursorData, error) {
	var v cursorData
	if raw == "" {
		return v, nil
	}

	// 限制游标长度，避免解析超大输入。
	if len(raw) > 8192 {
		return v, errCode("INVALID_CURSOR", "Cursor is invalid.")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return v, errCode("INVALID_CURSOR", "Cursor is invalid.")
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[0])
	signature, se := base64.RawURLEncoding.DecodeString(parts[1])

	// 先验签再解析，签名不匹配即拒绝。
	mac := hmac.New(sha256.New, cursorSecret())
	mac.Write(b)
	if se != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return v, errCode("INVALID_CURSOR", "Cursor signature is invalid.")
	}

	// 校验工具、归属与指纹，确保游标只能用于原查询。
	if e != nil || json.Unmarshal(b, &v) != nil || v.Tool != tool || v.Owner != owner || v.Filter != filter {
		return v, errCode("INVALID_CURSOR", "Cursor does not match this request.")
	}
	return v, nil
}

// fingerprint 对请求参数做摘要，用于游标绑定。
func fingerprint(v any) string { b, _ := json.Marshal(v); return security.Hash(string(b)) }
