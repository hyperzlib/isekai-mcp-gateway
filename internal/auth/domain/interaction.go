package domain

import (
	"time"
)

type Interaction struct {
	ID                string
	Request           AuthorizationRequest
	Resource          string
	Browser           string
	AccountID         string
	SessionID         string
	ClientFingerprint string
	GrantID           string
	AuthTime          time.Time
	DoneFlag          bool
	TrustAfter        time.Time
}

func (a *Interaction) GetID() string          { return a.ID }
func (a *Interaction) GetACR() string         { return "" }
func (a *Interaction) GetAMR() []string       { return []string{"oidc"} }
func (a *Interaction) GetAudience() []string  { return []string{a.Resource} }
func (a *Interaction) GetAuthTime() time.Time { return a.AuthTime }
func (a *Interaction) GetClientID() string    { return a.Request.ClientID }
func (a *Interaction) GetNonce() string       { return a.Request.Nonce }
func (a *Interaction) GetRedirectURI() string { return a.Request.RedirectURI }
func (a *Interaction) GetScopes() []string    { return a.Request.Scopes }
func (a *Interaction) GetState() string       { return a.Request.State }
func (a *Interaction) GetSubject() string     { return a.AccountID }
func (a *Interaction) Done() bool             { return a.DoneFlag }
