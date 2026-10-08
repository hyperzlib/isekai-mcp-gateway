package persistence

import (
	"context"
	"embed"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate 使用内嵌 SQL 文件通过 goose 升级数据库结构。
func (s *Store) Migrate(ctx context.Context) error {
	files, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectMySQL, s.SQL, files)
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	return err
}
