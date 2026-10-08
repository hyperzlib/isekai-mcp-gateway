package zitadel

import (
	"net/http"
)

func (s *Service) Revocation(w http.ResponseWriter, r *http.Request) {
	s.providerResponse(w, r, false)
}
