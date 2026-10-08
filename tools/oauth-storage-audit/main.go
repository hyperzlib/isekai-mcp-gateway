// Read-only storage audit. Outputs row counts, schema and Wiki content digests,
// never credentials or record payloads.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/persistence"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: oauth-storage-audit CONFIG_DIR OUTPUT_FILE")
	}
	cfg, err := config.Load(os.Args[1], nil)
	if err != nil {
		return err
	}
	s, err := persistence.Open(cfg)
	if err != nil {
		return err
	}
	defer s.SQL.Close()
	result := map[string]any{}
	var version int64
	if err = s.SQL.QueryRow("SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied=1").Scan(&version); err != nil {
		return err
	}
	result["schema_version"] = version
	counts := map[string]int64{}
	for _, kind := range persistence.Tables {
		var count int64
		if err = s.SQL.QueryRow("SELECT COUNT(*) FROM " + persistence.Table(kind)).Scan(&count); err != nil {
			return err
		}
		counts[kind] = count
	}
	result["oauth_counts"] = counts
	type column struct{ Name, Type, Nullable string }
	var columns []column
	rows, err := s.SQL.Query("SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME='oauth_clients' ORDER BY ORDINAL_POSITION", cfg.Static.Storage.Database)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c column
		if err = rows.Scan(&c.Name, &c.Type, &c.Nullable); err != nil {
			rows.Close()
			return err
		}
		columns = append(columns, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	result["client_columns"] = columns
	wiki := map[string]any{}
	for _, table := range []string{"edit_sessions", "commit_attempts", "challenges", "upload_tickets", "wiki_site_connections", "wiki_quota_locks"} {
		rows, err := s.SQL.Query("SELECT * FROM " + table + " ORDER BY id")
		if err != nil {
			return err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return err
		}
		h := sha256.New()
		count := 0
		for rows.Next() {
			values := make([][]byte, len(cols))
			args := make([]any, len(cols))
			for i := range args {
				args[i] = &values[i]
			}
			if err = rows.Scan(args...); err != nil {
				rows.Close()
				return err
			}
			b, err := json.Marshal(values)
			if err != nil {
				rows.Close()
				return err
			}
			h.Write(b)
			h.Write([]byte{'\n'})
			count++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		wiki[table] = map[string]any{"count": count, "sha256": hex.EncodeToString(h.Sum(nil))}
	}
	result["wiki_tables"] = wiki
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(os.Args[2]), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(os.Args[2], append(b, '\n'), 0600); err != nil {
		return err
	}
	fmt.Printf("Schema version: %d; audit: %s\n", version, os.Args[2])
	return nil
}
