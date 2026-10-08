package application

import (
	"isekai-mcp-gateway/internal/auth/domain"
)

var BasicScopes = domain.BasicScopes
var BusinessScopes = domain.BusinessScopes
var ConsentScopes = domain.ConsentScopes
var RequestedScopesAllowed = domain.RequestedScopesAllowed

func NormalizeScopes(raw, allowed []string) ([]string, error) {
	return domain.NormalizeScopes(raw, allowed)
}
func Subset(a, b []string) bool { return domain.Subset(a, b) }
