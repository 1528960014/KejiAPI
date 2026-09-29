package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// GetUserByEmail resolves a user by their unique email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email))
}

// CreateAccount creates a user with a login password and a zero balance.
func (s *Store) CreateAccount(ctx context.Context, email, passwordHash string) (*User, error) {
	u := &User{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, balance)
		VALUES ($1, $2, 0)
		RETURNING `+userColumns, email, passwordHash,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Balance, &u.Enabled, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// hashCredential stores only the SHA-256 of opaque credentials.
func hashCredential(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// CreateRefreshToken stores a new single-use refresh credential.
func (s *Store) CreateRefreshToken(ctx context.Context, userID int64, plain string, ttl time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, now() + $3)`,
		userID, hashCredential(plain), ttl)
	return err
}

// RedeemRefreshToken atomically revokes the token and returns its user.
// Unknown, expired or already-redeemed tokens yield ErrNotFound.
func (s *Store) RedeemRefreshToken(ctx context.Context, plain string) (int64, error) {
	var userID int64
	err := s.pool.QueryRow(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		RETURNING user_id`, hashCredential(plain),
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return userID, nil
}

// RevokeRefreshToken revokes one credential (idempotent).
func (s *Store) RevokeRefreshToken(ctx context.Context, plain string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL`, hashCredential(plain))
	return err
}

// RevokeAllRefreshTokens revokes every live credential of the user.
func (s *Store) RevokeAllRefreshTokens(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}
