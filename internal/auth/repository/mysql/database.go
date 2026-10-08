package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
)

type database struct{ db *gorm.DB }

// normalize 将 gorm 的 not-found 映射为应用层错误，其余包装为存储失败
func normalize(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrRecordNotFound
	}
	return failure.Storage("database.operation", err)
}

func (q database) WithContext(ctx context.Context) application.Database {
	return database{q.db.WithContext(ctx)}
}

// Transaction 在事务内注入新的 database 句柄，并归一化事务错误
func (q database) Transaction(fn func(application.Database) error) error {
	return normalize(q.db.Transaction(func(tx *gorm.DB) error { return fn(database{tx}) }))
}
func (q database) Table(name string) application.Database { return database{q.db.Table(name)} }
func (q database) Where(query any, args ...any) application.Database {
	return database{q.db.Where(query, args...)}
}

// Lock 追加行级排他锁，用于并发下的读改写保护
func (q database) Lock() application.Database {
	return database{q.db.Clauses(clause.Locking{Strength: "UPDATE"})}
}
func (q database) Take(value any) application.Database     { return database{q.db.Take(value)} }
func (q database) Find(value any) application.Database     { return database{q.db.Find(value)} }
func (q database) Count(value *int64) application.Database { return database{q.db.Count(value)} }
func (q database) Delete(value any) application.Database   { return database{q.db.Delete(value)} }
func (q database) Update(column string, value any) application.Database {
	return database{q.db.Update(column, value)}
}
func (q database) Order(value any) application.Database { return database{q.db.Order(value)} }
func (q database) Error() error                         { return normalize(q.db.Error) }
