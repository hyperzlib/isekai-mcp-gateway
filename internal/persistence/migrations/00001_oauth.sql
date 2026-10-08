-- +goose NO TRANSACTION
-- +goose Up
CREATE TABLE oauth_clients (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  INDEX idx_account_client (account_id, client_id),
  INDEX idx_client_state (client_id, state),
  INDEX idx_grant (grant_id),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE oauth_accounts (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  INDEX idx_account_client (account_id, client_id),
  INDEX idx_client_state (client_id, state),
  INDEX idx_grant (grant_id),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE oauth_sessions (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  INDEX idx_account_client (account_id, client_id),
  INDEX idx_client_state (client_id, state),
  INDEX idx_grant (grant_id),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE oauth_interactions (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  INDEX idx_account_client (account_id, client_id),
  INDEX idx_client_state (client_id, state),
  INDEX idx_grant (grant_id),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE oauth_login_transactions (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  INDEX idx_account_client (account_id, client_id),
  INDEX idx_client_state (client_id, state),
  INDEX idx_grant (grant_id),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE oauth_client_trusts (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  INDEX idx_account_client (account_id, client_id),
  INDEX idx_client_state (client_id, state),
  INDEX idx_grant (grant_id),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE oauth_grants (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  INDEX idx_account_client (account_id, client_id),
  INDEX idx_client_state (client_id, state),
  INDEX idx_grant (grant_id),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE oauth_authorization_codes (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  INDEX idx_account_client (account_id, client_id),
  INDEX idx_client_state (client_id, state),
  INDEX idx_grant (grant_id),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE oauth_access_tokens (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  INDEX idx_account_client (account_id, client_id),
  INDEX idx_client_state (client_id, state),
  INDEX idx_grant (grant_id),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE oauth_refresh_tokens (
  id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  account_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  client_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  grant_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  state VARCHAR(24) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  payload LONGBLOB NOT NULL,
  INDEX idx_account_client (account_id, client_id),
  INDEX idx_client_state (client_id, state),
  INDEX idx_grant (grant_id),
  INDEX idx_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- +goose Down
DROP TABLE oauth_clients;
DROP TABLE oauth_accounts;
DROP TABLE oauth_sessions;
DROP TABLE oauth_interactions;
DROP TABLE oauth_login_transactions;
DROP TABLE oauth_client_trusts;
DROP TABLE oauth_grants;
DROP TABLE oauth_authorization_codes;
DROP TABLE oauth_access_tokens;
DROP TABLE oauth_refresh_tokens;
