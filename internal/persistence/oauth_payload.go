package persistence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"isekai-mcp-gateway/internal/auth/domain"
	"strings"
)

// Each storage kind has an explicit credential field list. No field-name
// heuristics are used; query keys remain in the indexed Row columns.
func credentialPaths(kind string) []string {
	switch kind {
	case "clients":
		return []string{"SecretHash"}
	case "sessions":
		return []string{"ID", "Browser", "Identity.user_token"}
	case "login_transactions":
		return []string{"ID", "Browser", "Nonce", "Verifier", "Snapshot"}
	case "interactions":
		return []string{"Browser", "SessionID", "Request.state", "Request.nonce", "Request.id_token_hint", "Request.RequestParam"}
	case "grants":
		return []string{"SessionID"}
	case "access_tokens", "refresh_tokens":
		return []string{"ID"}
	default:
		return nil
	}
}

func moveCredential(src, dst map[string]any, path []string) {
	if len(path) == 1 {
		if v, ok := src[path[0]]; ok {
			if v != nil && v != "" {
				dst[path[0]] = v
			}
			delete(src, path[0])
		}
		return
	}
	child, ok := src[path[0]].(map[string]any)
	if !ok {
		return
	}
	out, _ := dst[path[0]].(map[string]any)
	if out == nil {
		out = map[string]any{}
	}
	moveCredential(child, out, path[1:])
	if len(out) > 0 {
		dst[path[0]] = out
	}
}

func mergeCredentials(dst, src map[string]any) {
	for k, v := range src {
		if child, ok := v.(map[string]any); ok {
			target, _ := dst[k].(map[string]any)
			if target == nil {
				target = map[string]any{}
			}
			mergeCredentials(target, child)
			dst[k] = target
		} else {
			dst[k] = v
		}
	}
}

func (s *Store) splitPayload(kind string, value any, id string) ([]byte, []byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	var plain map[string]any
	if err = decodeObject(b, &plain); err != nil {
		return nil, nil, err
	}
	secret := map[string]any{}
	for _, path := range credentialPaths(kind) {
		moveCredential(plain, secret, strings.Split(path, "."))
	}
	b, err = json.Marshal(plain)
	if err != nil {
		return nil, nil, err
	}
	if len(secret) == 0 {
		return b, nil, nil
	}
	encrypted, err := s.Vault.Seal(secret, Table(kind)+":"+id+":split-v1")
	return b, encrypted, err
}

func (s *Store) OpenRecord(row Row, value any, kind string) error {
	var plain map[string]any
	if err := decodeObject(row.Payload, &plain); err != nil {
		return err
	}
	// Reject credentials in plaintext rather than silently accepting a broken
	// write path or accidentally interpreting the old encrypted format.
	for _, path := range credentialPaths(kind) {
		probe := map[string]any{}
		moveCredential(plain, probe, strings.Split(path, "."))
		if len(probe) > 0 {
			return fmt.Errorf("credential in plaintext %s", kind)
		}
	}
	if len(row.EncryptedPayload) > 0 {
		var raw json.RawMessage
		if err := s.Vault.Open(row.EncryptedPayload, &raw, Table(kind)+":"+row.ID+":split-v1"); err != nil {
			return err
		}
		var secret map[string]any
		if err := decodeObject(raw, &secret); err != nil {
			return err
		}
		mergeCredentials(plain, secret)
	}
	b, err := json.Marshal(plain)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, value); err != nil {
		return err
	}
	if kind == "clients" && row.State != "registry" {
		d, ok := value.(*domain.StoredClient)
		if !ok {
			return nil
		}
		d.Metadata.Name = row.ClientName
		d.Metadata.LogoURL = row.LogoURL
		d.Metadata.TrustLevel = domain.TrustLevel(row.TrustLevel)
		d.Fingerprint = row.Fingerprint
	}
	return nil
}

func decodeObject(b []byte, out *map[string]any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	return d.Decode(out)
}
