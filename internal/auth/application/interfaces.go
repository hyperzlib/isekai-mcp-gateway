package application

import (
	"context"
	"errors"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/persistence"
)

var ErrRecordNotFound = errors.New("record not found")

// Database keeps transaction and locking semantics explicit without exposing GORM.
// SQL predicates are internal application queries; no caller-supplied SQL is accepted.
type Database interface {
	WithContext(context.Context) Database
	Transaction(func(Database) error) error
	Table(string) Database
	Where(any, ...any) Database
	Lock() Database
	Take(any) Database
	Find(any) Database
	Count(*int64) Database
	Delete(any) Database
	Update(string, any) Database
	Order(any) Database
	Error() error
}

type RecordStore interface {
	Database() Database
	Get(Database, string, string, any, bool) (persistence.Row, error)
	Put(Database, string, string, any, persistence.Row) error
	Create(Database, string, string, any, persistence.Row) error
	Delete(Database, string, string) error
	OpenRecord(persistence.Row, any, string) error
	Health(context.Context) error
}

type TokenDecoder interface{ Decrypt(string) (string, error) }

type MetadataFetcher func(context.Context, string, config.Gateway) ([]byte, error)
