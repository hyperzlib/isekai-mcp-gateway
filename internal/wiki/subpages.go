package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// 子页面来自标题索引，父页面解析缓存和修订号不能固定这个实时列表。
func (s *Service) readSubpages(ctx context.Context, c *Call, i Input) (Object, error) {
	if err := metadataSelector(i, false); err != nil {
		return nil, err
	}
	limit, err := positive(i.Limit, 20, 100)
	if err != nil {
		return nil, err
	}
	p, _ := pageParams(PageRef{Title: i.Title, PageID: i.PageID})
	info := url.Values(p)
	info.Set("action", "query")
	info.Set("prop", "info")
	v, err := c.Client.Query(ctx, c.Credentials, info)
	if err != nil {
		return nil, err
	}
	a, err := pages(v)
	if err != nil {
		return nil, err
	}
	parent := object(a[0])
	if _, invalid := parent["invalid"]; invalid {
		return nil, errCode("INVALID_REQUEST", "Wiki rejected the page title.")
	}
	id := number(parent["pageid"])
	_, missing := parent["missing"]
	if i.PageID > 0 && missing {
		return nil, errCode("NOT_FOUND", "Parent page was not found.")
	}
	title := str(parent["title"])
	ns, ok := parent["ns"].(float64)
	if !ok || ns != float64(int64(ns)) || title == "" {
		return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted parent page metadata.")
	}
	if ns < 0 {
		return nil, errCode("INVALID_REQUEST", "Subpages require a nonnegative namespace.")
	}
	if i.PageID > 0 && id != i.PageID || !missing && id <= 0 {
		return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned invalid parent page metadata.")
	}
	base := title
	if ns > 0 {
		_, base, ok = strings.Cut(title, ":")
		if !ok || base == "" {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted the namespace prefix.")
		}
	}
	q := url.Values{"action": {"query"}, "list": {"allpages"}, "apnamespace": {fmt.Sprint(int64(ns))}, "apprefix": {base + "/"}, "aplimit": {strconv.Itoa(limit)}, "apdir": {"ascending"}, "apfilterredir": {"all"}}
	filter := Object{"title": i.Title, "page_id": i.PageID, "limit": limit, "namespace": int64(ns), "prefix": base + "/"}
	out, err := s.queryList(ctx, c, "list_subpages", q, filter, i, "apcontinue")
	if err != nil {
		return nil, err
	}
	for _, value := range out["results"].([]any) {
		item := object(value)
		itemNS, validNS := item["ns"].(float64)
		relative, match := strings.CutPrefix(str(item["title"]), title+"/")
		if !validNS || itemNS != ns || number(item["pageid"]) <= 0 || !match || relative == "" {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned a page outside the requested subtree.")
		}
		item["relative_title"] = relative
		item["depth"] = strings.Count(relative, "/") + 1
	}
	addPageLinks(out["results"].([]any))
	out["page_id"] = nil
	if !missing {
		out["page_id"] = id
	}
	out["title"] = title
	if i.Title != "" {
		out["requested_title"] = i.Title
	}
	return out, nil
}
