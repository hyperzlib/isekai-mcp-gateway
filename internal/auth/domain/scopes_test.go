package domain

import (
	"slices"
	"testing"
)

func TestRefreshScopeGroups(t *testing.T) {
	// 刷新仅允许收窄基础权限，自定义权限原样继承。
	previous := []string{"openid", "offline_access", "profile", "mcp:access", "wiki:edit", "future:custom"}
	for _, requested := range [][]string{{"openid"}, {"openid", "wiki:delete", "future:other"}, {}} {
		got, err := RefreshScopes(previous, requested)
		want := []string{"mcp:access", "wiki:edit", "future:custom"}
		if len(requested) > 0 {
			want = append([]string{"openid"}, want...)
		}
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("refresh %v: %v %v", requested, got, err)
		}
	}
	if _, err := RefreshScopes(previous, []string{"email"}); err == nil {
		t.Fatal("basic scope escalation accepted")
	}
	got := BasicScopes()
	got[0] = "modified"
	if !IsBasicScope("openid") || IsBasicScope("mcp:access") {
		t.Fatal("classification mutated")
	}
	if !RequestedScopesAllowed([]string{"mcp:access", "wiki:edit", "openid"}, []string{"openid", "mcp:access"}) {
		t.Fatal("custom consent blocked by registration")
	}
}
