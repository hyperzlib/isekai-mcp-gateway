-- +goose NO TRANSACTION
-- +goose Up
CREATE TABLE edit_sessions (
 id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 owner CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 state VARCHAR(32) NOT NULL, version BIGINT NOT NULL,
 has_changes BOOLEAN NOT NULL DEFAULT FALSE,
 expires_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 payload LONGBLOB NOT NULL,
 INDEX owner_expiry (owner,expires_at), INDEX state_expiry (state,expires_at)
) ENGINE=InnoDB;
CREATE TABLE commit_attempts LIKE edit_sessions;
CREATE TABLE challenges LIKE edit_sessions;
CREATE TABLE upload_tickets LIKE edit_sessions;
CREATE TABLE wiki_site_connections LIKE edit_sessions;
CREATE TABLE wiki_quota_locks (id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY) ENGINE=InnoDB;
-- +goose Down
DROP TABLE wiki_quota_locks;
DROP TABLE wiki_site_connections;
DROP TABLE upload_tickets;
DROP TABLE challenges;
DROP TABLE commit_attempts;
DROP TABLE edit_sessions;
