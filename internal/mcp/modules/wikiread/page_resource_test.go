package wikiread

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"isekai-mcp-gateway/internal/wiki"
	"strconv"
	"testing"
)

func TestSDKPageTitleResourceRouting(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	for _, def := range Module(nil) {
		if def.Kind != "template" {
			continue
		}
		server.AddResourceTemplate(&mcp.ResourceTemplate{Name: def.Name, URITemplate: def.Key}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			i, err := wiki.PageResourceInput(req.Params.URI, def.Key)
			if err != nil {
				return nil, err
			}
			if i.RevisionID > 0 {
				i.Title = strconv.FormatInt(i.RevisionID, 10)
			}
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: i.Title}}}, nil
		})
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	templates, err := cs.ListResourceTemplates(ctx, nil)
	if err != nil || len(templates.ResourceTemplates) != 18 {
		t.Fatalf("templates: %#v %v", templates, err)
	}
	for _, title := range []string{"帮助:编辑/章节", "123", "A%2FB", "A B+C"} {
		for _, suffix := range []string{"", "/baseline", "/history", "/backlinks", "/structure", "/template-data", "/dependencies", "/transclusions"} {
			uri := wiki.PageResourceLinks(title, 123, suffix)["by_title"].(string)
			r, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
			if err != nil {
				t.Fatalf("%q: %v", uri, err)
			}
			if len(r.Contents) != 1 || r.Contents[0].Text != title {
				t.Fatalf("%q: %#v", title, r)
			}
		}
	}
	for _, suffix := range []string{"/structure", "/dependencies"} {
		uri := "wiki://revisions/10" + suffix
		out, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if err != nil || out.Contents[0].Text != "10" {
			t.Fatalf("revision resource: %#v %v", out, err)
		}
	}
}
