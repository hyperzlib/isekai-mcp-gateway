package middleware

import "net/http"

// SecurityHeaders applies the default CSP and framing policy to protected routes.
// Page handlers may override CSP with their specific resource and form sources.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
