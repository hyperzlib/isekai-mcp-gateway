-- +goose NO TRANSACTION
-- +goose Up
DELETE FROM oauth_clients;
DELETE FROM oauth_accounts;
DELETE FROM oauth_sessions;
DELETE FROM oauth_interactions;
DELETE FROM oauth_login_transactions;
DELETE FROM oauth_client_trusts;
DELETE FROM oauth_grants;
DELETE FROM oauth_authorization_codes;
DELETE FROM oauth_access_tokens;
DELETE FROM oauth_refresh_tokens;
ALTER TABLE oauth_clients ADD COLUMN encrypted_payload LONGBLOB NULL, MODIFY COLUMN payload JSON NOT NULL;
ALTER TABLE oauth_accounts ADD COLUMN encrypted_payload LONGBLOB NULL, MODIFY COLUMN payload JSON NOT NULL;
ALTER TABLE oauth_sessions ADD COLUMN encrypted_payload LONGBLOB NULL, MODIFY COLUMN payload JSON NOT NULL;
ALTER TABLE oauth_interactions ADD COLUMN encrypted_payload LONGBLOB NULL, MODIFY COLUMN payload JSON NOT NULL;
ALTER TABLE oauth_login_transactions ADD COLUMN encrypted_payload LONGBLOB NULL, MODIFY COLUMN payload JSON NOT NULL;
ALTER TABLE oauth_client_trusts ADD COLUMN encrypted_payload LONGBLOB NULL, MODIFY COLUMN payload JSON NOT NULL;
ALTER TABLE oauth_grants ADD COLUMN encrypted_payload LONGBLOB NULL, MODIFY COLUMN payload JSON NOT NULL;
ALTER TABLE oauth_authorization_codes ADD COLUMN encrypted_payload LONGBLOB NULL, MODIFY COLUMN payload JSON NOT NULL;
ALTER TABLE oauth_access_tokens ADD COLUMN encrypted_payload LONGBLOB NULL, MODIFY COLUMN payload JSON NOT NULL;
ALTER TABLE oauth_refresh_tokens ADD COLUMN encrypted_payload LONGBLOB NULL, MODIFY COLUMN payload JSON NOT NULL;
ALTER TABLE oauth_clients
 ADD COLUMN fingerprint VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
 ADD COLUMN client_name VARCHAR(256) NOT NULL DEFAULT '',
 ADD COLUMN logo_url TEXT NULL,
 ADD COLUMN trust_level ENUM('trusted','known','unknown') NOT NULL DEFAULT 'unknown',
 ADD INDEX idx_fingerprint (fingerprint);
-- +goose Down
DELETE FROM oauth_clients;
DELETE FROM oauth_accounts;
DELETE FROM oauth_sessions;
DELETE FROM oauth_interactions;
DELETE FROM oauth_login_transactions;
DELETE FROM oauth_client_trusts;
DELETE FROM oauth_grants;
DELETE FROM oauth_authorization_codes;
DELETE FROM oauth_access_tokens;
DELETE FROM oauth_refresh_tokens;
ALTER TABLE oauth_clients DROP INDEX idx_fingerprint, DROP COLUMN fingerprint, DROP COLUMN client_name, DROP COLUMN logo_url, DROP COLUMN trust_level;
ALTER TABLE oauth_clients DROP COLUMN encrypted_payload, MODIFY COLUMN payload LONGBLOB NOT NULL;
ALTER TABLE oauth_accounts DROP COLUMN encrypted_payload, MODIFY COLUMN payload LONGBLOB NOT NULL;
ALTER TABLE oauth_sessions DROP COLUMN encrypted_payload, MODIFY COLUMN payload LONGBLOB NOT NULL;
ALTER TABLE oauth_interactions DROP COLUMN encrypted_payload, MODIFY COLUMN payload LONGBLOB NOT NULL;
ALTER TABLE oauth_login_transactions DROP COLUMN encrypted_payload, MODIFY COLUMN payload LONGBLOB NOT NULL;
ALTER TABLE oauth_client_trusts DROP COLUMN encrypted_payload, MODIFY COLUMN payload LONGBLOB NOT NULL;
ALTER TABLE oauth_grants DROP COLUMN encrypted_payload, MODIFY COLUMN payload LONGBLOB NOT NULL;
ALTER TABLE oauth_authorization_codes DROP COLUMN encrypted_payload, MODIFY COLUMN payload LONGBLOB NOT NULL;
ALTER TABLE oauth_access_tokens DROP COLUMN encrypted_payload, MODIFY COLUMN payload LONGBLOB NOT NULL;
ALTER TABLE oauth_refresh_tokens DROP COLUMN encrypted_payload, MODIFY COLUMN payload LONGBLOB NOT NULL;
