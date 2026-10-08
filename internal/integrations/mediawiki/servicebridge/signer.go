package servicebridge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"isekai-mcp-gateway/internal/security"
)

// Sign follows the local Bridge's RFC 9421 bootstrap contract. The raw query and
// exact encoded body must remain unchanged after signing.
func Sign(r *http.Request, body []byte, keyID, secret string) error {
	// 校验 keyId，拒绝引号/反斜杠/换行注入与超长
	if strings.ContainsAny(keyID, "\"\\\r\n") || len(keyID) > 128 {
		return errors.New("invalid Bridge keyId")
	}

	// 对请求体计算 sha-256 Content-Digest
	h := sha256.Sum256(body)
	r.Header.Set("Content-Digest", "sha-256=:"+base64.StdEncoding.EncodeToString(h[:])+":")

	// 根据用户身份与 multipart 摘要选择参与签名的组件
	identity, digest := "x-isekai-subject", "content-digest"
	if r.Header.Get("X-Isekai-User-Id") != "0" {
		identity = "x-isekai-user-token"
	}
	if r.Header.Get("X-Isekai-Multipart-Digest") != "" {
		digest = "x-isekai-multipart-digest"
		r.Header.Del("Content-Digest")
	}

	// 组装签名组件与取值，Content-Type 非空时一并纳入签名
	components := []string{"@method", "@path", "@query", "x-isekai-user-id", identity, digest}
	values := []string{r.Method, r.URL.EscapedPath(), "?" + r.URL.RawQuery, r.Header.Get("X-Isekai-User-Id"), r.Header.Get(identity), r.Header.Get(digest)}
	if r.Header.Get("Content-Type") != "" {
		components = append(components, "content-type")
		values = append(values, r.Header.Get("Content-Type"))
	}
	var names, lines []string
	for i, n := range components {
		names = append(names, strconv.Quote(n))
		lines = append(lines, strconv.Quote(n)+": "+values[i])
	}

	// 构造 RFC 9421 签名参数并生成随机 nonce
	params := "(" + strings.Join(names, " ") + ");created=" + strconv.FormatInt(time.Now().Unix(), 10) + ";nonce=" + strconv.Quote(security.Random()) + ";keyid=" + strconv.Quote(keyID) + ";alg=\"hmac-sha256\""
	lines = append(lines, "\"@signature-params\": "+params)

	// 对签名基串计算 HMAC-SHA256 并写入 Signature 头
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.Join(lines, "\n")))
	r.Header.Set("Signature-Input", "sig1="+params)
	r.Header.Set("Signature", "sig1=:"+base64.StdEncoding.EncodeToString(mac.Sum(nil))+":")
	return nil
}
