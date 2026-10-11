package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Parse the complete snapshot once for links and, on the first read, files.
func (s *Service) pageSourceMetadata(ctx context.Context, c *Call, snapshot Object, files bool) (Object, []string, error) {
	links := []string{}
	if snapshot["content"] == nil {
		return Object{}, links, nil
	}
	prop := "links"
	if files {
		prop = "images|links"
	}
	q := url.Values{"action": {"parse"}, "prop": {prop}}
	if id := number(snapshot["revision_id"]); id > 0 {
		q.Set("oldid", fmt.Sprint(id))
	} else {
		q.Set("text", str(snapshot["content"]))
		q.Set("title", str(snapshot["title"]))
		q.Set("contentmodel", "wikitext")
	}
	v, err := c.Client.Query(ctx, c.Credentials, q)
	if err != nil {
		return nil, nil, err
	}
	parsed := object(v["parse"])
	values, ok := parsed["links"].([]any)
	if !ok {
		return nil, nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted parsed page links.")
	}
	seen := map[string]bool{}
	for _, value := range values {
		title := str(object(value)["title"])
		if strings.TrimSpace(title) == "" {
			return nil, nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned an invalid page link.")
		}
		if !seen[title] {
			seen[title] = true
			links = append(links, title)
		}
	}
	var urls Object
	if files {
		urls, err = s.parsedFileURLs(ctx, c, parsed)
		if err != nil {
			return nil, nil, err
		}
	}
	return urls, links, nil
}
