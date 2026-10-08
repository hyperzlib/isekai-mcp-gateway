package mysql

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
)

func TestRepositoryErrorNormalization(t *testing.T) {
	if err := repositoryError("store.get", gorm.ErrRecordNotFound); !errors.Is(err, application.ErrRecordNotFound) {
		t.Fatal("not-found contract changed")
	}
	secret := errors.New("secret corrupted encrypted payload")
	err := repositoryError("store.get.accounts", secret)
	if !errors.Is(err, secret) || failure.Describe(err).Code != failure.Internal || strings.Contains(err.Error(), "secret") {
		t.Fatal("payload corruption was masked or exposed")
	}
	err = repositoryError("store.get", context.DeadlineExceeded)
	if failure.HTTPStatus(err) != 503 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("database timeout misclassified")
	}
}
