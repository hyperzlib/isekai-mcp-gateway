package wikicommon

import (
	"context"
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/security"
	"isekai-mcp-gateway/internal/wiki"
	"log/slog"
	"time"
)

type Execute func(context.Context, *wiki.Call, wiki.Input) (wiki.Object, error)

func resolveSchema(value any) *jsonschema.Resolved {
	// 输入输出 schema 由代码内置且固定，编译期无法发现的错误在此直接 panic 暴露。
	b, e := json.Marshal(value)
	if e != nil {
		panic(e)
	}

	var s jsonschema.Schema
	if e = json.Unmarshal(b, &s); e != nil {
		panic(e)
	}

	r, e := s.Resolve(nil)
	if e != nil {
		panic(e)
	}
	return r
}
func Tool(service *wiki.Service, name string, read bool, execute Execute) gatewaymcp.Definition {
	return gatewaymcp.Definition{Kind: "tool", Name: "wiki." + name, Register: func(server *mcp.Server) {
		inputSchema, outputSchema := Schema(name), OutputSchema(name)
		inputResolved, outputResolved := resolveSchema(inputSchema), resolveSchema(outputSchema)
		server.AddTool(&mcp.Tool{Name: "wiki." + name, Description: service.Render("tools/"+name+".md", wiki.Object{"Tool": "wiki." + name}, Description(name)), InputSchema: inputSchema, OutputSchema: outputSchema, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: read, DestructiveHint: boolPointer(name == "commit_edit" || name == "delete_page" || name == "move_page" || name == "commit_upload" || name == "close_edit")}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx = gatewaymcp.StartCall(ctx)
			started := time.Now()

			// 先按已解析的 schema 校验原始 JSON，再做一次类型严格反序列化。
			var raw map[string]any
			if json.Unmarshal(req.Params.Arguments, &raw) != nil || inputResolved.Validate(raw) != nil {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "Invalid tool arguments. Read the tool input schema."}
			}
			var input wiki.Input
			if json.Unmarshal(req.Params.Arguments, &input) != nil {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "Invalid tool argument types."}
			}

			// 建立调用上下文（含鉴权），失败直接返回受控错误。
			call, e := service.Call(ctx, req.Extra, "")
			if e != nil {
				return gatewaymcp.ToolError(ctx, req.Extra, name, service.DescribeError(e)), nil
			}
			out, e := execute(ctx, call, input)

			// 记录不含敏感明文的调用日志，principal 做哈希处理。
			code := ""
			if e != nil {
				code = failure.Describe(e).Code
			}
			slog.DebugContext(ctx, "wiki operation", "request_id", failure.RequestID(ctx), "principal", security.Hash(call.Principal.AccountID+":"+call.Principal.OAuthClientID+":"+call.Principal.GrantID), "tool", "wiki."+name, "edit_id", input.EditID, "revision_id", out["revision_id"], "duration_ms", time.Since(started).Milliseconds(), "error_code", code)

			if e != nil {
				// 令牌无效时主动撤销 Grant，避免失效凭据被继续重试。使用 WithoutCancel 保证清理不受请求取消影响。
				if failure.Describe(e).Code == "INVALID_TOKEN" {
					_ = service.Auth.Store.Database().WithContext(context.WithoutCancel(ctx)).Transaction(func(db application.Database) error { return service.Auth.RevokeGrant(db, call.Principal.GrantID) })
				}
				return gatewaymcp.ToolError(ctx, req.Extra, name, service.DescribeError(e)), nil
			}

			// 返回前将结果按输出 schema 再校验一次，防止契约漂移。
			data, e := json.Marshal(out)
			if e != nil {
				return gatewaymcp.ToolError(ctx, req.Extra, name, e), nil
			}
			var validated any
			if e = json.Unmarshal(data, &validated); e != nil {
				return nil, e
			}
			if e = outputResolved.Validate(validated); e != nil {
				return gatewaymcp.ToolError(ctx, req.Extra, name, failure.New("INTERNAL_ERROR", "Tool result did not satisfy its schema.", "wiki.output", e)), nil
			}

			// 同时以 text 返回序列化结果，兼容只读取 content 的客户端。
			return &mcp.CallToolResult{StructuredContent: out, Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
		})
	}}
}
func boolPointer(v bool) *bool { return &v }
