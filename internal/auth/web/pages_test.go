package web

import (
	"bytes"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/templates"
)

func TestBuiltinAuthPages(t *testing.T) {
	m, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for mode, marker := range map[string]string{"login": "登录并继续", "account": "切换用户", "trust": "信任此应用并继续", "consent": "同意并连接", "error": "重新发起连接", "grants": "撤销授权", "clients": "取消信任", "apikeys": "创建 API Key"} {
		var out bytes.Buffer
		data := pageData{Mode: mode, Title: "<title>", Message: "<message>", CSRF: "csrf-test", Interaction: "interaction", ClientName: "Test app", Username: "Test user", Countdown: 5, Scopes: []scopeView{{Name: "mcp:access", Label: "连接", Checked: true}}, Entries: []entryView{{ID: "entry", ClientName: "Test app"}}, LoggedIn: mode != "login", Wide: mode == "grants" || mode == "clients" || mode == "apikeys", KeyEnabled: true, Initial: "T", ReturnTo: "/auth/grants", AllowPermanent: true, MaxPerUser: 10, TTLOptions: []ttlView{{Name: "7 天", Value: "7", Days: 7}}}
		if err := m.RenderHTML(&out, mode, data); err != nil {
			t.Fatal(mode, err)
		}
		body := out.String()
		for _, want := range []string{"<!doctype html>", "/assets/auth/style.css", "/assets/auth/auth.js", "&lt;title&gt;", marker} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q", mode, want)
			}
		}
		if mode != "error" && !strings.Contains(body, `value="csrf-test"`) {
			t.Fatal(mode, "missing CSRF field")
		}
		// 布局导航：登录态显示登出入口，未登录时导航为空。
		if data.LoggedIn && data.Wide && !strings.Contains(body, "退出登录") {
			t.Fatal(mode, "missing layout navigation")
		}
	}
}

func TestAccountPageVariants(t *testing.T) {
	m, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// 未登录视图：渲染登录按钮而非用户信息。
	if err := m.RenderHTML(&out, "account", pageData{Mode: "account", Title: "应用请求连接", CSRF: "csrf", Interaction: "i", ClientName: "App"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "登录并继续") || strings.Contains(out.String(), "切换用户") {
		t.Fatalf("unlogged account view: %s", out.String())
	}
	out.Reset()
	// 可沿用视图：渲染继续/更改授权与上次授权摘要。
	if err := m.RenderHTML(&out, "account", pageData{Mode: "account", Title: "应用请求连接", CSRF: "csrf", Interaction: "i", ClientName: "App", LoggedIn: true, Username: "U", Initial: "U", Summary: "连接到本服务、查看 Wiki 内容"}); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{`name="action" value="continue"`, `name="action" value="change"`, `name="action" value="switch"`, "上次授权"} {
		if !strings.Contains(body, want) {
			t.Fatalf("reusable account view missing %q", want)
		}
	}
}

func TestAPIKeysPageOneTimeKey(t *testing.T) {
	m, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := m.RenderHTML(&out, "apikeys", pageData{Mode: "apikeys", Title: "API Keys", CSRF: "csrf", CreatedKey: "sk-demo-secret", CreatedName: "desktop", KeyEnabled: true, AllowPermanent: true, MaxPerUser: 10}); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if !strings.Contains(body, "关闭此页面后将无法再次查看") || !strings.Contains(body, "sk-demo-secret") {
		t.Fatalf("one-time key display: %s", body)
	}
}

func TestErrorPageRetry(t *testing.T) {
	m, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// 可重试错误：渲染 POST 回发起登录端点的重试按钮。
	if err := m.RenderHTML(&out, "error", pageData{Mode: "error", Title: "操作未完成", Message: "登录换码失败", CSRF: "csrf", RetryURL: "/auth/interaction/abc"}); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{`action="/auth/interaction/abc"`, `name="action" value="login"`, "重试", `value="csrf"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("retryable error page missing %q", want)
		}
	}
	out.Reset()
	// 不可重试错误：不渲染重试按钮。
	if err := m.RenderHTML(&out, "error", pageData{Mode: "error", Title: "操作未完成", Message: "登录事务已过期"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "重试") {
		t.Fatal("non-retryable error page rendered retry")
	}
}
