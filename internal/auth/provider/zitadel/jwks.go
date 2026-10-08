package zitadel

import (
	"net/http"
)

func (s *Service) JWKS(w http.ResponseWriter, r *http.Request) {
	s.Provider.ServeHTTP(w, r)
}
