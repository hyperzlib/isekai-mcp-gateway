package wikicommon

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"isekai-mcp-gateway/internal/failure"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/security"
	"isekai-mcp-gateway/internal/wiki"
	"strings"
	"time"
)

func Resource(service *wiki.Service, name, uri, mimeType, operation string) gatewaymcp.Definition {
	// 固定 URI 注册为 resource，含占位符的注册为 template。
	kind := "template"
	if !strings.Contains(uri, "{") {
		kind = "resource"
	}
	return gatewaymcp.Definition{Kind: kind, Name: name, Key: uri, Register: func(server *mcp.Server) {
		handler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			ctx = gatewaymcp.StartCall(ctx)
			c, x := service.Call(ctx, req.Extra, "wiki:read")
			if x != nil {
				return nil, gatewaymcp.ResourceError(ctx, req.Extra, name, x)
			}

			// 页面和用户选择器统一校验编码；编辑资源继续使用会话 ID。
			input := wiki.Input{}
			if wiki.IsPageRead(operation) || wiki.IsActivityRead(operation) {
				input, x = wiki.ReadResourceInput(req.Params.URI, uri, operation)
				if x != nil {
					return nil, gatewaymcp.ResourceError(ctx, req.Extra, name, x)
				}
			} else if operation != "list_edits" {
				id := strings.TrimPrefix(req.Params.URI, "wiki://edit/")
				id = strings.TrimSuffix(id, strings.TrimPrefix(uri, "wiki://edit/{edit_id}"))
				if id == "" || strings.ContainsAny(id, "/?#%") {
					return nil, gatewaymcp.ResourceError(ctx, req.Extra, name, failure.New("NOT_FOUND", "Resource was not found.", "resource", nil))
				}
				input.EditID = id
			}

			// 按操作分派；预览与冲突读取需先取状态或限制分页。
			var out wiki.Object
			switch operation {
			case "get_latest_page", "get_baseline_page", "get_history", "get_backlinks",
				"get_user_contributions", "get_my_contributions", "get_recent_changes",
				"get_page_structure", "get_template_data", "get_page_dependencies", "get_transclusions":
				out, x = service.Read(ctx, c, operation, input)
			case "list_edits":
				out, x = service.List(ctx, c, input)
			case "preview_edit":
				var status wiki.Object
				status, x = service.Get(ctx, c, "get_edit_status", input)
				if x == nil {
					input.BaseVersion = int64(status["version"].(int64))
					out, x = service.Preview(ctx, c, input)
				}
			case "get_edit_content":
				input.Limit = int(c.Config.App.Results.TextMax)
				out, x = service.Get(ctx, c, operation, input)
			case "get_edit_conflict":
				input.Limit = 100
				out, x = service.Get(ctx, c, operation, input)
				if x == nil && out["next_cursor"] != nil && out["next_cursor"] != "" {
					x = failure.New("RESOURCE_TOO_LARGE", "Use wiki.get_edit_conflict for paginated reads.", "resource", nil)
				}
			default:
				out, x = service.Get(ctx, c, operation, input)
			}
			if x != nil {
				return nil, gatewaymcp.ResourceError(ctx, req.Extra, name, x)
			}

			// 根据 MIME 类型提取要返回的文本；纯文本资源若被截断则要求改用分页 Tool。
			text := ""
			switch mimeType {
			case "text/plain":
				key := "content"
				if operation == "get_edit_diff" {
					key = "diff"
				}
				text, _ = out[key].(string)
				if truncated, _ := out["truncated"].(bool); truncated {
					return nil, gatewaymcp.ResourceError(ctx, req.Extra, name, failure.New("RESOURCE_TOO_LARGE", "Use the corresponding Tool's paginated read.", "resource", nil))
				}
			case "text/html":
				text, _ = out["html"].(string)
			default:
				b, e := json.Marshal(out)
				if e != nil {
					return nil, e
				}
				text = string(b)
			}

			// HTML 预览使用更宽松的上限，其余资源受文本大小限制约束。
			limit := c.Config.App.Results.TextMax
			if mimeType == "text/html" {
				limit = c.Config.App.Results.PreviewMax
			}
			if int64(len(text)) > limit {
				return nil, gatewaymcp.ResourceError(ctx, req.Extra, name, failure.New("RESOURCE_TOO_LARGE", "Use the corresponding Tool's paginated read.", "resource", nil))
			}

			// Meta 中携带版本、生成时间与内容哈希，供客户端做缓存与变更判断。
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: mimeType, Text: text, Meta: mcp.Meta{"edited_version": out["version"], "generated_at": time.Now().UTC().Format(time.RFC3339Nano), "hash": security.Hash(text)}}}}, nil
		}
		if kind == "resource" {
			server.AddResource(&mcp.Resource{Name: name, URI: uri, MIMEType: mimeType}, handler)
		} else {
			server.AddResourceTemplate(&mcp.ResourceTemplate{Name: name, URITemplate: uri, MIMEType: mimeType}, handler)
		}
	}}
}
func DynamicList(service *wiki.Service) gatewaymcp.Definition {
	return gatewaymcp.Definition{Kind: "middleware", Name: "private-resource-list", Register: func(server *mcp.Server) {
		server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
				// 只接管 resources/list，其他方法交回原处理器。
				if method != "resources/list" {
					return next(ctx, method, request)
				}

				req := request.(*mcp.ListResourcesRequest)
				c, x := service.Call(ctx, req.Extra, "wiki:read")
				if x != nil {
					return nil, gatewaymcp.ResourceError(ctx, req.Extra, method, x)
				}

				// 固定分页大小 20，游标透传上游；无游标时注入静态资源作为首屏。
				i := wiki.Input{Limit: 20}
				if req.Params != nil {
					i.Cursor = req.Params.Cursor
				}
				out, x := service.List(ctx, c, i)
				if x != nil {
					return nil, gatewaymcp.ResourceError(ctx, req.Extra, method, x)
				}
				result := &mcp.ListResourcesResult{}
				if i.Cursor == "" {
					result.Resources = append(result.Resources, &mcp.Resource{Name: "help-index", URI: "wiki://help/index", MIMEType: "text/markdown"}, &mcp.Resource{Name: "edits", URI: "wiki://edits", MIMEType: "application/json"})
					result.Resources = append(result.Resources,
						&mcp.Resource{Name: "my-contributions", URI: "wiki://me/contributions", MIMEType: "application/json"},
						&mcp.Resource{Name: "recent-changes", URI: "wiki://recent-changes", MIMEType: "application/json"})
				}
				for _, e := range out["edits"].([]wiki.Object) {
					id := e["edit_id"].(string)
					result.Resources = append(result.Resources, &mcp.Resource{Name: id, URI: "wiki://edit/" + id, MIMEType: "application/json"})
				}
				result.NextCursor, _ = out["next_cursor"].(string)
				return result, nil
			}
		})
	}}
}
