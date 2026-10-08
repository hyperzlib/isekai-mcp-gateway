package domain

import (
	"time"
)

type Token struct {
	ID            string
	GrantID       string
	Subject       string
	ClientID      string
	Resource      string
	Scopes        []string
	AuthTime      time.Time
	IdleExpiresAt time.Time
}

func (t *Token) GetSubject() string               { return t.Subject }
func (t *Token) GetClientID() string              { return t.ClientID }
func (t *Token) GetAudience() []string            { return []string{t.Resource} }
func (t *Token) GetScopes() []string              { return t.Scopes }
func (t *Token) GetAMR() []string                 { return []string{"oidc"} }
func (t *Token) GetAuthTime() time.Time           { return t.AuthTime }
func (t *Token) SetCurrentScopes(scopes []string) { t.Scopes = scopes }
