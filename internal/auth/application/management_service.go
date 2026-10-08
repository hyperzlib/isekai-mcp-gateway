package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
)

func (s *Service) RevokeManaged(ctx context.Context, accountID, kind, id string) error {
	if kind != "grants" && kind != "client_trusts" {
		return errors.New("invalid management kind")
	}
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		// 锁定目标记录，确保只能撤销属于该账号的资源。
		var row persistence.Row
		if e := tx.Table(persistence.Table(kind)).Where("id = ? AND account_id = ?", id, accountID).Lock().Take(&row).Error(); e != nil {
			return e
		}
		if kind == "grants" {
			return s.RevokeGrant(tx, row.GrantID)
		}
		// 撤销信任时级联吊销该客户端下的全部授权。
		var grants []persistence.Row
		if e := tx.Table(persistence.Table("grants")).Where("account_id = ? AND client_id = ?", accountID, row.ClientID).Find(&grants).Error(); e != nil {
			return e
		}
		for _, g := range grants {
			if e := s.RevokeGrant(tx, g.GrantID); e != nil {
				return e
			}
		}
		return tx.Table(persistence.Table(kind)).Where("id = ? AND account_id = ?", row.ID, accountID).Delete(&persistence.Row{}).Error()
	})
}
func (s *Service) ListManaged(ctx context.Context, accountID, kind string) ([]domain.ManagementEntry, error) {
	if kind != "grants" && kind != "client_trusts" {
		return nil, errors.New("invalid management kind")
	}
	var rows []persistence.Row
	q := s.Store.Database().WithContext(ctx).Table(persistence.Table(kind)).Where("account_id = ? AND state = ?", accountID, "active")
	// 授权列表只展示尚未过期的记录。
	if kind == "grants" {
		q = q.Where("expires_at > ?", time.Now().UTC())
	}
	if err := q.Order("updated_at DESC").Find(&rows).Error(); err != nil {
		return nil, &FlowError{Message: "无法加载授权列表。", Status: 500, Cause: failure.Wrap("management.list", err)}
	}
	entries := []domain.ManagementEntry{}
	for _, row := range rows {
		entry := domain.ManagementEntry{ID: row.ID}
		if kind == "grants" {
			var g Grant
			if err := s.Store.OpenRecord(row, &g, kind); err != nil {
				return nil, &FlowError{Message: "无法加载授权列表。", Status: 500, Cause: failure.Wrap("management.list", err)}
			}
			entry.ClientID, entry.Scope, entry.Expiry = g.ClientID, strings.Join(g.Scopes, " "), row.ExpiresAt.UTC().Format(time.RFC3339)
		} else {
			var t ClientTrust
			if err := s.Store.OpenRecord(row, &t, kind); err != nil {
				return nil, &FlowError{Message: "无法加载已信任的应用。", Status: 500, Cause: failure.Wrap("management.open", err)}
			}
			entry.ClientID = t.ClientID
		}
		// 客户端已删除时列表项仍保留，仅缺失名称。
		if c, _, e := s.LoadClient(s.Store.Database().WithContext(ctx), entry.ClientID, false); e == nil {
			entry.ClientName = c.Name
			entry.LogoURL, entry.TrustLevel = c.LogoURL, c.TrustLevel
		} else if !errors.Is(e, ErrRecordNotFound) && failure.IsInfrastructure(e) {
			return nil, failure.Wrap("management.client", e)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
