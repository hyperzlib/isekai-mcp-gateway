package config

import (
	"crypto/x509"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTLSCertificateConfiguration(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	dir := t.TempDir()
	certificate := server.TLS.Certificates[0]
	key, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	// 将测试服务器的证书与私钥写成 PEM 文件供校验读取。
	for name, block := range map[string]*pem.Block{
		"cert.pem": {Type: "CERTIFICATE", Bytes: certificate.Certificate[0]},
		"key.pem":  {Type: "PRIVATE KEY", Bytes: key},
	} {
		if err := os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(block), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name, url, cert, key string
		valid                bool
	}{
		{"valid pair", "https://localhost:3000", "cert.pem", "key.pem", true},
		{"HTTP public URL", "http://localhost:3000", "cert.pem", "key.pem", false},
		{"missing key", "https://localhost:3000", "cert.pem", "", false},
		{"invalid PEM", "https://localhost:3000", "key.pem", "key.pem", false},
		{"unreadable file", "https://localhost:3000", "missing.pem", "key.pem", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := Defaults()
			c.Dir = dir
			c.Static.Server.PublicURL = test.url
			c.Static.Server.TLS = ServerTLS{Enabled: true, CertFile: test.cert, KeyFile: test.key}
			if err := c.validateTLS(); (err == nil) != test.valid {
				t.Fatalf("certificate validation: %v", err)
			}
			if test.valid && !filepath.IsAbs(c.Static.Server.TLS.CertFile) {
				t.Fatal("certificate path was not resolved against configuration directory")
			}
		})
	}
}
