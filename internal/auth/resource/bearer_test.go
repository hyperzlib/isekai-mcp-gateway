package resource

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
)

type bearerDB struct{ application.Database }

func (d *bearerDB) WithContext(context.Context) application.Database { return d }

type bearerStore struct {
	application.RecordStore
	cause error
}

func (s *bearerStore) Database() application.Database { return new(bearerDB) }
func (s *bearerStore) Get(application.Database, string, string, any, bool) (persistence.Row, error) {
	return persistence.Row{}, s.cause
}

type bearerDecoder struct{}

func (bearerDecoder) Decrypt(string) (string, error) { return "token:account", nil }

func TestBearerInfrastructureDoesNotChallenge(t *testing.T) {
	for _, test := range []struct {
		err       error
		status    int
		challenge bool
	}{
		{application.ErrRecordNotFound, 401, true},
		{errors.New("secret backend SQL"), 500, false},
		{&failure.Error{Code: failure.Unavailable, Message: "safe", Retryable: true}, 503, false},
	} {
		s := &Service{Service: &application.Service{Config: config.Defaults(), Store: &bearerStore{cause: test.err}, Decoder: bearerDecoder{}}}
		r := httptest.NewRequest("POST", "/mcp", nil)
		r.Header.Set("Authorization", "Bearer test")
		w := httptest.NewRecorder()
		s.Bearer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("failed bearer reached handler") })).ServeHTTP(w, r)
		if w.Code != test.status || (w.Header().Get("WWW-Authenticate") != "") != test.challenge || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("wrong bearer response: %d %s", w.Code, w.Body.String())
		}
	}
}
