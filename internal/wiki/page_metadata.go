package wiki

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"isekai-mcp-gateway/internal/integrations/mediawiki"
)

func parsedPageRead(name string) bool {
	return name == "get_page_structure" || name == "get_page_dependencies"
}

func metadataSelector(i Input, allowRevision bool) error {
	if i.Page.PageID != 0 || i.Page.Title != "" {
		return errCode("INVALID_REQUEST", "Use top-level title, page_id or revision_id.")
	}
	if i.RevisionID != 0 {
		if !allowRevision || i.RevisionID < 0 || i.PageID != 0 || i.Title != "" {
			return errCode("INVALID_REQUEST", "Supply exactly one title, page_id or positive revision_id.")
		}
		return nil
	}
	_, err := pageParams(PageRef{PageID: i.PageID, Title: i.Title})
	return err
}

// 解析结果绑定正式修订；缓存过期后重读并校验解析哈希。
func (s *Service) readParsedPage(ctx context.Context, c *Call, name string, i Input) (Object, error) {
	if err := metadataSelector(i, true); err != nil {
		return nil, err
	}
	limit, err := positive(i.Limit, 20, 100)
	if err != nil {
		return nil, err
	}
	if _, err = enumValues(i.DependencyTypes, "templates links images categories external_links"); err != nil {
		return nil, err
	}
	filter := fingerprint(Object{"title": i.Title, "page_id": i.PageID, "revision_id": i.RevisionID, "limit": limit, "dependency_types": i.DependencyTypes})
	owner := c.owner(c.Client.Config.SiteID)
	cur, err := readCursor(i.Cursor, name, owner, filter)
	if err != nil {
		return nil, err
	}
	q := url.Values{"action": {"parse"}, "prop": {"sections|revid"}}
	suffix := "/structure"
	if name == "get_page_dependencies" {
		props := []string{"revid"}
		selected := i.DependencyTypes
		if len(selected) == 0 {
			selected = []string{"templates", "links", "images", "categories", "external_links"}
		}
		for _, kind := range selected {
			if kind == "external_links" {
				kind = "externallinks"
			}
			props = append(props, kind)
		}
		q.Set("prop", strings.Join(props, "|"))
		suffix = "/dependencies"
	}
	id := i.RevisionID
	if i.Cursor != "" {
		id = number(cur.Continue["revision_id"])
		if id <= 0 || cur.Offset < 0 {
			return nil, errCode("INVALID_CURSOR", "Parsed snapshot cursor is invalid.")
		}
	}
	if id > 0 {
		q.Set("oldid", fmt.Sprint(id))
	} else if i.PageID > 0 {
		q.Set("pageid", fmt.Sprint(i.PageID))
	} else {
		q.Set("page", i.Title)
	}
	scope := c.cacheScope(name, filter)
	generation := s.pageCache.begin(scope, i.Cursor == "")
	cacheID := str(cur.Continue["cache_id"])
	var parsed Object
	if i.Cursor != "" {
		parsed = s.pageCache.get(cacheID, scope)
	}
	cacheHit := parsed != nil
	if cacheHit {
		if _, err := s.checkCachedRead(ctx, c, number(parsed["pageid"]), str(parsed["title"]), id, false); err != nil {
			return nil, err
		}
	} else {
		v, err := c.Client.Query(ctx, c.Credentials, q)
		if err != nil {
			return nil, err
		}
		parsed = object(v["parse"])
	}
	if number(parsed["revid"]) <= 0 || number(parsed["pageid"]) <= 0 || str(parsed["title"]) == "" {
		return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted parsed page metadata.")
	}
	if id > 0 && number(parsed["revid"]) != id {
		return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned a different revision.")
	}
	if i.PageID > 0 && number(parsed["pageid"]) != i.PageID {
		return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned a different page.")
	}
	var items []any
	if name == "get_page_structure" {
		var ok bool
		items, ok = parsed["sections"].([]any)
		if !ok {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted page sections.")
		}
	} else {
		items, err = dependencyItems(parsed, i.DependencyTypes)
		if err != nil {
			return nil, err
		}
	}
	hash := fingerprint(items)
	if i.Cursor != "" && str(cur.Continue["parse_hash"]) != hash {
		return nil, errCode("SNAPSHOT_UNAVAILABLE", "Parsed dependencies or sections changed. Restart the read.")
	}
	if !cacheHit {
		cacheID = s.pageCache.put(c, scope, generation, number(parsed["pageid"]), str(parsed["title"]), parsed)
	}
	if cur.Offset > len(items) {
		return nil, errCode("INVALID_CURSOR", "Parsed offset is outside the result.")
	}
	end := cur.Offset + limit
	if end > len(items) {
		end = len(items)
	}
	links := PageResourceLinks(str(parsed["title"]), number(parsed["pageid"]), suffix)
	links["by_revision"] = fmt.Sprintf("wiki://revisions/%d%s", number(parsed["revid"]), suffix)
	out := Object{
		"page_id": parsed["pageid"], "title": parsed["title"],
		"revision_id": parsed["revid"], "content_source": "baseline",
		"results": items[cur.Offset:end], "total": len(items),
		"next_cursor": nil, "resources": links,
	}
	if i.Title != "" {
		out["requested_title"] = i.Title
	}
	if end < len(items) {
		out["next_cursor"] = cursor(cursorData{Tool: name, Owner: owner, Filter: filter, Offset: end, Continue: Object{"revision_id": parsed["revid"], "parse_hash": hash, "cache_id": cacheID}})
	}
	return out, nil
}

