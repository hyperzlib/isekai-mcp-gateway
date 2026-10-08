package domain

type LoginTransaction struct {
	ID            string
	InteractionID string
	Browser       string
	Nonce         string
	Verifier      string
	// The encrypted snapshot resumes in-flight login with its original credentials,
	// including after a process restart or an outbound configuration reload.
	Snapshot []byte
}
