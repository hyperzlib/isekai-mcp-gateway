package wiki

import (
	"context"
	"gorm.io/gorm"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/integrations/mediawiki"
	"isekai-mcp-gateway/internal/security"
	"time"
)

type savedSite struct {
	MediaWiki config.MediaWiki
	Secret    string
}

// Old site connections are encrypted so drafts remain addressable after a site
// switch and process restart. Same-site key rotations replace the connection.
func (s *Service) rememberSite(ctx context.Context, client *mediawiki.Client) error {
	// 同一客户端实例已注册时无需重复持久化。
	if old, ok := s.sites.Load(client.Config.SiteID); ok && old == client {
		return nil
	}
	site := savedSite{MediaWiki: client.Config, Secret: client.Secret}

	// 桥接密钥单独存放，主配置副本中清空避免明文落库。
	site.MediaWiki.Bridge.Secret = ""
	row := Record{ID: security.Hash(client.Config.SiteID), Owner: security.Hash(client.Config.SiteID), State: "active", ExpiresAt: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)}

	// 在事务中锁定站点配额并写入连接记录。
	if x := s.Repo.tx(ctx, func(db *gorm.DB) error {
		if x := s.Repo.quota(db, security.Hash("sites")); x != nil {
			return x
		}
		var count int64
		if x := db.Table("wiki_site_connections").Where("id = ?", row.ID).Count(&count).Error; x != nil {
			return x
		}

		// 首次写入使用 create，已存在则走更新分支。
		return s.Repo.save(db, "wiki_site_connections", row, site, count == 0)
	}); x != nil {
		return x
	}
	s.sites.Store(client.Config.SiteID, client)
	return nil
}
func (s *Service) restoreSites(ctx context.Context) error {
	var rows []Record
	if x := s.Repo.Store.DB.WithContext(ctx).Table("wiki_site_connections").Find(&rows).Error; x != nil {
		return x
	}

	// 从持久化记录重建各站点的 MediaWiki 客户端。
	for _, r := range rows {
		var site savedSite
		if x := s.Repo.Store.Vault.Open(r.Payload, &site, "wiki_site_connections:"+r.ID); x != nil {
			return x
		}
		cfg := config.Defaults()
		cfg.App.MediaWiki = site.MediaWiki
		cfg.Secrets.BridgeSecret = site.Secret
		s.sites.Store(site.MediaWiki.SiteID, mediawiki.New(cfg))
	}

	// 恢复完成后重新登记当前活跃的上游连接。
	if s.Auth != nil && s.Auth.Upstream != nil {
		if b := s.Auth.Upstream.Get(); b != nil {
			return s.rememberSite(ctx, b.Bridge)
		}
	}
	return nil
}
