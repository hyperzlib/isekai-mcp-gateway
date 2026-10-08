package wiki

import (
	"context"
	"isekai-mcp-gateway/internal/failure"
	"os"
	"path/filepath"
	"strings"
)

// The upload directory is private application storage. Only files created with
// the reserved upload- prefix and absent from all tickets are orphan candidates.
func (s *Service) PrepareRuntime(ctx context.Context) error {
	cfg := s.Auth.Upstream.Get().Config
	root := cfg.App.Uploads.TempPath
	if x := os.MkdirAll(root, 0700); x != nil {
		return failure.Storage("uploads.directory", x)
	}
	// 通过临时文件验证上传目录可写。
	test, x := os.CreateTemp(root, ".write-check-*")
	if x != nil {
		return failure.Storage("uploads.directory", x)
	}
	name := test.Name()
	if x = test.Close(); x != nil {
		return x
	}
	if x = os.Remove(name); x != nil {
		return x
	}
	var rows []Record
	if x = s.Repo.Store.DB.WithContext(ctx).Table("upload_tickets").Find(&rows).Error; x != nil {
		return x
	}

	// 汇总所有票据占用的路径，避免误删仍在使用的文件。
	used := map[string]bool{}
	for _, r := range rows {
		var t Ticket
		if x = s.Repo.Store.Vault.Open(r.Payload, &t, "upload_tickets:"+r.ID); x != nil {
			return x
		}
		used[t.Path] = true
		used[t.ReceivingPath] = true
	}
	entries, x := os.ReadDir(root)
	if x != nil {
		return x
	}

	// 仅清理 upload- 前缀且未被任何票据引用的普通文件。
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "upload-") || !entry.Type().IsRegular() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if used[path] {
			continue
		}

		// 文件已被并发删除时忽略，其余错误上报。
		if x = os.Remove(path); x != nil && !os.IsNotExist(x) {
			return x
		}
	}
	return nil
}
