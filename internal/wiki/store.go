package wiki

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
	"time"
)

type Record struct {
	ID         string `gorm:"primaryKey"`
	Owner      string
	State      string
	Version    int64
	ExpiresAt  time.Time
	UpdatedAt  time.Time
	Payload    []byte
	HasChanges bool
}
type Repository struct{ Store *persistence.Store }

func (r *Repository) load(db *gorm.DB, table, id string, value any, lock bool) (Record, error) {
	var row Record
	q := db.Table(table).Where("id = ?", id)

	// lock=true 时使用 SELECT ... FOR UPDATE 行锁，供事务内串行化访问。
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	e := q.Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return row, errCode("NOT_FOUND", "The requested object was not found.")
	}
	if e != nil {
		return row, failure.Storage("wiki.load", e)
	}
	e = r.Store.Vault.Open(row.Payload, value, table+":"+id)
	return row, e
}
func (r *Repository) save(db *gorm.DB, table string, row Record, value any, create bool) error {
	payload, e := r.Store.Vault.Seal(value, table+":"+row.ID)
	if e != nil {
		return e
	}
	row.Payload = payload
	row.UpdatedAt = time.Now().UTC()

	// create 决定插入还是定点更新，更新仅覆盖受管字段。
	q := db.Table(table)
	if create {
		e = q.Create(&row).Error
	} else {
		e = q.Where("id = ?", row.ID).Select("owner", "state", "version", "expires_at", "updated_at", "payload", "has_changes").Updates(&row).Error
	}
	return failure.Storage("wiki.save", e)
}

// pack 与 unpack 在持久化结构与领域结构间搬移私有草稿字段。
func pack(e Edit) editPayload {
	return editPayload{e, e.Origin, e.Content, e.Owner, e.SiteID, e.StartTimestamp, e.PreviousState, e.Operation, e.Marker}
}
func unpack(p editPayload) Edit {
	e := p.Edit
	e.Origin = p.Origin
	e.Content = p.Content
	e.Owner = p.Owner
	e.SiteID = p.SiteID
	e.StartTimestamp = p.StartTimestamp
	e.PreviousState = p.PreviousState
	e.Operation = p.Operation
	e.Marker = p.Marker
	return e
}
func (r *Repository) edit(db *gorm.DB, id string, lock bool) (Edit, error) {
	var p editPayload
	_, e := r.load(db, "edit_sessions", id, &p, lock)
	return unpack(p), e
}
func (r *Repository) saveEdit(db *gorm.DB, e Edit, create bool) error {
	// 内容与原始草稿不同才算有变更。
	e.HasChanges = e.Content != e.Origin
	state := e.State

	// 存在结束时间时状态归一为 ended。
	if e.EndedAt != nil {
		state = "ended"
	}
	return r.save(db, "edit_sessions", Record{ID: e.ID, Owner: e.Owner, State: state, Version: e.Version, ExpiresAt: e.ExpiresAt, HasChanges: e.HasChanges}, pack(e), create)
}

// tx 将操作包裹在带上下文的数据库事务中。
func (r *Repository) tx(ctx context.Context, fn func(*gorm.DB) error) error {
	return r.Store.DB.WithContext(ctx).Transaction(fn)
}

// A stable row locks a quota domain even when it currently contains no objects.
func (r *Repository) quota(db *gorm.DB, key string) error {
	// 先确保锁行存在，再对其加行锁以串行化同一配额的写入。
	if e := db.Exec("INSERT IGNORE INTO wiki_quota_locks (id) VALUES (?)", key).Error; e != nil {
		return e
	}
	var v struct{ ID string }
	return db.Table("wiki_quota_locks").Where("id = ?", key).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&v).Error
}
