package security

import (
	"crypto/sha256"
	"encoding/hex"
)

func Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
