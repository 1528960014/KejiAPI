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

// APIKey is a user-facing access key; only its hash is stored.
type APIKey struct {
	ID            int64
	Name          string
	UserID        *int64
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

const keyColumns = `id, name, user_id, allowed_models, quota, spend, expires_at, created_at`

func scanKey(row pgx.Row) (*APIKey, error) {
	k := &APIKey{}
	err := row.Scan(&k.ID, &k.Name, &k.UserID, &k.AllowedModels, &k.Quota, &k.Spend, &k.ExpiresAt, &k.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
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
	).Scan(&key.ID, &key.Name, &key.UserID, &key.AllowedModels, &key.Quota, &key.Spend, &key.ExpiresAt, &key.CreatedAt)
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
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.UserID, &k.AllowedModels, &k.Quota, &k.Spend, &k.ExpiresAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
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
