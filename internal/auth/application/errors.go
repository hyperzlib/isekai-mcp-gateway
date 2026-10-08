package application

import (
	"errors"

	"isekai-mcp-gateway/internal/failure"
)

var ErrInvalidToken = failure.ErrInvalidToken
var ErrAuthenticationRequired = failure.ErrAuthenticationRequired

// lookupError maps absent or invalid records to the caller's business failure,
// preserving infrastructure failures from the repository or payload decoder.
func lookupError(err, invalid error) error {
	if errors.Is(err, ErrRecordNotFound) {
		return invalid
	}
	if f := failure.Known(err); f != nil && !failure.IsInfrastructure(err) {
		return invalid
	}
	return failure.Wrap("auth.lookup", err)
}
func invalidMetadata(message string) error {
	return failure.New(failure.InvalidMetadata, message, "client.validate", nil)
}
