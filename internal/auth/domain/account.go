package domain

type Account struct {
	ID          string
	Issuer      string
	Subject     string
	SiteID      string
	WikiUserID  int64
	Username    string
	DisplayName string
	// AvatarURL 是 IDP 侧头像地址，非凭据，按明文 payload 存储。
	AvatarURL string
}
