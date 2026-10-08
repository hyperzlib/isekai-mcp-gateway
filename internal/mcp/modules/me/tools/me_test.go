package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"isekai-mcp-gateway/internal/auth/domain"
)

type userReader struct{ err error }

func (r *userReader) MCPCurrentUser(context.Context, *mcp.RequestExtra) (domain.UserInfo, error) {
	return domain.UserInfo{UserID: 123, Username: "Example", DisplayName: "示例用户"}, r.err
}

func TestMeSDK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reader := new(userReader)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	Tool(reader).Register(server)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "initialize-client", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, override := range []bool{false, true} {
		params := &mcp.CallToolParams{Name: "wiki.me", Arguments: map[string]any{}}
		want := "initialize-client"
		if override {
			params.Meta = mcp.Meta{mcp.MetaKeyClientInfo: &mcp.Implementation{Name: "request-client", Version: "2", Title: "Request"}}
			want = "request-client"
		}
		result, err := cs.CallTool(ctx, params)
		if err != nil || result.IsError {
			t.Fatalf("call: %v, %+v", err, result)
		}
		b, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out Output
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		if out.User.UserID != 123 || out.ClientInfo == nil || out.ClientInfo.Name != want {
			t.Fatalf("output: %s", b)
		}
		if override && out.ClientInfo.Title != "Request" {
			t.Fatal("optional clientInfo fields must be preserved")
		}
		text := result.Content[0].(*mcp.TextContent).Text
		var textOut Output
		if err := json.Unmarshal([]byte(text), &textOut); err != nil || textOut.User != out.User {
			t.Fatal("text and structured content differ")
		}
		if strings.Contains(string(b), "user_token") || strings.Contains(string(b), "subject") {
			t.Fatal("sensitive fields leaked")
		}
	}
	reader.err = errors.New("secret backend payload")
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wiki.me", Arguments: map[string]any{}})
	if err != nil || !result.IsError || strings.Contains(result.Content[0].(*mcp.TextContent).Text, "secret") {
		t.Fatal("error must be controlled and redacted")
	}
}

func TestMissingClientInfo(t *testing.T) {
	req := new(mcp.CallToolRequest)
	b, err := json.Marshal(Output{ClientInfo: req.ClientInfo()})
	if err != nil || !strings.Contains(string(b), `"clientInfo":null`) {
		t.Fatalf("missing client info: %s, %v", b, err)
	}
}
