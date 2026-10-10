package wiki

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

func IsPageRead(name string) bool {
	switch name {
	case "get_latest_page", "get_baseline_page", "get_history", "get_backlinks",
		"get_page_structure", "get_template_data", "get_page_dependencies", "get_transclusions", "list_subpages":
		return true
	}
	return false
}

// PageResourceInput decodes exactly one escaped path segment. Query strings and
// fragments are not selectors; encoded slashes remain part of the page title.
func PageResourceInput(uri, template string) (Input, error) {
	invalid := func() (Input, error) { return Input{}, errCode("NOT_FOUND", "Resource was not found.") }

	// 修订入口与页面入口共用路由校验，但修订始终用独立 ID 定位。
	if strings.Contains(template, "{revision_id}") {
		raw, err := resourceSegment(uri, template, "{revision_id}")
		if err != nil {
			return invalid()
		}
		id, err := resourceID(raw)
		if err != nil {
			return invalid()
		}
		return Input{RevisionID: id}, nil
	}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "wiki" || u.Host != "pages" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(uri, "#") {
		return invalid()
	}
	marker := "{encoded_title}"
	byTitle := strings.Contains(template, marker)
	if !byTitle {
		marker = "{page_id}"
	}
	parts := strings.Split(template, marker)
	if len(parts) != 2 || !strings.HasPrefix(uri, parts[0]) || !strings.HasSuffix(uri, parts[1]) {
		return invalid()
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(uri, parts[0]), parts[1])
	if raw == "" || strings.ContainsAny(raw, "/?#") {
		return invalid()
	}
	i := Input{}
	if byTitle {
		title, err := url.PathUnescape(raw)
		if err != nil || !utf8.ValidString(title) {
			return invalid()
		}
		i.Title = title
	} else {
		if strings.Trim(raw, "0123456789") != "" {
			return invalid()
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return invalid()
		}
		i.PageID = id
	}
	if _, err := pageParams(PageRef{PageID: i.PageID, Title: i.Title}); err != nil {
		return invalid()
	}
	return i, nil
}

func PageResourceLinks(title string, id int64, suffix string) Object {
	encoded := strings.ReplaceAll(url.QueryEscape(title), "+", "%20")
	links := Object{"by_title": "wiki://pages/by-title/" + encoded + suffix}
	if id > 0 {
		links["by_id"] = fmt.Sprintf("wiki://pages/by-id/%d%s", id, suffix)
	}
	return links
}
