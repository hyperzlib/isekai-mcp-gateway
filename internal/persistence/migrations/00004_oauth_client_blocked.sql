-- +goose Up
ALTER TABLE oauth_clients MODIFY COLUMN trust_level ENUM('trusted','known','unknown','blocked') NOT NULL DEFAULT 'unknown';

-- +goose Down
UPDATE oauth_clients SET trust_level = 'unknown' WHERE trust_level = 'blocked';
ALTER TABLE oauth_clients MODIFY COLUMN trust_level ENUM('trusted','known','unknown') NOT NULL DEFAULT 'unknown';
