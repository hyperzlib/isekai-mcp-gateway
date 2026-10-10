package wikiactivity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikiread"
	"isekai-mcp-gateway/internal/wiki"
)

func TestActivityResourceRoutes(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	for _, def := range Module(nil) {
		if def.Kind == "tool" {
			continue
		}
		operation := "get_user_contributions"
		if def.Key == "wiki://me/contributions" {
			operation = "get_my_contributions"
		}
		if def.Key == "wiki://recent-changes" {
			operation = "get_recent_changes"
		}
		handler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			i, err := wiki.ReadResourceInput(req.Params.URI, def.Key, operation)
			if err != nil {
				return nil, err
			}
			data, _ := json.Marshal(i)
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: string(data)}}}, nil
		}
		if def.Kind == "resource" {
			server.AddResource(&mcp.Resource{Name: def.Name, URI: def.Key}, handler)
		} else {
			server.AddResourceTemplate(&mcp.ResourceTemplate{Name: def.Name, URITemplate: def.Key}, handler)
		}
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, tt := range []struct {
		uri, username string
		id            int64
	}{
		{"wiki://me/contributions", "", 0}, {"wiki://recent-changes", "", 0},
		{"wiki://users/by-id/17/contributions", "", 17},
		{"wiki://users/by-name/%E6%B5%8B%E8%AF%95%2F%E7%94%A8%E6%88%B7/contributions", "测试/用户", 0},
		{"wiki://users/by-name/A%252FB/contributions", "A%2FB", 0},
	} {
		out, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: tt.uri})
		if err != nil {
			t.Fatalf("%s: %v", tt.uri, err)
		}
		var input wiki.Input
		if err := json.Unmarshal([]byte(out.Contents[0].Text), &input); err != nil {
			t.Fatal(err)
		}
		if input.Username != tt.username || input.UserID != tt.id {
			t.Fatalf("%s: %#v", tt.uri, input)
		}
	}
	for _, uri := range []string{"wiki://users/by-id/0/contributions", "wiki://users/by-name/A%7CB/contributions", "wiki://me/contributions?user_id=17"} {
		if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Fatalf("accepted %s", uri)
		}
	}
}

func TestReadModuleRegistration(t *testing.T) {
	ctx := context.Background()
	service := &wiki.Service{}
	defs := append(wikiread.Module(service), Module(service)...)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	if err := gatewaymcp.Register(server, defs); err != nil {
		t.Fatal(err)
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	tools, err := cs.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 19 {
		t.Fatalf("tools: %#v %v", tools, err)
	}
	foundSubpages := false
	foundResolveURL := false
	for _, tool := range tools.Tools {
		if tool.Name == "wiki.resolve_page_url" {
			foundResolveURL = true
		}
		if tool.Name == "wiki.list_subpages" {
			foundSubpages = true
		}
		if tool.InputSchema == nil || tool.OutputSchema == nil || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("invalid read tool: %#v", tool)
		}
	}
	if !foundSubpages {
		t.Fatal("list_subpages was not discovered by the official MCP client")
	}
	if !foundResolveURL {
		t.Fatal("resolve_page_url was not discovered by the official MCP client")
	}
	templates, err := cs.ListResourceTemplates(ctx, nil)
	if err != nil || len(templates.ResourceTemplates) != 20 {
		t.Fatalf("templates: %#v %v", templates, err)
	}
	resources, err := cs.ListResources(ctx, nil)
	if err != nil || len(resources.Resources) != 2 {
		t.Fatalf("resources: %#v %v", resources, err)
	}
	// 非法参数必须在鉴权和上游请求之前被协议 schema 拒绝。
	for _, tt := range []struct {
		name string
		args map[string]any
	}{
		{"wiki.get_my_contributions", map[string]any{"user_id": float64(17)}},
		{"wiki.get_page_structure", map[string]any{"title": "A", "page_id": float64(3)}},
		{"wiki.list_subpages", map[string]any{"title": "A", "page_id": float64(3)}},
		{"wiki.list_subpages", map[string]any{"title": "A", "limit": float64(0)}},
		{"wiki.list_subpages", map[string]any{"title": "A", "limit": float64(101)}},
		{"wiki.list_subpages", map[string]any{"revision_id": float64(3)}},
		{"wiki.resolve_page_url", map[string]any{}},
		{"wiki.resolve_page_url", map[string]any{"url": ""}},
		{"wiki.resolve_page_url", map[string]any{"url": "/首页", "title": "首页"}},
	} {
		if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args}); err == nil {
			t.Fatalf("accepted %s invalid parameters", tt.name)
		}
	}
}
