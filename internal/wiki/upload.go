package wiki

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
	"io"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/security"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var zeroTime time.Time

func ticketRow(t Ticket) Record {
	return Record{ID: t.ID, Owner: t.Owner, State: t.State, ExpiresAt: t.ResultExpiresAt}
}
func (s *Service) loadTicket(db *gorm.DB, id string, lock bool) (Ticket, error) {
	var t Ticket
	_, x := s.Repo.load(db, "upload_tickets", id, &t, lock)
	return t, x
}
func (s *Service) saveTicket(db *gorm.DB, t Ticket, create bool) error {
	return s.Repo.save(db, "upload_tickets", ticketRow(t), t, create)
}
func ticketResult(t Ticket, c *Call) Object {
	// 已有最终结果的上传直接返回，避免重复生成状态。
	if t.Result != nil {
		return t.Result
	}
	out := Object{"upload_id": t.ID, "state": t.State}
	// 按状态拼装对外字段：ready 暴露上传地址，staged 提示下一步 commit。
	switch t.State {
	case "ready":
		out["upload_url"] = strings.TrimRight(c.Config.Static.Server.PublicURL, "/") + "/uploads?task_id=" + url.QueryEscape(t.ID)
		out["method"] = "PUT"
		out["headers"] = Object{"Content-Type": "application/octet-stream"}
		out["expires_at"] = t.ExpiresAt
	case "staged", "awaiting_confirmation":
		out["next_tool"] = "wiki.commit_upload"
		out["staged_expires_at"] = t.StagedExpiresAt
		if t.Warnings != nil {
			out["warnings"] = t.Warnings
		}
	}
	return out
}
func (s *Service) PrepareUpload(ctx context.Context, c *Call, i Input) (Object, error) {
	if x := require(c, "wiki:read", "wiki:upload"); x != nil {
		return nil, x
	}
	// retry_upload_id 走恢复路径：只接受该字段，其余元数据必须为空。
	if i.RetryUploadID != "" {
		if i.Filename != "" || i.MIMEType != "" || i.SizeBytes != 0 || i.SHA256 != "" || i.Comment != "" || i.Overwrite {
			return nil, errCode("INVALID_REQUEST", "Recovery accepts only retry_upload_id.")
		}
		t, x := s.loadTicket(s.Repo.Store.DB.WithContext(ctx), i.RetryUploadID, false)
		if x != nil {
			return nil, x
		}
		if t.Owner != c.principalKey() {
			return nil, errCode("NOT_FOUND", "Upload was not found.")
		}
		// unknown 状态结果不可判定，禁止再次发布，仅返回缓存结果。
		if t.State == "unknown" {
			return nil, errCode("MUTATION_RESULT_UNKNOWN", "Upload result is unknown. Inspect imageinfo; publishing is disabled.")
		}
		if t.State != "ready" {
			return ticketResult(t, c), nil
		}
		var next Ticket
		x = s.Repo.tx(ctx, func(db *gorm.DB) error {
			// 事务内加锁重读，防止并发下重复重试。
			t, x := s.loadTicket(db, i.RetryUploadID, true)
			if x != nil {
				return x
			}
			if t.State != "ready" {
				return errCode("UPLOAD_BUSY", "Upload state changed.")
			}
			next = t
			if x = s.checkUploadQuota(db, c, t, t.ID, time.Now()); x != nil {
				return x
			}
			// 作废旧 ticket，换发新 ID 并重置各项 TTL。
			next.ID = "uuid_" + security.Random()
			next.ExpiresAt = time.Now().UTC().Add(time.Duration(c.Config.App.Uploads.TaskTTL) * time.Second)
			next.ResultExpiresAt = next.ExpiresAt.Add(time.Duration(c.Config.App.Uploads.Retention) * time.Second)
			next.StagedTTL = c.Config.App.Uploads.StagedTTL
			next.RetentionTTL = c.Config.App.Uploads.Retention
			next.Fingerprint = policyFingerprint(c)
			t.State = "failed"
			if x = s.saveTicket(db, t, false); x != nil {
				return x
			}
			return s.saveTicket(db, next, true)
		})
		if x != nil {
			return nil, x
		}
		return ticketResult(next, c), nil
	}
	// 严格校验元数据：sha256 须为小写十六进制且长度 32，大小、MIME、文件名均需合规。
	digest, x := hex.DecodeString(i.SHA256)
	if x != nil || len(digest) != 32 || strings.ToLower(i.SHA256) != i.SHA256 || i.SizeBytes <= 0 || i.SizeBytes > c.Config.App.Uploads.MaxBytes || !slices.Contains(c.Config.App.Uploads.Mimes, i.MIMEType) || i.Filename == "" || len(i.Filename) > 240 || filepath.Base(i.Filename) != i.Filename || strings.ContainsAny(i.Filename, "/\\|:\x00\r\n") {
		return nil, errCode("INVALID_REQUEST", "Upload metadata is invalid.")
	}
	now := time.Now().UTC()
	t := Ticket{ID: "uuid_" + security.Random(), Owner: c.principalKey(), GrantID: c.Principal.GrantID, SiteID: c.Client.Config.SiteID, Filename: i.Filename, MIMEType: i.MIMEType, SHA256: i.SHA256, Comment: i.Comment, SizeBytes: i.SizeBytes, Overwrite: i.Overwrite, State: "ready", Fingerprint: policyFingerprint(c), ExpiresAt: now.Add(time.Duration(c.Config.App.Uploads.TaskTTL) * time.Second), ResultExpiresAt: now.Add(time.Duration(c.Config.App.Uploads.Retention) * time.Second)}
	t.StagedTTL = c.Config.App.Uploads.StagedTTL
	t.RetentionTTL = c.Config.App.Uploads.Retention
	x = s.Repo.tx(ctx, func(db *gorm.DB) error {
		if x := s.checkUploadQuota(db, c, t, "", time.Now()); x != nil {
			return x
		}
		return s.saveTicket(db, t, true)
	})
	if x != nil {
		return nil, x
	}
	return ticketResult(t, c), nil
}
func (s *Service) checkUploadQuota(db *gorm.DB, c *Call, t Ticket, exclude string, now time.Time) error {
	if x := s.Repo.quota(db, security.Hash("uploads:global")); x != nil {
		return x
	}
	var rows []Record
	if x := db.Table("upload_tickets").Where("state IN ?", []string{"ready", "receiving", "staged", "awaiting_confirmation", "publishing", "unknown"}).Find(&rows).Error; x != nil {
		return x
	}
	// 统计占用中的 ticket：仍在接收/发布、已落盘或未过期的 ready 才计预留空间。
	var total, owned int64
	count := 0
	for _, r := range rows {
		if r.ID == exclude {
			continue
		}
		var u Ticket
		if x := s.Repo.Store.Vault.Open(r.Payload, &u, "upload_tickets:"+r.ID); x != nil {
			return x
		}
		reserved := u.State == "receiving" || u.State == "publishing" || u.Path != "" || u.State == "ready" && u.ExpiresAt.After(now)
		if !reserved {
			continue
		}
		total += u.SizeBytes
		if u.Owner == t.Owner {
			owned += u.SizeBytes
			count++
		}
	}
	// 同时限制活动 ticket 数量、单主体预留额度和全局预留额度。
	if count >= c.Config.App.Uploads.MaxActive || owned+t.SizeBytes > c.Config.App.Uploads.ReservedPrincipal || total+t.SizeBytes > c.Config.App.Uploads.ReservedGlobal {
		return errCode("UPLOAD_QUOTA_EXCEEDED", "Upload space or ticket quota is exceeded.")
	}
	return nil
}
func (s *Service) Routes(r chi.Router) { r.HandleFunc("/uploads", s.Receive) }
func uploadHTTP(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Service) Receive(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		w.Header().Set("Allow", "PUT")
		uploadHTTP(w, 405, Object{"error": "METHOD_NOT_ALLOWED"})
		return
	}
	// 仅接受 PUT，查询串须恰好是一个 uuid_ 前缀的 task_id。
	q, x := url.ParseQuery(r.URL.RawQuery)
	if x != nil || len(q) != 1 || len(q["task_id"]) != 1 || !strings.HasPrefix(q.Get("task_id"), "uuid_") {
		uploadHTTP(w, 400, Object{"error": "INVALID_REQUEST"})
		return
	}
	id := q.Get("task_id")
	cfg := config.GetConfig()
	var t Ticket
	duplicate := false
	var previous string
	// 加锁占用 ticket：staged 视为重复上传，仅 ready 才可进入接收状态。
	x = s.Repo.tx(r.Context(), func(db *gorm.DB) error {
		var e error
		t, e = s.loadTicket(db, id, true)
		if e != nil {
			return e
		}
		if !t.ExpiresAt.After(time.Now()) {
			return errCode("UPLOAD_EXPIRED", "Upload URL expired.")
		}
		if t.State == "staged" {
			duplicate = true
		} else if t.State != "ready" {
			return errCode("UPLOAD_BUSY", "Upload is busy or has already been published.")
		}
		previous = t.State
		t.PreviousState = previous
		t.State = "receiving"
		return s.saveTicket(db, t, false)
	})
	if x != nil {
		// 把内部错误码映射成对应的 HTTP 状态码。
		status := 409
		switch failure.Describe(x).Code {
		case "NOT_FOUND":
			status = 404
		case "UPLOAD_EXPIRED":
			status = 410
		}
		uploadHTTP(w, status, Object{"error": failure.Describe(x).Code})
		return
	}
	// restore 在接收失败时把 ticket 回滚到进入前的状态。
	restore := func() {
		_ = s.Repo.tx(context.WithoutCancel(r.Context()), func(db *gorm.DB) error {
			u, e := s.loadTicket(db, id, true)
			if e != nil {
				return e
			}
			if u.State == "receiving" {
				u.State = previous
				return s.saveTicket(db, u, false)
			}
			return nil
		})
	}
	// 只接受 application/octet-stream，临时目录以 0700 创建。
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		restore()
		uploadHTTP(w, 422, Object{"error": "UPLOAD_TYPE_MISMATCH"})
		return
	}
	if x = os.MkdirAll(cfg.App.Uploads.TempPath, 0700); x != nil {
		restore()
		uploadHTTP(w, 500, Object{"error": "STORAGE_UNAVAILABLE"})
		return
	}
	f, x := os.CreateTemp(cfg.App.Uploads.TempPath, "upload-*")
	if x != nil {
		restore()
		uploadHTTP(w, 500, Object{"error": "STORAGE_UNAVAILABLE"})
		return
	}
	path := f.Name()
	t.ReceivingPath = path
	if x = s.saveTicket(s.Repo.Store.DB.WithContext(r.Context()), t, false); x != nil {
		f.Close()
		_ = os.Remove(path)
		restore()
		uploadHTTP(w, 500, Object{"error": "STORAGE_UNAVAILABLE"})
		return
	}
	// 边写盘边算 sha256，LimitReader 多读 1 字节用于判超长，读取上限 120 秒。
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(120 * time.Second))
	_ = f.Chmod(0600)
	hash := sha256.New()
	size, x := io.Copy(io.MultiWriter(f, hash), io.LimitReader(r.Body, t.SizeBytes+1))
	closeErr := f.Close()
	if x == nil {
		x = closeErr
	}
	// 大小或摘要不符即删除临时文件并回滚；超出预期大小返回 413。
	if x != nil || size != t.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != t.SHA256 {
		_ = os.Remove(path)
		restore()
		status := 422
		if size > t.SizeBytes {
			status = 413
		}
		uploadHTTP(w, status, Object{"error": "UPLOAD_BODY_MISMATCH"})
		return
	}
	file, x := os.Open(path)
	if x != nil {
		_ = os.Remove(path)
		restore()
		uploadHTTP(w, 500, Object{"error": "STORAGE_UNAVAILABLE"})
		return
	}
	// 嗅探前 512 字节判断真实 MIME，防止声明与实际内容不符。
	head := make([]byte, 512)
	n, _ := file.Read(head)
	file.Close()
	actual := http.DetectContentType(head[:n])
	// SVG 文本探测不可靠，需解析 XML 根元素确认，最多读取 64KB。
	if t.MIMEType == "image/svg+xml" {
		svg, e := os.Open(path)
		if e == nil {
			decoder := xml.NewDecoder(io.LimitReader(svg, min(t.SizeBytes, 65536)))
			for {
				token, e := decoder.Token()
				if e != nil {
					break
				}
				if root, ok := token.(xml.StartElement); ok {
					if root.Name.Local == "svg" && (root.Name.Space == "" || root.Name.Space == "http://www.w3.org/2000/svg") {
						actual = "image/svg+xml"
					}
					break
				}
			}
			svg.Close()
		}
	}
	// 去掉 charset 等参数后与声明的类型比对。
	base, _, _ := mime.ParseMediaType(actual)
	if base != t.MIMEType {
		_ = os.Remove(path)
		restore()
		uploadHTTP(w, 422, Object{"error": "UPLOAD_TYPE_MISMATCH"})
		return
	}
	// 重复上传丢弃本次临时文件；首次上传登记路径并计算 staged 过期时间。
	if duplicate {
		_ = os.Remove(path)
	} else {
		t.Path = path
		t.StagedExpiresAt = time.Now().UTC().Add(time.Duration(t.StagedTTL) * time.Second)
	}
	t.State = "staged"
	t.ReceivingPath = ""
	t.PreviousState = ""
	// 提交前再次确认仍持有 receiving 占用，避免并发覆盖。
	x = s.Repo.tx(context.WithoutCancel(r.Context()), func(db *gorm.DB) error {
		u, e := s.loadTicket(db, id, true)
		if e != nil {
			return e
		}
		if u.State != "receiving" {
			return errCode("UPLOAD_BUSY", "Reception lost occupancy.")
		}
		return s.saveTicket(db, t, false)
	})
	if x != nil {
		if !duplicate {
			_ = os.Remove(path)
		}
		restore()
		uploadHTTP(w, 500, Object{"error": "STORAGE_UNAVAILABLE"})
		return
	}
	// 首次上传返回 201，重复上传返回 200。
	status := 201
	if duplicate {
		status = 200
	}
	uploadHTTP(w, status, Object{"upload_id": id, "state": "staged", "staged_expires_at": t.StagedExpiresAt, "next_tool": "wiki.commit_upload"})
}
func (s *Service) CommitUpload(ctx context.Context, c *Call, i Input) (Object, error) {
	if x := require(c, "wiki:read", "wiki:upload"); x != nil {
		return nil, x
	}
	var t Ticket
	previous := ""
	// 加锁校验所有权、保留期与状态，合法则置为 publishing 进入提交。
	x := s.Repo.tx(ctx, func(db *gorm.DB) error {
		var x error
		t, x = s.loadTicket(db, i.UploadID, true)
		if x != nil {
			return x
		}
		if t.Owner != c.principalKey() || t.GrantID != c.Principal.GrantID {
			return errCode("NOT_FOUND", "Upload was not found.")
		}
		if !t.ResultExpiresAt.After(time.Now()) {
			return errCode("UPLOAD_EXPIRED", "Upload result retention expired.")
		}
		if t.State == "completed" {
			return nil
		}
		if t.State == "unknown" {
			return errCode("MUTATION_RESULT_UNKNOWN", "Inspect imageinfo; this upload cannot be published again.")
		}
		if t.State != "staged" && t.State != "awaiting_confirmation" {
			return errCode("UPLOAD_BUSY", "Upload has no available staged file.")
		}
		if !t.StagedExpiresAt.After(time.Now()) {
			return errCode("UPLOAD_EXPIRED", "Staged upload expired.")
		}
		previous = t.State
		t.State = "publishing"
		t.PreviousState = previous
		t.Sent = false
		return s.saveTicket(db, t, false)
	})
	if x != nil {
		return nil, x
	}
	// 已完成的请求幂等返回缓存结果。
	if t.State == "completed" {
		return t.Result, nil
	}
	challenge := ""
	// restore 按 cause 是否代表结果未知来回滚状态并结算 challenge。
	restore := func(cause error, unknown bool) (Object, error) {
		// 结果未知时冻结为 unknown，否则恢复到提交前状态。
		state := previous
		if unknown {
			state = "unknown"
		}
		t.State = state
		saveErr := s.saveTicket(s.Repo.Store.DB.WithContext(context.WithoutCancel(ctx)), t, false)
		// 同步结算 challenge：未知则冻结，否则释放为 ready。
		chstate := "ready"
		if unknown {
			chstate = "frozen"
		}
		_ = s.finishChallenge(context.WithoutCancel(ctx), challenge, chstate)
		if saveErr != nil {
			return nil, saveErr
		}
		if unknown {
			return nil, detail("MUTATION_RESULT_UNKNOWN", "Inspect imageinfo before further action; publishing this ticket is disabled.", Object{"reason": failure.Describe(cause).Code})
		}
		return nil, cause
	}
	if x = s.site(c, t.SiteID); x != nil {
		return restore(x, false)
	}
	// 策略指纹变化意味着配置已变，需重新准备上传。
	if t.Fingerprint != policyFingerprint(c) {
		return restore(errCode("UPLOAD_POLICY_CHANGED", "Upload policy changed. Prepare a new upload."), false)
	}
	// 检查目标文件是否已存在，决定是否需要覆盖授权。
	snap, x := s.snapshot(ctx, c, PageRef{Title: "File:" + t.Filename}, false)
	if x != nil {
		return restore(x, false)
	}
	id := number(snap["page_id"])
	if id > 0 && !t.Overwrite {
		return restore(errCode("FILE_EXISTS", "Prepare an upload with overwrite=true to replace this file."), false)
	}
	// 覆盖已有文件前走 uploadOverwrite 策略挑战。
	if id > 0 {
		challenge, x = s.policy(ctx, c, "uploadOverwrite", "commit_upload", id, Object{"siteId": t.SiteID, "upload_id": t.ID, "page_id": id, "filename": t.Filename, "mime_type": t.MIMEType, "size_bytes": t.SizeBytes, "sha256": t.SHA256, "comment": t.Comment, "overwrite": t.Overwrite}, i.ChallengeResponses, zeroTime)
		if x != nil {
			return restore(x, false)
		}
	}
	// 仅在上次返回 warning 后允许确认，且必须逐项匹配。
	if previous != "awaiting_confirmation" && len(i.ConfirmedWarnings) > 0 {
		return restore(errCode("INVALID_REQUEST", "Warnings can only be confirmed after they have been returned."), false)
	}
	if previous == "awaiting_confirmation" {
		// 确认集合必须与返回的 warning 键完全一致。
		expected := make([]string, 0, len(t.Warnings))
		for k := range t.Warnings {
			expected = append(expected, k)
		}
		if len(i.ConfirmedWarnings) != len(expected) {
			return restore(detail("UPLOAD_WARNINGS", "Explicitly confirm every returned warning.", Object{"warnings": t.Warnings}), false)
		}
		for _, k := range expected {
			if !slices.Contains(i.ConfirmedWarnings, k) {
				return restore(errCode("INVALID_REQUEST", "Warning confirmation does not match."), false)
			}
		}
	}
	// 发布前重新校验磁盘文件大小与摘要，防止 staged 文件被篡改。
	file, x := os.ReadFile(t.Path)
	if x != nil {
		return restore(errCode("UPLOAD_FILE_UNAVAILABLE", "Staged file is unavailable."), false)
	}
	h := sha256.Sum256(file)
	if int64(len(file)) != t.SizeBytes || hex.EncodeToString(h[:]) != t.SHA256 {
		return restore(errCode("UPLOAD_BODY_MISMATCH", "Staged file does not match its intent."), false)
	}
	// 获取 CSRF token 并组装上传表单；确认过 warning 的带上 ignorewarnings。
	token, x := c.Client.CSRF(ctx, c.Credentials)
	if x != nil {
		return restore(x, false)
	}
	q := url.Values{"action": {"upload"}, "filename": {t.Filename}, "comment": {t.Comment}, "token": {token}}
	if previous == "awaiting_confirmation" {
		q.Set("ignorewarnings", "1")
	}
	// 标记已发送，配合恢复逻辑判定结果是否可能未知。
	t.Sent = true
	t.ChallengeID = challenge
	if x = s.saveTicket(s.Repo.Store.DB.WithContext(ctx), t, false); x != nil {
		return restore(x, false)
	}
	// 网络或响应异常视为结果未知，进入 unknown 保护。
	v, x := c.Client.Upload(ctx, c.Credentials, q, file, t.Filename, t.MIMEType)
	if x != nil {
		return restore(x, failure.Describe(x).Code == "API_NETWORK_ERROR" || failure.Describe(x).Code == "UPSTREAM_RESPONSE_INVALID")
	}
	// Wiki 返回 Warning 时需用户逐项确认，仅接受可枚举的 warning 集合。
	up := object(v["upload"])
	if up["result"] == "Warning" {
		t.State = "awaiting_confirmation"
		t.Warnings = object(up["warnings"])
		if len(t.Warnings) == 0 {
			return restore(errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned an empty warning set."), true)
		}
		for warning := range t.Warnings {
			switch warning {
			case "exists", "exists-normalized", "was-deleted", "duplicate", "duplicate-archive", "badfilename", "large-file", "largefile", "filetype-unwanted", "emptyfile":
			default:
				return restore(errCode("UPLOAD_WARNING_UNSUPPORTED", "Wiki returned a warning that cannot be confirmed by this gateway."), false)
			}
		}
		if x = s.saveTicket(s.Repo.Store.DB.WithContext(context.WithoutCancel(ctx)), t, false); x != nil {
			return nil, x
		}
		_ = s.finishChallenge(context.WithoutCancel(ctx), challenge, "ready")
		return nil, detail("UPLOAD_WARNINGS", "Confirm the returned warnings and call wiki.commit_upload again.", Object{"upload_id": t.ID, "warnings": t.Warnings, "next_tool": "wiki.commit_upload"})
	}
	// 响应结果非 Success 说明发布结果不可信，按未知处理。
	if up["result"] != "Success" {
		return restore(errCode("UPSTREAM_RESPONSE_INVALID", "Wiki did not confirm uploading."), true)
	}
	// 发布成功：转 completed 并按 retention 刷新结果过期时间，清理 staged 文件。
	t.State = "completed"
	t.ResultExpiresAt = time.Now().UTC().Add(time.Duration(t.RetentionTTL) * time.Second)
	t.Result = Object{"upload_id": t.ID, "state": "completed", "filename": up["filename"], "imageinfo": up["imageinfo"]}
	if x = s.saveTicket(s.Repo.Store.DB.WithContext(context.WithoutCancel(ctx)), t, false); x != nil {
		return nil, x
	}
	_ = os.Remove(t.Path)
	if x = s.finishChallenge(context.WithoutCancel(ctx), challenge, "consumed"); x != nil {
		return nil, x
	}
	return t.Result, nil
}
