package zitadel

import (
	"encoding/json"
	"net/http"
)

func (s *Service) Discovery(w http.ResponseWriter, r *http.Request) {
	// Start with provider metadata, then publish only the Gateway adaptations.
	clone := r.Clone(r.Context())
	clone.URL.Path = "/.well-known/openid-configuration"
	b := newResponseBuffer()
	s.Provider.ServeHTTP(b, clone)
	var doc map[string]any
	if b.status != 200 || json.Unmarshal(b.body.Bytes(), &doc) != nil {
		writeJSON(w, r, 503, map[string]string{"error": "metadata_unavailable"})
		return
	}
	// 在 provider 元数据之上覆盖网关特有的能力声明
	g := s.Config.Static.Auth.Gateway
	doc["client_id_metadata_document_supported"] = g.Registration.CIMD
	doc["authorization_response_iss_parameter_supported"] = true
	doc["code_challenge_methods_supported"] = []string{"S256"}
	doc["token_endpoint_auth_methods_supported"] = []string{"none", "client_secret_basic", "client_secret_post"}
	if g.Registration.Dynamic {
		doc["registration_endpoint"] = g.Issuer + "/oauth/register"
	}
	writeJSON(w, r, 200, doc)
}
