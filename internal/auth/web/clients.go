package web

import (
	"net/http"
)

func (s *Service) Clients(w http.ResponseWriter, r *http.Request) {
	s.management(w, r, "client_trusts", "clients", "管理已信任的应用")
}
