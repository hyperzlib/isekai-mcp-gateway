package domain

// APIKey 是用户自建访问凭据的存储快照。明文 key 只在创建时返回一次，
// 落库仅保留 secret 哈希；Identity 是创建时的 wiki 身份快照，UserToken
// 随凭据拆分走独立加密。
type APIKey struct {
	ID         string   // 明文 key 的可检索 id 段
	AccountID  string   // 所属账号
	Name       string   // 用户自定义名称
	Scopes     []string // 授权范围，恒含 mcp:access
	Identity   Identity // wiki 身份快照
	SecretHash string   // SHA-256(secret 段) 的十六进制摘要
}
