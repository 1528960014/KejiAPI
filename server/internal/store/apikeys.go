package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// APIKey is a user-facing access key; only its hash is stored. A non-nil
// AgentID marks a reseller subkey: it bills the agent's balance at the
// agent's wholesale rate. Markup is the informational reseller price factor
// the agent shows its own customers.
type APIKey struct {
	ID            int64
	Name          string
	UserID        *int64
	AgentID       *int64
	Markup        float64
	AllowedModels []string
	Quota         *int64
	Spend         int64
	ExpiresAt     *time.Time
	CreatedAt     time.Time
}

// HashKey returns the SHA-256 hex digest used to store keys.
func HashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func generateKey() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sk-" + hex.EncodeToString(buf), nil
}

const keyColumns = `id, name, user_id, agent_id, markup, allowed_models, quota, spend, expires_at, created_at`

func scanKey(row pgx.Row) (*APIKey, error) {
	k := &APIKey{}
	err := row.Scan(&k.ID, &k.Name, &k.UserID, &k.AgentID, &k.Markup, &k.AllowedModels, &k.Quota, &k.Spend, &k.ExpiresAt, &k.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return k, err
}

func scanKeyRow(rows pgx.Rows) (*APIKey, error) {
	k := &APIKey{}
	err := rows.Scan(&k.ID, &k.Name, &k.UserID, &k.AgentID, &k.Markup, &k.AllowedModels, &k.Quota, &k.Spend, &k.ExpiresAt, &k.CreatedAt)
	return k, err
}

// CreateAPIKey generates a new key and returns the plain value exactly once.
func (s *Store) CreateAPIKey(ctx context.Context, name string) (string, *APIKey, error) {
	plain, err := generateKey()
	if err != nil {
		return "", nil, err
	}
	key := &APIKey{}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (key_hash, name) VALUES ($1, $2)
		RETURNING `+keyColumns,
		HashKey(plain), name,
	).Scan(&key.ID, &key.Name, &key.UserID, &key.AgentID, &key.Markup, &key.AllowedModels, &key.Quota, &key.Spend, &key.ExpiresAt, &key.CreatedAt)
	if err != nil {
		return "", nil, err
	}
	return plain, key, nil
}

// GetAPIKeyByHash resolves a stored key hash to its record.
func (s *Store) GetAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error) {
	return scanKey(s.pool.QueryRow(ctx, `
		SELECT `+keyColumns+`
		FROM api_keys WHERE key_hash = $1`, hash))
}

// GetAPIKey fetches one key by ID.
func (s *Store) GetAPIKey(ctx context.Context, id int64) (*APIKey, error) {
	return scanKey(s.pool.QueryRow(ctx, `SELECT `+keyColumns+` FROM api_keys WHERE id = $1`, id))
}

// ListAPIKeys returns all keys, newest first.
func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+keyColumns+`
		FROM api_keys ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		k, err := scanKeyRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

// KeyPatch carries the mutable fields for UpdateAPIKey; nil fields are left
// unchanged. UserID/Quota values of 0 clear the binding/quota; an ExpiresAt
// pointing at the zero time clears the expiry.
type KeyPatch struct {
	Name          *string
	UserID        *int64
	Quota         *int64 // micro-USD
	AllowedModels *[]string
	ExpiresAt     *time.Time
}

// UpdateAPIKey applies a partial update and returns the fresh key.
func (s *Store) UpdateAPIKey(ctx context.Context, id int64, p *KeyPatch) (*APIKey, error) {
	sets := []string{}
	args := []any{}
	nextArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if p.Name != nil {
		sets = append(sets, "name="+nextArg(*p.Name))
	}
	if p.UserID != nil {
		if *p.UserID == 0 {
			sets = append(sets, "user_id="+nextArg(nil))
		} else {
			var ok int
			if err := s.pool.QueryRow(ctx, `SELECT 1 FROM users WHERE id = $1`, *p.UserID).Scan(&ok); errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound
			} else if err != nil {
				return nil, err
			}
			sets = append(sets, "user_id="+nextArg(*p.UserID))
		}
	}
	if p.Quota != nil {
		if *p.Quota == 0 {
			sets = append(sets, "quota="+nextArg(nil))
		} else {
			sets = append(sets, "quota="+nextArg(*p.Quota))
		}
	}
	if p.AllowedModels != nil {
		sets = append(sets, "allowed_models="+nextArg(*p.AllowedModels))
	}
	if p.ExpiresAt != nil {
		if p.ExpiresAt.IsZero() {
			sets = append(sets, "expires_at="+nextArg(nil))
		} else {
			sets = append(sets, "expires_at="+nextArg(*p.ExpiresAt))
		}
	}
	if len(sets) == 0 {
		return s.GetAPIKey(ctx, id)
	}
	args = append(args, id)
	return scanKey(s.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE api_keys SET %s WHERE id = $%d
		RETURNING `+keyColumns, strings.Join(sets, ", "), len(args)+1), args...))
}

// DeleteAPIKey removes a key by ID.
func (s *Store) DeleteAPIKey(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateAPIKeyForUser is CreateAPIKey with the user binding set.
func (s *Store) CreateAPIKeyForUser(ctx context.Context, userID int64, name string) (string, *APIKey, error) {
	plain, err := generateKey()
	if err != nil {
		return "", nil, err
	}
	key := &APIKey{}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (key_hash, name, user_id) VALUES ($1, $2, $3)
		RETURNING `+keyColumns,
		HashKey(plain), name, userID,
	).Scan(&key.ID, &key.Name, &key.UserID, &key.AgentID, &key.Markup, &key.AllowedModels, &key.Quota, &key.Spend, &key.ExpiresAt, &key.CreatedAt)
	if err != nil {
		return "", nil, err
	}
	return plain, key, nil
}

// ListAPIKeysByUser returns the user's own keys (reseller subkeys are
// excluded; use ListSubkeys for those), newest first.
func (s *Store) ListAPIKeysByUser(ctx context.Context, userID int64) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+keyColumns+`
		FROM api_keys WHERE user_id = $1 AND agent_id IS NULL ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		k, err := scanKeyRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

// DeleteAPIKeyByUser removes a key only if it belongs to the user and is not
// a reseller subkey (subkeys are managed via DeleteSubkey).
func (s *Store) DeleteAPIKeyByUser(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1 AND user_id = $2 AND agent_id IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateSubkey generates a reseller subkey for an agent. The key bills the
// agent's user balance at the agent's wholesale rate; markup is the
// informational reseller factor shown to the agent's customers.
func (s *Store) CreateSubkey(ctx context.Context, agentID, userID int64, name string, markup float64, allowedModels []string, quota *int64, expiresAt *time.Time) (string, *APIKey, error) {
	if markup <= 0 {
		markup = 1
	}
	plain, err := generateKey()
	if err != nil {
		return "", nil, err
	}
	key := &APIKey{}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (key_hash, name, user_id, agent_id, markup, allowed_models, quota, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+keyColumns,
		HashKey(plain), name, userID, agentID, markup, allowedModels, quota, expiresAt,
	).Scan(&key.ID, &key.Name, &key.UserID, &key.AgentID, &key.Markup, &key.AllowedModels, &key.Quota, &key.Spend, &key.ExpiresAt, &key.CreatedAt)
	if err != nil {
		return "", nil, err
	}
	return plain, key, nil
}

// ListSubkeys returns one agent's reseller keys, newest first.
func (s *Store) ListSubkeys(ctx context.Context, agentID int64) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+keyColumns+`
		FROM api_keys WHERE agent_id = $1 ORDER BY id DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		k, err := scanKeyRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

// DeleteSubkey removes a subkey only if it belongs to the agent.
func (s *Store) DeleteSubkey(ctx context.Context, agentID, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1 AND agent_id = $2`, id, agentID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}