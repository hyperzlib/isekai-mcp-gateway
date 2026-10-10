package wiki

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gorm.io/gorm"
	"io"
	"isekai-mcp-gateway/internal/failure"
	"os"
	"time"
)

// Recover runs before accepting requests; no interrupted write is resent.
func (s *Service) Recover(ctx context.Context) error {
	if x := s.restoreSites(ctx); x != nil {
		return x
	}
	// 复位 loading 状态的编辑会话：根据尝试记录推断中断原因后转为可恢复状态。
	var rows []Record
	if x := s.Repo.Store.DB.WithContext(ctx).Table("edit_sessions").Where("state = 'loading'").Find(&rows).Error; x != nil {
		return x
	}
	for _, r := range rows {
		if x := s.Repo.tx(ctx, func(db *gorm.DB) error {
			e, x := s.Repo.edit(db, r.ID, true)
			if x != nil {
				return x
			}
			if e.State != "loading" {
				return nil
			}
			// commit 中断需区分：已有结果、已知原因或已发送但结果未知。
			if e.Operation == "commit" {
				reason := "save_failed"
				var a Attempt
				_, x := s.Repo.load(db, "commit_attempts", fmt.Sprintf("%s:%d", e.ID, e.Version), &a, false)
				if x == nil {
					if a.Result != nil {
						reason = "saved_sync_failed"
					} else if a.Reason != "" {
						reason = a.Reason
					} else if a.Sent {
						reason = "result_unknown"
					}
				}
				e.State = "unsync"
				e.Error = sessionError(reason, "Interrupted submission requires wiki.pull_edit.")
			} else {
				e.State = e.PreviousState
			}
			e.Operation = ""
			if e.CloseRequested {
				endEdit(&e)
			}
			return s.Repo.saveEdit(db, e, false)
		}); x != nil {
			return x
		}
	}
	// 复位收据：publishing 判断结果是否未知，receiving 清理临时文件。
	rows = nil
	if x := s.Repo.Store.DB.WithContext(ctx).Table("upload_tickets").Where("state IN ?", []string{"receiving", "publishing", "staged", "awaiting_confirmation"}).Find(&rows).Error; x != nil {
		return x
	}
	for _, r := range rows {
		if x := s.Repo.tx(ctx, func(db *gorm.DB) error {
			t, x := s.loadTicket(db, r.ID, true)
			if x != nil {
				return x
			}
			// publishing 若已发送则结果未知，否则回退到进入前状态。
			if t.State == "publishing" {
				if t.Sent {
					t.State = "unknown"
				} else {
					t.State = t.PreviousState
					if t.State == "" {
						t.State = "staged"
					}
				}
			} else if t.State == "receiving" {
				// receiving 删除半途文件，回退到 ready，已有 staged 文件则保留。
				if t.ReceivingPath != "" {
					if x = os.Remove(t.ReceivingPath); x != nil && !os.IsNotExist(x) {
						return x
					}
					t.ReceivingPath = ""
				}
				t.State = "ready"
				if t.Path != "" {
					t.State = "staged"
				}
			}
			// 对 staged/awaiting_confirmation 重新校验磁盘文件，失效则判失败并清理。
			if t.State == "staged" || t.State == "awaiting_confirmation" {
				f, x := os.Open(t.Path)
				valid := false
				if x == nil {
					h := sha256.New()
					size, x := io.Copy(h, io.LimitReader(f, t.SizeBytes+1))
					f.Close()
					valid = x == nil && size == t.SizeBytes && hex.EncodeToString(h.Sum(nil)) == t.SHA256
				}
				if !valid {
					t.State = "failed"
					if t.Path != "" {
						_ = os.Remove(t.Path)
						t.Path = ""
					}
				}
			}
			return s.saveTicket(db, t, false)
		}); x != nil {
			return x
		}
	}
	return s.recoverChallenges(ctx)
}
func (s *Service) recoverChallenges(ctx context.Context) error {
	var rows []Record
	if x := s.Repo.Store.DB.WithContext(ctx).Table("challenges").Where("state = 'reserved'").Find(&rows).Error; x != nil {
		return x
	}
	// 依据工具与对应记录推断 reserved challenge 的真实归属状态。
	for _, row := range rows {
		var ch Challenge
		if _, x := s.Repo.load(s.Repo.Store.DB.WithContext(ctx), "challenges", row.ID, &ch, false); x != nil {
			return x
		}
		// 默认冻结为 frozen，只有确认无副作用时才释放为 ready 或标记 consumed。
		state := "frozen"
		switch ch.Tool {
		case "close_edit":
			// close_edit：会话已结束或已请求关闭则视为已消费。
			state = "ready"
			e, x := s.Repo.edit(s.Repo.Store.DB.WithContext(ctx), str(ch.Params["edit_id"]), false)
			if x == nil && (e.EndedAt != nil || e.CloseRequested) {
				state = "consumed"
			}
		case "commit_edit":
			// commit_edit：遍历尝试记录，按结果/原因/是否发送判定最终状态。
			state = "ready"
			var attempts []Record
			if x := s.Repo.Store.DB.WithContext(ctx).Table("commit_attempts").Where("id LIKE ?", str(ch.Params["edit_id"])+":%").Find(&attempts).Error; x != nil {
				return x
			}
			for _, r := range attempts {
				var a Attempt
				if x := s.Repo.Store.Vault.Open(r.Payload, &a, "commit_attempts:"+r.ID); x != nil {
					return x
				}
				if a.ChallengeID == ch.ID {
					if a.Result != nil {
						state = "consumed"
					} else if a.Reason == "save_failed" {
						state = "ready"
					} else if a.Sent {
						state = "frozen"
					}
				}
			}
		case "commit_upload":
			// commit_upload：按 ticket 是否完成或未知决定 challenge 状态。
			state = "ready"
			t, x := s.loadTicket(s.Repo.Store.DB.WithContext(ctx), str(ch.Params["upload_id"]), false)
			if x == nil && t.ChallengeID == ch.ID {
				if t.State == "completed" {
					state = "consumed"
				} else if t.State == "unknown" {
					state = "frozen"
				}
			}
		}
		if x := s.finishChallenge(ctx, ch.ID, state); x != nil {
			return x
		}
	}
	return nil
}
func (s *Service) Cleanup(ctx context.Context) error {
	now := time.Now().UTC()
	var rows []Record
	if x := s.Repo.Store.DB.WithContext(ctx).Table("upload_tickets").Where("state NOT IN ?", []string{"receiving", "publishing"}).Find(&rows).Error; x != nil {
		return x
	}
	for _, r := range rows {
		if x := s.Repo.tx(ctx, func(db *gorm.DB) error {
			t, x := s.loadTicket(db, r.ID, true)
			if x != nil {
				return x
			}
			if t.State == "receiving" || t.State == "publishing" {
				return nil
			}
			// 不同状态使用不同过期基准：staged 看 staged 过期，其余看任务过期。
			expired := !t.ExpiresAt.After(now)
			if t.State == "staged" || t.State == "awaiting_confirmation" {
				expired = !t.StagedExpiresAt.After(now)
			}
			// 过期且有文件时删除文件，并把非终态置为 failed。
			if expired && t.Path != "" {
				if x = os.Remove(t.Path); x != nil && !os.IsNotExist(x) {
					return x
				}
				t.Path = ""
				if t.State != "completed" && t.State != "unknown" {
					t.State = "failed"
				}
				if x = s.saveTicket(db, t, false); x != nil {
					return x
				}
			}
			// 结果保留期也过期后彻底删除 ticket 记录。
			if expired && !t.ResultExpiresAt.After(now) {
				return db.Table("upload_tickets").Where("id = ?", t.ID).Delete(&Record{}).Error
			}
			return nil
		}); x != nil {
			return x
		}
	}
	// 清理其他过期表；commit_attempts 需跳过仍处于 loading 的会话。
	for _, table := range []string{"commit_attempts", "challenges", "edit_sessions"} {
		q := s.Repo.Store.DB.WithContext(ctx).Table(table).Where("expires_at <= ? AND state NOT IN ?", now, []string{"loading", "reserved"})
		if table == "commit_attempts" {
			q = q.Where("NOT EXISTS (SELECT 1 FROM edit_sessions e WHERE e.id = SUBSTRING_INDEX(commit_attempts.id, ':', 1) AND e.state = 'loading')")
		}
		if x := q.Delete(&Record{}).Error; x != nil {
			return x
		}
	}
	return nil
}
func (s *Service) RunCleanup(ctx context.Context) {
	// 每分钟触发一次清理，ctx 取消时退出。
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Cleanup(ctx); err != nil {
				failure.Log(ctx, "wiki.cleanup", failure.Wrap("wiki.cleanup", err))
			}
		}
	}
}