func dependencyItems(parsed Object, selected []string) ([]any, error) {
	items := []any{}
	for _, spec := range []struct{ kind, key string }{
		{"templates", "templates"}, {"links", "links"}, {"images", "images"}, {"categories", "categories"}, {"external_links", "externallinks"},
	} {
		include := len(selected) == 0
		for _, kind := range selected {
			if kind == spec.kind {
				include = true
			}
		}
		if !include {
			continue
		}
		values, ok := parsed[spec.key].([]any)
		if !ok {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted parsed dependencies.")
		}
		for _, value := range values {
			entry := Object{"type": spec.kind, "data": value}
			// 保留解析器原始字段；只有已经明确的完整标题才生成页面链接。
			if title := str(object(value)["title"]); title != "" {
				entry["resources"] = PageResourceLinks(title, 0, "")
			}
			items = append(items, entry)
		}
	}
	return items, nil
}

func (s *Service) readTemplateData(ctx context.Context, c *Call, i Input) (Object, error) {
	if err := metadataSelector(i, false); err != nil {
		return nil, err
	}
	p, _ := pageParams(PageRef{PageID: i.PageID, Title: i.Title})
	q := url.Values(p)
	q.Set("action", "templatedata")
	q.Set("includeMissingTitles", "1")
	if i.Language != "" {
		q.Set("lang", i.Language)
	}
	v, err := c.Client.Query(ctx, c.Credentials, q)
	if err != nil {
		var apiError *mediawiki.APIError
		if errors.As(err, &apiError) && apiError.Code == "unknown_action" {
			return nil, errCode("FEATURE_UNAVAILABLE", "Wiki does not provide the TemplateData API.")
		}
		return nil, err
	}
	// TemplateData 的 pages 是以 ID 为键的对象，不是 query.pages 数组。
	entries, ok := v["pages"].(map[string]any)
	if !ok || len(entries) != 1 {
		return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki did not return exactly one template.")
	}
	var id int64
	var data Object
	for key, value := range entries {
		id, err = strconv.ParseInt(key, 10, 64)
		data, ok = value.(map[string]any)
		if err != nil || !ok {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned invalid template metadata.")
		}
	}
	if _, missing := data["missing"]; missing || id <= 0 {
		return nil, errCode("NOT_FOUND", "Template page was not found.")
	}
	if i.PageID > 0 && id != i.PageID || str(data["title"]) == "" {
		return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned a different template.")
	}
	_, absent := data["notemplatedata"]
	out := Object{
		"page_id": id, "title": data["title"],
		"has_template_data": !absent, "data": nil,
		"resources":  PageResourceLinks(str(data["title"]), id, "/template-data"),
		"queried_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if i.Title != "" {
		out["requested_title"] = i.Title
	}
	if !absent {
		// 声明的参数应是对象；缺失或损坏的数据不能冒充有效 TemplateData。
		if _, ok := data["params"].(map[string]any); !ok {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted declared template parameters.")
		}
		out["data"] = data
	}
	return out, nil
}

func (s *Service) readTransclusions(ctx context.Context, c *Call, i Input) (Object, error) {
	if err := metadataSelector(i, false); err != nil {
		return nil, err
	}
	limit, err := positive(i.Limit, 20, 100)
	if err != nil {
		return nil, err
	}
	namespaces, err := namespaceValues(i.Namespaces)
	if err != nil {
		return nil, err
	}
	q := url.Values{"action": {"query"}, "list": {"embeddedin"}, "eilimit": {strconv.Itoa(limit)}}
	if i.PageID > 0 {
		q.Set("eipageid", fmt.Sprint(i.PageID))
	} else {
		q.Set("eititle", i.Title)
	}
	if namespaces != "" {
		q.Set("einamespace", namespaces)
	}
	out, err := s.queryList(ctx, c, "get_transclusions", q, q, i, "eicontinue")
	if err != nil {
		return nil, err
	}
	addPageLinks(out["results"].([]any))
	if i.Title != "" {
		out["requested_title"] = i.Title
		out["resources"] = PageResourceLinks(i.Title, 0, "/transclusions")
	} else {
		out["page_id"] = i.PageID
		out["resources"] = Object{"by_id": fmt.Sprintf("wiki://pages/by-id/%d/transclusions", i.PageID)}
	}
	return out, nil
}
