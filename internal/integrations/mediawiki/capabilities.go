package mediawiki

import (
	"context"
	"encoding/json"
	"io"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

func (c *Client) publicQuery(ctx context.Context, q url.Values) (map[string]any, error) {
	u, e := url.Parse(c.Config.APIURL)
	if e != nil {
		return nil, e
	}
	q.Set("format", "json")
	q.Set("formatversion", "2")
	u.RawQuery = q.Encode()
	r, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return nil, e
	}
	response, e := c.HTTP.Do(r)
	if e != nil {
		return nil, failure.New("API_NETWORK_ERROR", "Wiki capability request failed without automatic retry.", "wiki.capabilities", e)
	}
	defer response.Body.Close()

	// 限制响应 2MiB，超限或非 200 统一视为能力不可用
	b, e := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 2*1024*1024 || response.StatusCode != 200 {
		return nil, failure.New("WIKI_CAPABILITY_UNAVAILABLE", "Wiki capabilities could not be verified.", "wiki.capabilities", nil)
	}
	var v map[string]any
	if json.Unmarshal(b, &v) != nil || v["error"] != nil {
		return nil, failure.New("WIKI_CAPABILITY_UNAVAILABLE", "Wiki capabilities could not be verified.", "wiki.capabilities", nil)
	}
	return v, nil
}
func (c *Client) Capabilities(ctx context.Context, cfg *config.Config) error {
	v, e := c.publicQuery(ctx, url.Values{"action": {"query"}, "meta": {"siteinfo"}, "siprop": {"extensions|general"}})
	if e != nil {
		return e
	}
	// 收集已安装扩展名
	q, _ := v["query"].(map[string]any)
	extensions, _ := q["extensions"].([]any)
	names := []string{}
	for _, raw := range extensions {
		if ex, ok := raw.(map[string]any); ok {
			if name, ok := ex["name"].(string); ok {
				names = append(names, name)
			}
		}
	}

	// 仅在配置开启时要求对应扩展已安装，配置关闭时忽略扩展是否存在
	missing := []string{}
	if cfg.Static.Extensions.Moderation.Enabled && !slices.Contains(names, "Moderation") {
		missing = append(missing, "Moderation")
	}
	if cfg.Static.Extensions.ACL.Enabled && !slices.Contains(names, "IsekaiLitePageACL") {
		missing = append(missing, "IsekaiLitePageACL")
	}
	if len(missing) > 0 {
		return failure.New("WIKI_EXTENSION_MISMATCH", "Configured Wiki extensions are not installed: "+strings.Join(missing, ", ")+".", "wiki.capabilities", nil)
	}
	if !slices.Contains(names, "IsekaiServiceBridge") {
		return failure.New("WIKI_CAPABILITY_UNAVAILABLE", "IsekaiServiceBridge is required.", "wiki.capabilities", nil)
	}

	// 组装所需 API 模块，Moderation 启用时追加相关查询模块
	modules := []string{"edit", "upload", "query+isekaiuseredited"}
	if cfg.Static.Extensions.Moderation.Enabled {
		modules = append(modules, "query+moderationeditstate", "query+moderationrevision", "query+moderationrevisions")
	}
	v, e = c.publicQuery(ctx, url.Values{"action": {"paraminfo"}, "modules": {strings.Join(modules, "|")}})
	if e != nil {
		return e
	}

	// 按 path 建索引并确认所需模块全部存在
	pi, _ := v["paraminfo"].(map[string]any)
	defs, _ := pi["modules"].([]any)
	found := map[string]map[string]any{}
	for _, raw := range defs {
		m, _ := raw.(map[string]any)
		path, _ := m["path"].(string)
		found[path] = m
	}
	for _, path := range modules {
		if found[path] == nil {
			return failure.New("WIKI_CAPABILITY_UNAVAILABLE", "A required Wiki API module is unavailable.", "wiki.capabilities", nil)
		}
	}

	// Moderation 要求 edit 支持非负 moderation_baserevid
	if cfg.Static.Extensions.Moderation.Enabled {
		params, _ := found["edit"]["parameters"].([]any)
		valid := false
		for _, raw := range params {
			p, _ := raw.(map[string]any)
			if p["name"] == "moderation_baserevid" && p["type"] == "integer" && p["min"] == float64(0) {
				valid = true
			}
		}
		if !valid {
			return failure.New("WIKI_CAPABILITY_UNAVAILABLE", "edit must support nonnegative moderation_baserevid.", "wiki.capabilities", nil)
		}
	}
	v, e = c.publicQuery(ctx, url.Values{"action": {"query"}, "list": {"tags"}, "tgprop": {"defined|active"}, "tglimit": {"max"}})
	if e != nil {
		return e
	}
	// 校验配置的 AI 变更标签均已定义且启用
	q, _ = v["query"].(map[string]any)
	tags, _ := q["tags"].([]any)
	defined := map[string]bool{}
	for _, raw := range tags {
		tag, _ := raw.(map[string]any)
		name, _ := tag["name"].(string)
		if tag["defined"] == true && tag["active"] == true {
			defined[name] = true
		}
	}
	for _, name := range cfg.App.Additions.Tags.Mapping {
		if !defined[name] {
			return failure.New("AI_TAG_UNAVAILABLE", "Configured AI change tags are not defined and active.", "wiki.capabilities", nil)
		}
	}
	return nil
}
