package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestSecurityHeadersRouteScopeAndOverrides(t *testing.T) {
	router := chi.NewRouter()
	serve := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }
	router.Get("/", serve)
	router.Get("/assets/*", serve)
	router.Group(func(r chi.Router) {
		r.Use(SecurityHeaders)
		for _, path := range []string{"/oauth/token", "/mcp", "/.well-known/oauth-protected-resource", "/health/live", "/uploads"} {
			r.HandleFunc(path, serve)
		}
		forms := chi.NewRouter()
		forms.Get("/login", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Referrer-Policy", "same-origin")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; form-action https://idp.example; frame-ancestors 'none'")
			serve(w, r)
		})
		r.Mount("/auth", forms)
	})
	handler := Headers(router)
	for _, path := range []string{"/", "/assets/home/home.js", "/oauth/token", "/mcp", "/.well-known/oauth-protected-resource", "/health/live", "/uploads", "/auth/login"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			if w.Code != http.StatusNoContent || w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("unexpected response: %d, %v", w.Code, w.Header())
			}
			protected := path != "/" && path != "/assets/home/home.js"
			if (w.Header().Get("Content-Security-Policy") != "") != protected || (w.Header().Get("X-Frame-Options") == "DENY") != protected {
				t.Fatalf("incorrect security header scope: %v", w.Header())
			}
			wantReferrer := "strict-origin-when-cross-origin"
			if path == "/auth/login" {
				wantReferrer = "same-origin"
				if w.Header().Get("Content-Security-Policy") != "default-src 'self'; form-action https://idp.example; frame-ancestors 'none'" {
					t.Fatal("page CSP override lost")
				}
			}
			if w.Header().Get("Referrer-Policy") != wantReferrer {
				t.Fatalf("unexpected referrer policy: %v", w.Header())
			}
		})
	}
}
