package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

func boundedRead(c *Call, out Object, err error) (Object, error) {
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > c.Config.App.Results.TextMax {
		return nil, errCode("RESULT_TOO_LARGE", "Result exceeds the read limit. Reduce limit or narrow the filters.")
	}
	return out, nil
}

func listResult(v Object, key string) ([]any, error) {
	items, ok := object(v["query"])[key].([]any)
	if !ok {
		return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted the requested list.")
	}
	return items, nil
}

func enumValues(values []string, allowed string) (string, error) {
	seen := map[string]bool{}
	for _, value := range values {
		if !strings.Contains(" "+allowed+" ", " "+value+" ") || value == "" || strings.ContainsAny(value, " |\r\n") || seen[value] {
			return "", errCode("INVALID_REQUEST", "Invalid or repeated filter value.")
		}
		seen[value] = true
		if strings.HasPrefix(value, "!") && seen[strings.TrimPrefix(value, "!")] || seen["!"+value] {
			return "", errCode("INVALID_REQUEST", "Conflicting filter values.")
		}
	}
	return strings.Join(values, "|"), nil
}

func namespaceValues(values []int) (string, error) {
	parts := make([]string, 0, len(values))
	seen := map[int]bool{}
	for _, value := range values {
		if value < 0 || seen[value] {
			return "", errCode("INVALID_REQUEST", "Invalid or repeated namespace.")
		}
		seen[value] = true
		parts = append(parts, fmt.Sprint(value))
	}
	return strings.Join(parts, "|"), nil
}

// 只接受明确的 UTC/时区时间；方向决定起止范围的大小关系。
func activityRange(i Input) (string, error) {
	direction := i.Direction
	if direction == "" {
		direction = "older"
	}
	if direction != "older" && direction != "newer" {
		return "", errCode("INVALID_REQUEST", "direction must be older or newer.")
	}
	var start, end time.Time
	var err error
	if i.Start != "" {
		start, err = time.Parse(time.RFC3339, i.Start)
		if err != nil {
			return "", errCode("INVALID_REQUEST", "start must be an RFC3339 timestamp.")
		}
	}
	if i.End != "" {
		end, err = time.Parse(time.RFC3339, i.End)
		if err != nil {
			return "", errCode("INVALID_REQUEST", "end must be an RFC3339 timestamp.")
		}
	}
	if !start.IsZero() && !end.IsZero() && (direction == "older" && start.Before(end) || direction == "newer" && start.After(end)) {
		return "", errCode("INVALID_REQUEST", "Time range does not match direction.")
	}
	return direction, nil
}

// 续页参数只允许当前模块的已知字段，保留 continue 整体以支持上游分页。
func (s *Service) queryList(ctx context.Context, c *Call, name string, q url.Values, filter any, input Input, allowed string) (Object, error) {
	fp := fingerprint(filter)
	owner := c.owner(c.Client.Config.SiteID)
	cur, err := readCursor(input.Cursor, name, owner, fp)
	if err != nil {
		return nil, err
	}
	allowedKeys := append([]string{"continue"}, strings.Fields(allowed)...)
	for key, value := range cur.Continue {
		text, ok := value.(string)
		if !ok || !slices.Contains(allowedKeys, key) {
			return nil, errCode("INVALID_CURSOR", "Cursor contains an unsupported continuation.")
		}
		q.Set(key, text)
	}
	v, err := c.Client.Query(ctx, c.Credentials, q)
	if err != nil {
		return nil, err
	}
	items, err := listResult(v, q.Get("list"))
	if err != nil {
		return nil, err
	}
	out := Object{"results": items, "next_cursor": nil, "queried_at": time.Now().UTC().Format(time.RFC3339Nano)}
	if value, exists := v["continue"]; exists {
		co, ok := value.(map[string]any)
		if !ok || len(co) == 0 {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned invalid continuation.")
		}
		for key, value := range co {
			if _, ok := value.(string); !ok || !slices.Contains(allowedKeys, key) {
				return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned unsupported continuation.")
			}
		}
		out["next_cursor"] = cursor(cursorData{Tool: name, Owner: owner, Filter: fp, Continue: co})
	}
	return out, nil
}

func addPageLinks(items []any) {
	for _, value := range items {
		item := object(value)
		if title := str(item["title"]); title != "" {
			item["resources"] = PageResourceLinks(title, number(item["pageid"]), "")
		}
	}
}
