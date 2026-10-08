package web

import (
	"slices"
	"testing"

	"isekai-mcp-gateway/internal/auth/domain"
)

func TestConsentScopeGroups(t *testing.T) {
	basic := domain.BasicScopes()
	got := expandConsentScopes([]string{"basic", "mcp:access", "wiki:edit"}, basic)
	want := append(slices.Clone(basic), "mcp:access", "wiki:edit")
	if !slices.Equal(got, want) {
		t.Fatalf("expanded %v", got)
	}
	views := consentScopeViews([]string{"openid", "offline_access"}, []string{"mcp:access", "wiki:read", "future:tool"}, []string{"wiki:read"})
	if len(views) != 4 || views[0].Name != "basic" || !views[0].Checked || views[3].Name != "future:tool" {
		t.Fatalf("views %+v", views)
	}
	// mcp:access 恒选中，配置的默认 scope 选中，其余不选中。
	if !views[1].Checked || !views[2].Checked || views[3].Checked {
		t.Fatalf("default selection %+v", views)
	}
	if !slices.Equal(expandConsentScopes(nil, basic), basic) {
		t.Fatal("requested basic scopes omitted from consent")
	}
	got = expandConsentScopes([]string{"basic"}, []string{"openid"})
	if !slices.Equal(got, []string{"openid"}) {
		t.Fatal("basic exceeds requested scopes")
	}
}

func TestConsentAutomaticallyIncludesRequestedBasicScopes(t *testing.T) {
	for _, tt := range []struct {
		name                      string
		selected, requested, want []string
	}{
		{"disabled checkbox omitted", []string{"wiki:read"}, []string{"mcp:access", "offline_access", "profile"}, []string{"offline_access", "profile", "wiki:read"}},
		{"no form scopes", nil, []string{"openid", "offline_access"}, []string{"openid", "offline_access"}},
		{"no automatic custom consent", nil, []string{"wiki:delete"}, nil},
		{"forged basic scope", []string{"basic", "offline_access", "email", "wiki:read"}, []string{"openid"}, []string{"openid", "wiki:read"}},
		{"duplicate scopes", []string{"basic", "profile", "wiki:read", "wiki:read"}, []string{"profile", "profile"}, []string{"profile", "wiki:read"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandConsentScopes(tt.selected, tt.requested); !slices.Equal(got, tt.want) {
				t.Fatalf("consent scopes = %v, want %v", got, tt.want)
			}
		})
	}
}
