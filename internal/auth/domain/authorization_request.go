package domain

import (
	"encoding/json"
	"strings"
)

// SpaceDelimited preserves the authorization-request JSON representation.
// 以空格分隔的字符串序列化 scope、prompt 等字段，兼容 OIDC 的表示方式。
type SpaceDelimited []string

func (s SpaceDelimited) MarshalJSON() ([]byte, error) { return json.Marshal(strings.Join(s, " ")) }
func (s *SpaceDelimited) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = strings.Split(value, " ")
	return nil
}

type AuthorizationRequest struct {
	Scopes              SpaceDelimited  `json:"scope"`
	ResponseType        string          `json:"response_type"`
	ClientID            string          `json:"client_id"`
	RedirectURI         string          `json:"redirect_uri"`
	State               string          `json:"state"`
	Nonce               string          `json:"nonce"`
	ResponseMode        string          `json:"response_mode"`
	Display             string          `json:"display"`
	Prompt              SpaceDelimited  `json:"prompt"`
	MaxAge              *uint           `json:"max_age"`
	UILocales           json.RawMessage `json:"ui_locales"`
	IDTokenHint         string          `json:"id_token_hint"`
	LoginHint           string          `json:"login_hint"`
	ACRValues           SpaceDelimited  `json:"acr_values"`
	CodeChallenge       string          `json:"code_challenge"`
	CodeChallengeMethod string          `json:"code_challenge_method"`
	RequestParam        string
}
type TokenRequest interface{ GetScopes() []string }
