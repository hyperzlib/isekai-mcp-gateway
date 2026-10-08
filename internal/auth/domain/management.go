package domain

type ManagementEntry struct {
	ID, ClientID, ClientName, Scope, Expiry string
	LogoURL                                 *string
	TrustLevel                              TrustLevel
}
