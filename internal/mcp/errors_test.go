package gatewaymcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"isekai-mcp-gateway/internal/failure"
)

func TestToolErrorCodesAndInvocationIDs(t *testing.T) {
	parent := failure.NewRequest(context.Background())
	ids := map[string]bool{}
	for _, test := range []struct {
		err       error
		code      string
		retryable bool
	}{
		{failure.ErrInvalidToken, failure.InvalidToken, false},
		{&failure.Error{Code: failure.Unavailable, Message: "服务暂不可用，请稍后重试。", Retryable: true}, failure.Unavailable, true},
		{errors.New("secret SQL and upstream body"), failure.Internal, false},
	} {
		ctx := StartCall(parent)
		result := ToolError(ctx, nil, "wiki.me", test.err)
		text := result.Content[0].(*mcp.TextContent).Text
		var body struct {
			Error toolFailure `json:"error"`
		}
		if err := json.Unmarshal([]byte(text), &body); err != nil {
			t.Fatal(err)
		}
		if !result.IsError || body.Error.Code != test.code || body.Error.Retryable != test.retryable || body.Error.Details == nil || body.Error.RequestID == "" || body.Error.RequestID == failure.RequestID(parent) || ids[body.Error.RequestID] || strings.Contains(text, "secret") {
			t.Fatalf("unsafe tool error: %s", text)
		}
		ids[body.Error.RequestID] = true
	}
}
