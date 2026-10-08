package zitadel

import (
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"isekai-mcp-gateway/internal/auth/domain"
)

type authRequest struct{ *domain.Interaction }

func (a *authRequest) GetCodeChallenge() *oidc.CodeChallenge {
	return &oidc.CodeChallenge{Challenge: a.Request.CodeChallenge, Method: oidc.CodeChallengeMethod(a.Request.CodeChallengeMethod)}
}
func (a *authRequest) GetResponseType() oidc.ResponseType {
	return oidc.ResponseType(a.Request.ResponseType)
}
func (a *authRequest) GetResponseMode() oidc.ResponseMode {
	return oidc.ResponseMode(a.Request.ResponseMode)
}
