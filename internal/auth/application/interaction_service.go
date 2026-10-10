package application

import (
	"context"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/persistence"
)

func (s *Service) InteractionInfo(ctx context.Context, id string) (*Interaction, persistence.Row, error) {
	return s.LoadInteraction(s.Store.Database().WithContext(ctx), id, false)
}
func (s *Service) ClientInfo(ctx context.Context, id string) (*Client, error) {
	c, _, err := s.LoadClient(s.Store.Database().WithContext(ctx), id, false)
	if err != nil {
		return nil, lookupError(err, domain.ErrInvalidClient())
	}
	return c, nil
}
func (s *Service) IsTrusted(ctx context.Context, account string, client *Client) (bool, error) {
	return s.Trusted(s.Store.Database().WithContext(ctx), account, client)
}
func (s *Service) CancelInteraction(ctx context.Context, id, browser string) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		a, row, err := s.LoadInteraction(tx, id, true)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		// 已完成同意或已签发授权码的交互不允许取消。
		if a.Browser != browser {
			return interactionError("本次授权不属于当前浏览器会话，请在发起连接的浏览器中继续，或返回应用重新发起连接。")
		}
		if row.State == "consented" || row.State == "code" || row.State == "consumed" {
			return interactionError("本次授权已完成，无法再取消。请返回应用查看连接结果。")
		}
		return tx.Table(persistence.Table("interactions")).Where("id = ?", row.ID).Update("state", "cancelled").Error()
	})
}

// 二次确认等待时长按客户端信任等级区分：已信任无需等待，已知 3 秒，未知 10 秒。
func trustCountdown(level domain.TrustLevel) time.Duration {
	switch level {
	case domain.TrustTrusted:
		return 0
	case domain.TrustKnown:
		return 3 * time.Second
	default:
		return 10 * time.Second
	}
}

func (s *Service) BeginTrustCountdown(ctx context.Context, a *Interaction, c *Client) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		fresh, row, err := s.LoadInteraction(tx, a.ID, true)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		if fresh.Browser != a.Browser {
			return interactionError("本次授权的浏览器会话已变化，请返回应用重新发起连接。")
		}
		if row.State != "authenticated" {
			return interactionError("本次授权当前无法开始信任确认，请刷新页面并完成登录后重试。")
		}
		if fresh.AccountID != a.AccountID {
			return interactionError("本次授权的登录账号已变化。请返回应用重新发起连接。")
		}
		// 首次进入信任倒计时时按等级设置等待，之后保持不变。
		if fresh.TrustAfter.IsZero() {
			fresh.TrustAfter = time.Now().UTC().Add(trustCountdown(c.TrustLevel))
		}
		a.TrustAfter = fresh.TrustAfter
		return s.Store.Put(tx, "interactions", a.ID, fresh, row)
	})
}

// interactionError 为交互校验失败提供具体原因与恢复操作。
func interactionError(message string) *FlowError {
	return &FlowError{Code: "INVALID_INTERACTION", Message: message, Status: 400}
}
