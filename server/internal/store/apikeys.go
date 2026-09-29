package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// CreateAPIKey generates a new key and returns the plain value exactly once.
func (s *Store) CreateAPIKey(ctx context.Context, name string) (string, *APIKey, error) {
	plain, err := generateKey()
	if err != nil {
		return "", nil, err
	}
	key := &APIKey{}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (key_hash, name) VALUES ($1, $2)
		RETURNING id, name, user_id, allowed_models, quota, expires_at, created_at`,
		HashKey(plain), name,
	).Scan(&key.ID, &key.Name, &key.UserID, &key.AllowedModels, &key.Quota, &key.ExpiresAt, &key.CreatedAt)
	if err != nil {
		return "", nil, err
	}
	return plain, key, nil
}

// GetAPIKeyByHash resolves a stored key hash to its record.
func (s *Store) GetAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error) {
	key := &APIKey{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, user_id, allowed_models, quota, expires_at, created_at
		FROM api_keys WHERE key_hash = $1`, hash,
	).Scan(&key.ID, &key.Name, &key.UserID, &key.AllowedModels, &key.Quota, &key.ExpiresAt, &key.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return key, nil
}

// ListAPIKeys returns all keys, newest first.
func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, user_id, allowed_models, quota, expires_at, created_at
		FROM api_keys ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.UserID, &k.AllowedModels, &k.Quota, &k.ExpiresAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
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