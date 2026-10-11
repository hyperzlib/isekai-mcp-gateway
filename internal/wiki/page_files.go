package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

func (s *Service) readFileURLs(ctx context.Context, c *Call, titles []string) (Object, error) {
	if len(titles) == 0 || len(titles) > 500 {
		return nil, errCode("INVALID_REQUEST", "Supply between 1 and 500 file titles.")
	}
	names := []string{}
	seen := map[string]bool{}
	for _, title := range titles {
		prefix, name, ok := strings.Cut(title, ":")
		prefix = strings.ToLower(prefix)
		if !ok || (prefix != "file" && prefix != "image" && prefix != "文件" && prefix != "檔案" && prefix != "图像" && prefix != "圖像") || strings.TrimSpace(name) == "" || !utf8.ValidString(title) || strings.ContainsAny(title, "|\r\n\x00#") {
			return nil, errCode("INVALID_REQUEST", "Supply file titles with a File: or 文件: namespace prefix.")
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	files, err := s.resolveFileURLs(ctx, c, names)
	if err != nil {
		return nil, err
	}
	return Object{"file_urls": files}, nil
}

// pageFileURLs resolves files used by the selected complete page snapshot.
func (s *Service) pageFileURLs(ctx context.Context, c *Call, snapshot Object) (Object, error) {
	out := Object{}
	if snapshot["content"] == nil {
		return out, nil
	}
	q := url.Values{"action": {"parse"}, "prop": {"images"}}
	if id := number(snapshot["revision_id"]); id > 0 {
		q.Set("oldid", fmt.Sprint(id))
	} else {
		q.Set("text", str(snapshot["content"]))
		q.Set("title", str(snapshot["title"]))
		q.Set("contentmodel", "wikitext")
	}
	v, err := c.Client.Query(ctx, c.Credentials, q)
	if err != nil {
		return nil, err
	}
	return s.parsedFileURLs(ctx, c, object(v["parse"]))
}

func (s *Service) parsedFileURLs(ctx context.Context, c *Call, parsed Object) (Object, error) {
	images, ok := parsed["images"].([]any)
	if !ok {
		return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted parsed files.")
	}
	names := []string{}
	seen := map[string]bool{}
	for _, image := range images {
		name, ok := image.(string)
		if !ok || name == "" {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned an invalid file name.")
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return s.resolveFileURLs(ctx, c, names)
}

func (s *Service) resolveFileURLs(ctx context.Context, c *Call, names []string) (Object, error) {
	out := Object{}
	for start := 0; start < len(names); start += 50 {
		end := start + 50
		if end > len(names) {
			end = len(names)
		}
		titles := make([]string, end-start)
		for n, name := range names[start:end] {
			titles[n] = "File:" + name
		}
		v, err := c.Client.Query(ctx, c.Credentials, url.Values{
			"action": {"query"}, "prop": {"imageinfo"}, "iiprop": {"url"},
			"titles": {strings.Join(titles, "|")}, "redirects": {"1"},
		})
		if err != nil {
			return nil, err
		}
		query := object(v["query"])
		pgs, ok := query["pages"].([]any)
		if !ok {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted file pages.")
		}
		aliases := map[string]string{}
		for _, key := range []string{"normalized", "converted", "redirects"} {
			items, _ := query[key].([]any)
			for _, item := range items {
				entry := object(item)
				aliases[str(entry["from"])] = str(entry["to"])
			}
		}
		urls := map[string]string{}
		for _, pg := range pgs {
			page := object(pg)
			info, _ := page["imageinfo"].([]any)
			if len(info) > 0 {
				if u := str(object(info[0])["url"]); u != "" {
					urls[str(page["title"])] = u
				}
			}
		}
		for n, title := range titles {
			visited := map[string]bool{}
			for aliases[title] != "" && !visited[title] {
				visited[title] = true
				title = aliases[title]
			}
			if u := urls[title]; u != "" {
				out["文件:"+names[start+n]] = u
			}
		}
	}
	return out, nil
}
