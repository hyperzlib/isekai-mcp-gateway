package persistence

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"isekai-mcp-gateway/internal/security"
)

var Tables = []string{"clients", "accounts", "sessions", "interactions", "login_transactions", "client_trusts", "grants", "authorization_codes", "access_tokens", "refresh_tokens"}

// Query metadata and business JSON are plaintext. Credentials are stored in
// a separate AEAD payload bound to the table, lookup hash and format version.
type Row struct {
	ID               string `gorm:"primaryKey;type:char(64)"`
	AccountID        string
	ClientID         string
	GrantID          string
	State            string
	ExpiresAt        time.Time
	UpdatedAt        time.Time
	Payload          []byte
	EncryptedPayload []byte
	Fingerprint      string  `gorm:"->"`
	ClientName       string  `gorm:"->"`
	LogoURL          *string `gorm:"->"`
	TrustLevel       string  `gorm:"->"`
}

// Table 将逻辑表名映射到带 oauth_ 前缀的物理表名。
func Table(kind string) string {
	for _, t := range Tables {
		if t == kind {
			return "oauth_" + t
		}
	}
	// 未知表名属于编程错误，直接 panic。
	panic("unknown storage table")
}

func (s *Store) encode(kind, key string, value any, meta Row) (Row, error) {
	// 外部 key 哈希后作为主键，避免明文标识入库。
	meta.ID, meta.UpdatedAt = security.Hash(key), time.Now().UTC()
	// 凭据拆分到绑定表名、主键与版本的独立密文。
	p, encrypted, err := s.splitPayload(kind, value, meta.ID)
	meta.Payload, meta.EncryptedPayload = p, encrypted
	return meta, err
}

func (s *Store) Put(db *gorm.DB, kind, key string, value any, meta Row) error {
	r, err := s.encode(kind, key, value, meta)
	if err != nil {
		return err
	}
	// upsert 语义：主键冲突时整体更新。
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table(Table(kind)).Clauses(clause.OnConflict{UpdateAll: true}).Create(&r).Error; err != nil {
			return err
		}
		return writeClientColumns(tx, kind, r)
	})
}

func (s *Store) Create(db *gorm.DB, kind, key string, value any, meta Row) error {
	r, err := s.encode(kind, key, value, meta)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table(Table(kind)).Create(&r).Error; err != nil {
			return err
		}
		return writeClientColumns(tx, kind, r)
	})
}

func (s *Store) Get(db *gorm.DB, kind, key string, value any, lock bool) (Row, error) {
	q := db.Table(Table(kind)).Where("id = ?", security.Hash(key))
	// lock 使用 SELECT ... FOR UPDATE，保证读改写期间的独占。
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var r Row
	if err := q.Take(&r).Error; err != nil {
		return r, err
	}
	return r, s.OpenRecord(r, value, kind)
}

func (s *Store) Delete(db *gorm.DB, kind, key string) error {
	return db.Table(Table(kind)).Where("id = ?", security.Hash(key)).Delete(&Row{}).Error
}

// CheckSchema 确认迁移已执行到受支持版本，且所有业务表均存在。
func (s *Store) CheckSchema() error {
	var version int64
	if err := s.DB.Raw("SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = 1").Scan(&version).Error; err != nil || version < 3 {
		return errors.New("schema migrations are missing; run gateway migrate")
	}
	for _, table := range []string{"edit_sessions", "commit_attempts", "challenges", "upload_tickets", "wiki_quota_locks", "wiki_site_connections"} {
		if !s.DB.Migrator().HasTable(table) {
			return errors.New("Wiki schema is missing; run gateway migrate")
		}
	}
	for _, t := range Tables {
		if !s.DB.Migrator().HasTable(Table(t)) {
			return errors.New("OAuth schema is missing; run gateway migrate")
		}
	}
	return nil
}

func writeClientColumns(db *gorm.DB, kind string, r Row) error {
	if kind != "clients" || r.State == "registry" {
		return nil
	}
	return db.Table(Table(kind)).Where("id = ?", r.ID).Updates(map[string]any{"fingerprint": r.Fingerprint, "client_name": r.ClientName, "logo_url": r.LogoURL, "trust_level": r.TrustLevel}).Error
}
