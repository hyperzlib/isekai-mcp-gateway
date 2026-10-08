package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/persistence"
)

type clientEnvelope = domain.StoredClient

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: export-oauth-clients CONFIG_DIR OUTPUT_FILE")
	}
	c, err := config.Load(os.Args[1], nil)
	if err != nil {
		return err
	}
	s, err := persistence.Open(c)
	if err != nil {
		return err
	}
	defer s.SQL.Close()
	var rows []persistence.Row
	if err := s.DB.Table("oauth_clients").Order("id").Find(&rows).Error; err != nil {
		return err
	}
	entries := make([]any, 0, len(rows))
	clients, matches := 0, 0
	for _, row := range rows {
		var payload json.RawMessage
		if err := s.OpenRecord(row, &payload, "clients"); err != nil {
			return fmt.Errorf("decrypt row %s: %w", row.ID, err)
		}
		entry := map[string]any{"id": row.ID, "state": row.State, "expires_at": row.ExpiresAt, "updated_at": row.UpdatedAt, "payload": payload, "fingerprint": row.Fingerprint, "client_name": row.ClientName, "logo_url": row.LogoURL, "trust_level": row.TrustLevel}
		if row.State != "registry" {
			var d clientEnvelope
			if err := s.OpenRecord(row, &d, "clients"); err != nil {
				return err
			}
			d.Metadata.SecretHash, d.Metadata.ResourceClient = d.SecretHash, d.ResourceClient
			recomputed, input, err := domain.ClientFingerprint(&d.Metadata)
			if err != nil {
				return err
			}
			match := recomputed == d.Fingerprint
			clients++
			if match {
				matches++
			}
			entry["fingerprint_check"] = map[string]any{"input_json": string(input), "stored": d.Fingerprint, "recomputed": recomputed, "matches": match}
		}
		entries = append(entries, entry)
	}
	result := map[string]any{"exported_at": time.Now().UTC(), "table": "oauth_clients", "row_count": len(rows), "client_count": clients, "matching_fingerprints": matches, "fingerprint_algorithm": "lowercase_hex(SHA256(UTF8(json.Marshal([v2, client_name, client_uri, sorted_unique_normalized_redirect_uris, sorted_unique_grant_types, sorted_unique_response_types, sorted_normalized_scopes, token_endpoint_auth_method, SecretHash, ResourceClient]))))", "rows": entries}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(os.Args[2]), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(os.Args[2], append(b, '\n'), 0600); err != nil {
		return err
	}
	fmt.Printf("Exported %d rows; fingerprint matches: %d/%d; file: %s\n", len(rows), matches, clients, os.Args[2])
	return nil
}
