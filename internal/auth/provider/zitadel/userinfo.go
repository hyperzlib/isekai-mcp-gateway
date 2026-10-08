package zitadel

import (
	"net/http"
)

func (s *Service) Userinfo(w http.ResponseWriter, r *http.Request) {
	s.providerResponse(w, r, false)
}
