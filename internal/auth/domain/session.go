package domain

type Session struct {
	ID        string
	AccountID string
	SiteID    string
	Browser   string
	Identity  Identity
}
