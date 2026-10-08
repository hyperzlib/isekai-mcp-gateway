package security

import (
	"encoding/json"
	"errors"

	"github.com/go-jose/go-jose/v4"

	"isekai-mcp-gateway/internal/failure"
)

// SigningKey validates both inline and environment-sourced private JWKS.
func SigningKey(raw string) (jose.JSONWebKey, error) {
	var keys jose.JSONWebKeySet
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return jose.JSONWebKey{}, failure.New("CONFIG_SIGNING_INVALID", "signing JWKS must contain one private key", "security.signing", err)
	}
	// 必须恰好包含一个有效的私钥 JWK。
	if len(keys.Keys) != 1 {
		return jose.JSONWebKey{}, errors.New("signing JWKS must contain one private key")
	}
	k := keys.Keys[0]
	if !k.Valid() || k.IsPublic() || k.KeyID == "" || k.Use != "sig" {
		return jose.JSONWebKey{}, errors.New("invalid signing JWK")
	}
	// 仅接受三种受支持的签名算法。
	if k.Algorithm != "RS256" && k.Algorithm != "ES256" && k.Algorithm != "EdDSA" {
		return jose.JSONWebKey{}, errors.New("unsupported signing algorithm")
	}
	return k, nil
}
