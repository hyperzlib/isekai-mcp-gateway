package web

import (
	"html/template"
	"net/http/httptest"
	"testing"
)

func TestMalformedOIDCCallbackDoesNotConsumeTransaction(t *testing.T) {
	// No application service: malformed input must fail before reaching storage.
	s := &Service{Pages: testPages{template.Must(template.New("page").Parse("{{.Message}}"))}}
	for _, query := range []string{
		"code=a", "state=&code=a", "state=s", "state=s&code=a&error=denied",
		"state=s&state=t&code=a", "state=s&code=a&code=b", "state=s&error=a&error=b",
	} {
		w := httptest.NewRecorder()
		s.OidcConnectCallback(w, httptest.NewRequest("GET", "https://gateway.example/auth/oidc_client/callback?"+query, nil))
		if w.Code != 400 {
			t.Fatalf("%s: status %d", query, w.Code)
		}
	}
}
