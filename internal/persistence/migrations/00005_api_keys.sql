-- +goose NO TRANSACTION
-- +goose Up
CREATE TABLE oauth_api_keys (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  encrypted_payload LONGBLOB NULL,
  INDEX idx_account_state (account_id, state),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- +goose Down
DROP TABLE oauth_api_keys;
