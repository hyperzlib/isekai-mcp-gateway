package gatewaycli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"

	"isekai-mcp-gateway/internal/security"
)

func TestCLI(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		fail bool
	}{
		{nil, "generate-keys", false},
		{[]string{"serve", "--help"}, "Start the gateway", false},
		{[]string{"bogus"}, "unknown action", true},
		{[]string{"generate-keys", "extra"}, "no positional", true},
		{[]string{"--bad-flag"}, "flag", true},
		{[]string{"--check-config", "--write-schema"}, "cannot be combined", true},
		{[]string{"--check-config", "generate-keys"}, "explicit action", true},
		{[]string{"generate-keys", "--write-schema"}, "explicit action", true},
		{[]string{"--config-dir", "missing", "--data-dir", "missing", "generate-keys"}, "cookieSecret", false},
		{[]string{"generate-keys", "--config-dir", "missing", "--data-dir", "missing"}, "cookieSecret", false},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			err := Run(context.Background(), append([]string{"gateway"}, tc.args...), &out, &stderr)
			if (err != nil) != tc.fail {
				t.Fatalf("unexpected error: %v", err)
			}
			result := out.String()
			if err != nil {
				result = err.Error()
			}
			if !strings.Contains(result, tc.want) {
				t.Fatalf("unexpected result: %s", result)
			}
			if !tc.fail && stderr.Len() != 0 {
				t.Fatal("unexpected stderr")
			}
		})
	}
}

func TestGenerateKeysOffline(t *testing.T) {
	dir := t.TempDir()
	changeDirectory(t, dir)
	var previous string
	for i := 0; i < 2; i++ {
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), []string{"gateway", "generate-keys"}, &out, &stderr); err != nil {
			t.Fatal(err)
		}
		if stderr.Len() != 0 || !strings.HasSuffix(out.String(), "\n") {
			t.Fatal("invalid output streams")
		}
		var keys security.GeneratedKeys
		if err := json.Unmarshal(out.Bytes(), &keys); err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{keys.CookieSecret, keys.StorageEncryptionKey, keys.ServiceBridgeSecret} {
			if _, err := security.Key(value); err != nil {
				t.Fatal(err)
			}
		}
		// 三类密钥必须互不相同
		if keys.CookieSecret == keys.StorageEncryptionKey || keys.CookieSecret == keys.ServiceBridgeSecret {
			t.Fatal("keys reused")
		}

		// 生成的签名密钥应能完成 EdDSA 签名并通过验签
		raw, err := json.Marshal(keys.SigningJWKS)
		if err != nil {
			t.Fatal(err)
		}
		key, err := security.SigningKey(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: key}, nil)
		if err != nil {
			t.Fatal(err)
		}
		signed, err := signer.Sign([]byte("test payload"))
		if err != nil {
			t.Fatal(err)
		}
		verified, err := signed.Verify(key.Public())
		if err != nil || string(verified) != "test payload" {
			t.Fatal("signature verification failed", err)
		}
		if previous == out.String() {
			t.Fatal("generation repeated")
		}
		previous = out.String()
	}
	entries, err := os.ReadDir(filepath.Clean(dir))
	if err != nil || len(entries) != 0 {
		t.Fatal("generation wrote files", err)
	}
}

func TestLegacyDispatch(t *testing.T) {
	changeDirectory(t, t.TempDir())
	if err := os.Mkdir("docs", 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"check-config"}, {"--check-config"}} {
		var out, stderr bytes.Buffer
		err := Run(context.Background(), append([]string{"gateway"}, args...), &out, &stderr)
		if err == nil || !strings.Contains(err.Error(), "config") {
			t.Fatalf("expected configuration validation: %v", err)
		}
	}
	for _, args := range [][]string{{"write-schema"}, {"--write-schema"}} {
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), append([]string{"gateway"}, args...), &out, &stderr); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat("docs/gateway-config.server.schema.json"); err != nil {
			t.Fatal(err)
		}
	}
}

func changeDirectory(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
}
