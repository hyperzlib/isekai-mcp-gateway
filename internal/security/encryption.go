package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"

	"isekai-mcp-gateway/internal/failure"
)

// Key 解码标准 base64 并强制要求 32 字节，供 AES-256 使用。
func Key(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, failure.New("CONFIG_KEY_INVALID", "key must contain 32 bytes encoded as standard base64", "security.key", err)
	}
	if len(b) != 32 {
		return nil, errors.New("key must contain 32 bytes encoded as standard base64")
	}
	return b, nil
}

type Vault struct{ aead cipher.AEAD }

// NewVault 构建 AES-GCM 认证加密器。
func NewVault(key []byte) (*Vault, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: a}, nil
}

func (v *Vault) Seal(value any, aad string) ([]byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	// 随机 nonce 置于密文前缀，每次加密必须重新生成。
	n := make([]byte, v.aead.NonceSize())
	if _, err = rand.Read(n); err != nil {
		return nil, err
	}
	return v.aead.Seal(n, n, b, []byte(aad)), nil
}

// Open 从密文前缀取出 nonce，并用相同 aad 验证解密。
func (v *Vault) Open(b []byte, value any, aad string) error {
	n := v.aead.NonceSize()
	if len(b) < n {
		return errors.New("invalid encrypted record")
	}
	p, err := v.aead.Open(nil, b[:n], b[n:], []byte(aad))
	if err != nil {
		return err
	}
	return json.Unmarshal(p, value)
}
