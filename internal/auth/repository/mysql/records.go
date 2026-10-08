package mysql

import (
	"context"
	"errors"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
)

// Store binds the application repository port to the MySQL record store.
type Store struct{ storage *persistence.Store }

func New(store *persistence.Store) *Store       { return &Store{storage: store} }
func (s *Store) Database() application.Database { return database{s.storage.DB} }
func (s *Store) Get(tx application.Database, kind, key string, value any, lock bool) (persistence.Row, error) {
	row, err := s.storage.Get(tx.(database).db, kind, key, value, lock)
	return row, repositoryError("store.get."+kind, err)
}
func (s *Store) Put(tx application.Database, kind, key string, value any, row persistence.Row) error {
	return repositoryError("store.put."+kind, s.storage.Put(tx.(database).db, kind, key, value, row))
}
func (s *Store) Create(tx application.Database, kind, key string, value any, row persistence.Row) error {
	return repositoryError("store.create."+kind, s.storage.Create(tx.(database).db, kind, key, value, row))
}
func (s *Store) Delete(tx application.Database, kind, key string) error {
	return repositoryError("store.delete."+kind, s.storage.Delete(tx.(database).db, kind, key))
}
func (s *Store) OpenRecord(row persistence.Row, value any, kind string) error {
	return failure.Wrap("store.open_record", s.storage.OpenRecord(row, value, kind))
}
func (s *Store) Health(ctx context.Context) error { return s.storage.Health(ctx) }

var _ application.RecordStore = (*Store)(nil)

// repositoryError 统一归一化存储错误：保留 not-found 语义，其余包装为存储失败
func repositoryError(op string, err error) error {
	err = normalize(err)
	if err == nil || errors.Is(err, application.ErrRecordNotFound) {
		return err
	}
	return failure.Storage(op, err)
}
