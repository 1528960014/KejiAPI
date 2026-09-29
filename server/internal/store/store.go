package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the persistence layer over PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// New connects to PostgreSQL and applies the schema idempotently.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// Pool exposes the raw pool for read-only diagnostics.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Money amounts are stored as BIGINT micro-USD (1 USD = 1_000_000 units) so
// ledger arithmetic stays exact.

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    balance BIGINT NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ledger_entries (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    kind TEXT NOT NULL,
    amount BIGINT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    request_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_request_id ON ledger_entries(request_id) WHERE request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ledger_user ON ledger_entries(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS models (
    id BIGSERIAL PRIMARY KEY,
    model_id TEXT UNIQUE NOT NULL,
    provider TEXT NOT NULL,
    upstream_model TEXT NOT NULL,
    capabilities TEXT[] NOT NULL DEFAULT '{}',
    input_price_per_1k NUMERIC(18,10) NOT NULL DEFAULT 0,
    output_price_per_1k NUMERIC(18,10) NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS channels (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    provider TEXT NOT NULL,
    base_url TEXT NOT NULL,
    api_key TEXT NOT NULL,
    model_id TEXT NOT NULL REFERENCES models(model_id) ON DELETE CASCADE,
    priority INT NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_channels_model ON channels(model_id, enabled, priority DESC);

CREATE TABLE IF NOT EXISTS api_keys (
    id BIGSERIAL PRIMARY KEY,
    key_hash TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    user_id BIGINT REFERENCES users(id),
    allowed_models TEXT[] NOT NULL DEFAULT '{}',
    quota BIGINT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);

CREATE TABLE IF NOT EXISTS tasks (
    id BIGSERIAL PRIMARY KEY,
    task_uuid TEXT UNIQUE NOT NULL,
    api_key_id BIGINT REFERENCES api_keys(id),
    type TEXT NOT NULL,
    model_id TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'queued',
    result_urls TEXT[] NOT NULL DEFAULT '{}',
    cost NUMERIC(18,10) NOT NULL DEFAULT 0,
    error_msg TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tasks_key ON tasks(api_key_id, created_at DESC);

CREATE TABLE IF NOT EXISTS usage_logs (
    id BIGSERIAL PRIMARY KEY,
    api_key_id BIGINT REFERENCES api_keys(id),
    model_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    stream BOOLEAN NOT NULL DEFAULT FALSE,
    prompt_tokens INT NOT NULL DEFAULT 0,
    completion_tokens INT NOT NULL DEFAULT 0,
    cost NUMERIC(18,10) NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    error_msg TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_usage_key_time ON usage_logs(api_key_id, created_at DESC);

-- M2: lifetime spend per key (micro-USD), used for quota enforcement.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS spend BIGINT NOT NULL DEFAULT 0;
-- M2: users may exist without a login password (billing-only accounts);
-- console login lands in M3.
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

-- M3: refresh tokens for console login (single-use, revocable).
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

-- M4: per-unit pricing for media models. price_unit is 'token' (default,
-- priced per 1K tokens via the *_price_per_1k columns) or one of
-- 'image'/'video'/'music'/'tts' (priced per generated item via unit_price).
ALTER TABLE models ADD COLUMN IF NOT EXISTS price_unit TEXT NOT NULL DEFAULT 'token';
ALTER TABLE models ADD COLUMN IF NOT EXISTS unit_price NUMERIC(18,10) NOT NULL DEFAULT 0;
-- M4: frozen amount (micro-USD) per task, fixed at submission so the worker
-- can settle or release exactly what was held.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS hold_micro BIGINT NOT NULL DEFAULT 0;

-- P2-1: comic-drama pipeline. A drama fans a script out into storyboard
-- shots (image + optional TTS per shot); the shots array is the exportable
-- asset pack (JSONB), held cost is frozen at submission and settled
-- all-or-nothing (any failed shot releases the whole hold).
CREATE TABLE IF NOT EXISTS dramas (
    id BIGSERIAL PRIMARY KEY,
    drama_uuid TEXT UNIQUE NOT NULL,
    api_key_id BIGINT REFERENCES api_keys(id),
    title TEXT NOT NULL DEFAULT '',
    script TEXT NOT NULL,
    style TEXT NOT NULL DEFAULT '',
    storyboard_model TEXT NOT NULL DEFAULT '',
    image_model TEXT NOT NULL,
    tts_model TEXT,
    shots_planned INT NOT NULL DEFAULT 8,
    status TEXT NOT NULL DEFAULT 'queued',
    hold_micro BIGINT NOT NULL DEFAULT 0,
    shots JSONB NOT NULL DEFAULT '[]',
    cost NUMERIC(18,10) NOT NULL DEFAULT 0,
    error_msg TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_dramas_key ON dramas(api_key_id, created_at DESC);
`

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}
