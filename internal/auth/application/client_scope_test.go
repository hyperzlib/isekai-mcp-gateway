package application

import (
	"slices"
	"strings"
	"testing"
)

func TestClientDefaultScopes(t *testing.T) {
	for _, tc := range []struct {
		name            string
		public, refresh bool
		scope           string
		want            []string
	}{
		{"dcr_refresh", true, true, "", append([]string{"mcp:access"}, BasicScopes()...)},
		{"dcr_code_only", true, false, "", []string{"mcp:access", "openid", "profile", "email"}},
		{"explicit_scope", true, true, "mcp:access", []string{"mcp:access"}},
		{"preconfigured", false, true, "", []string{"mcp:access"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Client{Redirects: []string{"https://chatgpt.com/connector_platform_oauth_redirect"}, Scope: tc.scope, Grants: []string{"authorization_code"}}
			if tc.refresh {
				c.Grants = append(c.Grants, "refresh_token")
			}
			if err := new(Service).validateClient(&c, tc.public); err != nil {
				t.Fatal(err)
			}
			if got := strings.Fields(c.Scope); !slices.Equal(got, tc.want) {
				t.Fatalf("scopes = %v, want %v", got, tc.want)
			}
			if tc.name == "dcr_refresh" {
				requested := []string{"openid", "email", "offline_access"}
				if !RequestedScopesAllowed(requested, strings.Fields(c.Scope)) {
					t.Fatal("ChatGPT scopes rejected")
				}
				consent := ConsentScopes(requested, strings.Fields(c.Scope))
				if slices.Contains(consent, "profile") {
					t.Fatal("unrequested basic scope added to consent")
				}
			}
		})
	}
}
