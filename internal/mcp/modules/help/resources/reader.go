package resources

import (
	"isekai-mcp-gateway/internal/mcp/modules/help/domain"
)

type Reader interface {
	Read(string) (domain.Result, error)
}
type Result = domain.Result
