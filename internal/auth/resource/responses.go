package resource

import (
	"net/http"

	"isekai-mcp-gateway/internal/httpresponse"
)

func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	httpresponse.JSON(r.Context(), w, status, value)
}
