package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"

	"github.com/go-jose/go-jose/v4"
)

type GeneratedKeys struct {
	CookieSecret         string             `json:"cookieSecret"`
	StorageEncryptionKey string             `json:"storageEncryptionKey"`
	SigningJWKS          jose.JSONWebKeySet `json:"signingJwks"`
	ServiceBridgeSecret  string             `json:"serviceBridgeSecret"`
}

func GenerateKeys() (GeneratedKeys, error) {
	var result GeneratedKeys
	// 三个对称密钥均为 32 字节随机值，以 base64 存储。
	for _, dst := range []*string{&result.CookieSecret, &result.StorageEncryptionKey, &result.ServiceBridgeSecret} {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return GeneratedKeys{}, err
		}
		*dst = base64.StdEncoding.EncodeToString(b)
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return GeneratedKeys{}, err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return GeneratedKeys{}, err
	}
	// 签名密钥使用 Ed25519，keyId 随机生成，避免跨环境重复。
	result.SigningJWKS = jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: private, KeyID: base64.RawURLEncoding.EncodeToString(id), Algorithm: "EdDSA", Use: "sig"}}}
	return result, nil
}
