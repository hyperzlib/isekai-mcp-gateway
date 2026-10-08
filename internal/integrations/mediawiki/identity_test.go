package mediawiki

import (
	"strings"
	"testing"
)

func TestIdentityErrorsAreControlledAndUseful(t *testing.T) {
	for code, expected := range map[string]string{
		"isekai-servicebridge-unknown_key":            "keyId",
		"isekai-servicebridge-bootstrap_forbidden":    "allowBootstrapIdentity",
		"isekai-servicebridge-oidc_user_not_found":    "绑定",
		"isekai-servicebridge-oidc_binding_ambiguous": "多个",
		"secret-from-untrusted-response":              "无法解析",
	} {
		message := identityError(code).Error()
		if !strings.Contains(message, expected) || strings.Contains(message, "secret-from-untrusted-response") {
			t.Fatalf("unexpected public identity error: %s", message)
		}
	}
}
