package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"isekai-mcp-gateway/internal/security"
)

// Immutable JSON entries avoid sharing mutable maps between concurrent reads.
// Entries expire from insertion, and the process retains at most 64 MiB/256 entries.
type readCacheEntry struct {
	data               []byte
	scope, site, title string
	pageID             int64
	expires            time.Time
}
type readCache struct {
	mu         sync.Mutex
	entries    map[string]readCacheEntry
	bytes      int
	generation uint64
}

func (c *Call) cacheScope(name, filter string) string {
	return fingerprint(Object{"owner": c.owner(c.Client.Config.SiteID), "principal": c.principalKey(),
		"credentials": c.Credentials, "api": c.Client.Config.APIURL, "key": c.Client.Config.Bridge.KeyID,
		"secret": security.Hash(c.Client.Secret), "config": c.Config.App, "extensions": c.Config.Static.Extensions,
		"tool": name, "filter": filter})
}

func (r *readCache) remove(id string) {
	r.bytes -= len(r.entries[id].data)
	delete(r.entries, id)
}

func (r *readCache) begin(scope string, refresh bool) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for id, entry := range r.entries {
		if !entry.expires.After(now) || refresh && entry.scope == scope {
			r.remove(id)
		}
	}
	if refresh {
		r.generation++
	}
	return r.generation
}

func (r *readCache) get(id, scope string) Object {
	r.mu.Lock()
	entry, ok := r.entries[id]
	if ok && !entry.expires.After(time.Now()) {
		r.remove(id)
		ok = false
	}
	r.mu.Unlock()
	if !ok || entry.scope != scope {
		return nil
	}
	var out Object
	if json.Unmarshal(entry.data, &out) != nil {
		return nil
	}
	return out
}

func (r *readCache) put(c *Call, scope string, generation uint64, pageID int64, title string, value Object) string {
	data, err := json.Marshal(value)
	if err != nil || len(data) > 32*1024*1024 {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if generation != r.generation {
		return ""
	} // A first-page read or write invalidated this in-flight read.
	if r.entries == nil {
		r.entries = make(map[string]readCacheEntry)
	}
	for len(r.entries) >= 256 || r.bytes+len(data) > 64*1024*1024 {
		oldest := ""
		var expiry time.Time
		for id, entry := range r.entries {
			if oldest == "" || entry.expires.Before(expiry) {
				oldest, expiry = id, entry.expires
			}
		}
		r.remove(oldest)
	}
	id := security.Random()
	r.entries[id] = readCacheEntry{data: data, scope: scope, site: c.Client.Config.SiteID,
		pageID: pageID, title: title, expires: time.Now().Add(time.Duration(c.Config.App.MediaWiki.PageCacheTTL) * time.Second)}
	r.bytes += len(data)
	return id
}

func (s *Service) invalidatePageCache(c *Call, pageID int64, title string) {
	s.pageCache.mu.Lock()
	defer s.pageCache.mu.Unlock()
	s.pageCache.generation++
	for id, entry := range s.pageCache.entries {
		if entry.site == c.Client.Config.SiteID && (pageID > 0 && entry.pageID == pageID || title != "" && entry.title == title) {
			s.pageCache.remove(id)
		}
	}
}

// Recheck page access and revision visibility without downloading content or parsing it.
// Page reads also get a fresh contribution flag in the same request.
func (s *Service) checkCachedRead(ctx context.Context, c *Call, pageID int64, title string, revision int64, contribution bool) (Object, error) {
	p := PageRef{PageID: pageID}
	if pageID <= 0 {
		p = PageRef{Title: title}
	}
	params, err := pageParams(p)
	if err != nil {
		return nil, err
	}
	q := url.Values(params)
	q.Set("action", "query")
	q.Set("prop", "info")
	q.Set("intestactions", "read")
	q.Set("intestactionsdetail", "full")
	if contribution && pageID > 0 {
		q.Set("prop", "info|isekaiuseredited")
	}
	if revision > 0 {
		q.Del("pageids")
		q.Del("titles")
		q.Set("revids", fmt.Sprint(revision))
		q.Set("prop", q.Get("prop")+"|revisions")
		q.Set("rvprop", "ids|slotsha1")
		q.Set("rvslots", "main")
	}
	v, err := c.Client.Query(ctx, c.Credentials, q)
	if err != nil {
		return nil, err
	}
	items, err := pages(v)
	if err != nil {
		return nil, err
	}
	pg := object(items[0])
	readErrors, ok := object(pg["actions"])["read"].([]any)
	if !ok || len(readErrors) != 0 {
		return nil, errCode("PERMISSION_DENIED", "Wiki denied access to the cached page.")
	}
	if pageID > 0 && number(pg["pageid"]) != pageID {
		return nil, errCode("SNAPSHOT_UNAVAILABLE", "Cached page is no longer available.")
	}
	if revision > 0 {
		revs, _ := pg["revisions"].([]any)
		if len(revs) != 1 || number(object(revs[0])["revid"]) != revision {
			return nil, errCode("SNAPSHOT_UNAVAILABLE", "Cached revision is no longer available.")
		}
		if _, ok := object(object(object(revs[0])["slots"])["main"])["sha1"].(string); !ok {
			return nil, errCode("PERMISSION_DENIED", "Cached revision content is no longer visible.")
		}
	}
	if contribution && pageID > 0 {
		edited, err := pageContribution(pg)
		if err != nil {
			return nil, err
		}
		return Object{"has_edited": edited}, nil
	}
	return nil, nil
}
