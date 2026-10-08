package mediawiki

import (
	"isekai-mcp-gateway/internal/auth/domain"
)

type Identity = domain.Identity

func identityError(code string) error {
	message := "无法解析 Wiki 用户，请检查已有 OIDC 账号绑定和 Bridge 配置。"

	// 将 Bridge 错误码映射为可操作的中文提示，未知码归一化
	switch code {
	case "isekai-servicebridge-unknown_key":
		message = "Wiki 未登记 Gateway 使用的 Bridge keyId，请检查两端的 keyId 配置。"
	case "isekai-servicebridge-signature_mismatch":
		message = "Wiki Bridge 签名验证失败，请检查两端的密钥和签名配置。"
	case "isekai-servicebridge-bootstrap_forbidden":
		message = "Wiki Bridge 密钥未启用 allowBootstrapIdentity，无法解析登录身份。"
	case "isekai-servicebridge-oidc_user_not_found":
		message = "该身份尚未绑定已有 Wiki 账号，请先完成 Wiki 账号绑定。"
	case "isekai-servicebridge-oidc_binding_ambiguous":
		message = "该身份绑定了多个 Wiki 账号，请修正重复绑定。"
	case "isekai-servicebridge-oidc_unavailable":
		message = "Wiki OIDC 账号绑定服务不可用，请检查扩展和数据库配置。"
	case "isekai-servicebridge-oidc_user_unavailable":
		message = "已绑定的 Wiki 账号或服务端凭据不可用。"
	default:
		code = "WIKI_IDENTITY_UNAVAILABLE"
	}
	return &domain.IdentityError{Code: code, Message: message}
}
