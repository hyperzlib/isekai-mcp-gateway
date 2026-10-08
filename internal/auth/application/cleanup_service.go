package application

import (
	"context"
	"time"

	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
)

func (s *Service) Cleanup(ctx context.Context) error {
	now := time.Now().UTC()
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		if err := s.registryLock(tx); err != nil {
			return err
		}
		for _, kind := range persistence.Tables {
			// accounts、clients、client_trusts 不按过期时间直接删除，另行处理。
			if kind == "accounts" || kind == "clients" || kind == "client_trusts" {
				continue
			}
			if err := tx.Table(persistence.Table(kind)).Where("expires_at < ?", now).Delete(&persistence.Row{}).Error(); err != nil {
				return err
			}
		}
		// 回收已过期的待处理客户端，以及长期空闲的活跃客户端。
		var rows []persistence.Row
		if err := tx.Table(persistence.Table("clients")).Where("(state = ? AND expires_at < ?) OR (state = ? AND updated_at < ?)", "pending", now, "active", now.Add(-Seconds(s.Config.Static.Auth.Gateway.Registration.IdleTTL))).Lock().Find(&rows).Error(); err != nil {
			return err
		}
		for _, row := range rows {
			var n int64
			if err := tx.Table(persistence.Table("grants")).Where("client_id = ? AND state = ? AND expires_at > ?", row.ClientID, "active", now).Count(&n).Error(); err != nil {
				return err
			}
			// 存在活跃授权时保留客户端。
			if n > 0 {
				continue
			}
			if err := tx.Table(persistence.Table("clients")).Where("id = ?", row.ID).Delete(&persistence.Row{}).Error(); err != nil {
				return err
			}
			if err := tx.Table(persistence.Table("client_trusts")).Where("client_id = ?", row.ClientID).Delete(&persistence.Row{}).Error(); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) RunCleanup(ctx context.Context) {
	// 每分钟执行一次清理，直到上下文被取消。
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Cleanup(ctx); err != nil {
				failure.Log(ctx, "oauth.cleanup", failure.Wrap("oauth.cleanup", err))
			}
		}
	}
}
