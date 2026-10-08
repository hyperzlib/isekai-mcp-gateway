package zitadel

import (
	"encoding/json"
	"net/http"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
)

func (s *Service) Registration(w http.ResponseWriter, r *http.Request) {
	if !s.Config.Static.Auth.Gateway.Registration.Dynamic {
		http.NotFound(w, r)
		return
	}
	var c application.Client
	d := json.NewDecoder(r.Body)
	if err := d.Decode(&c); err != nil {
		writeJSON(w, r, 400, map[string]string{"error": "invalid_client_metadata"})
		return
	}
	if err := s.Register(r.Context(), &c); err != nil {
		failure.Log(r.Context(), "client.register", err)
		status, body := registrationFailure(err)
		writeJSON(w, r, status, body)
		return
	}
	writeJSON(w, r, 201, &c)
}

// registrationFailure 将注册失败原因映射为对外的 HTTP 状态码与 OAuth 错误
func registrationFailure(err error) (int, map[string]string) {
	f := failure.Describe(err)
	switch f.Code {
	case failure.InvalidMetadata:
		return 400, map[string]string{"error": "invalid_client_metadata", "error_description": f.Message}
	case failure.Capacity:
		return 503, map[string]string{"error": "temporarily_unavailable", "error_description": "Client registration capacity reached."}
	default:
		return 500, map[string]string{"error": "server_error", "error_description": "Client registration failed."}
	}
}
