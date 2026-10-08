package application

import (
	"isekai-mcp-gateway/internal/auth/domain"
)

// StoredClient includes internal fields that never appear in registration responses.
type storedClient = domain.StoredClient

type Client = domain.Client
type Interaction = domain.Interaction
type Token = domain.Token
type Account = domain.Account
type Session = domain.Session
type LoginTransaction = domain.LoginTransaction
type Grant = domain.Grant
type Code = domain.Code
type ClientTrust = domain.ClientTrust
type Principal = domain.Principal
