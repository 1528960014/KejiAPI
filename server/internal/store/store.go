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
-- P2-1b: composed MP4 (local file under MEDIA_DIR, served at
-- /media/dramas/{uuid}.mp4) and the reason a video is missing.
ALTER TABLE dramas ADD COLUMN IF NOT EXISTS video_url TEXT NOT NULL DEFAULT '';
ALTER TABLE dramas ADD COLUMN IF NOT EXISTS video_error TEXT NOT NULL DEFAULT '';

-- P2-3: agent distribution. An agent is a billing user with a wholesale
-- rate (0 < rate <= 1) applied to every hold/settle of their balance;
-- subkeys (api_keys.agent_id) are reseller keys created by the agent that
-- bill the agent's balance at the agent's rate. markup is an informational
-- reseller price factor shown to the agent's customers (the platform does
-- not collect from end customers).
CREATE TABLE IF NOT EXISTS agents (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT UNIQUE NOT NULL REFERENCES users(id),
    rate NUMERIC(6,4) NOT NULL CHECK (rate > 0 AND rate <= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS agent_id BIGINT REFERENCES agents(id) ON DELETE SET NULL;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS markup NUMERIC(8,4) NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_api_keys_agent ON api_keys(agent_id);

-- P2-2: chat agents (predefined assistant templates). A chat request with
-- model = <agent_id> gets the template's system prompt injected and is
-- routed to the bound real model (billing follows the real model's prices).
CREATE TABLE IF NOT EXISTS assistants (
    id BIGSERIAL PRIMARY KEY,
    agent_id TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    system_prompt TEXT NOT NULL,
    model_id TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Built-in templates, seeded once per template and bound to the first
-- enabled chat model at seed time (admins can rebind via /admin/assistants).
INSERT INTO assistants (agent_id, name, description, system_prompt, model_id)
SELECT 'agent-translator', '翻译官', '专业翻译：只输出译文，保留语气与格式。',
       '你是一位专业翻译。将用户的消息翻译成目标语言（未指定时默认翻译成英文）。只输出译文，不要解释、不要附加原文。保留原文的语气、称谓与排版格式。',
       first.model_id
FROM (SELECT model_id FROM models WHERE enabled AND 'chat' = ANY(capabilities) ORDER BY id LIMIT 1) first
WHERE NOT EXISTS (SELECT 1 FROM assistants WHERE agent_id = 'agent-translator');
INSERT INTO assistants (agent_id, name, description, system_prompt, model_id)
SELECT 'agent-writer', '写手', '内容写手：按主题产出结构清晰、可直接使用的文章或文案。',
       '你是一位经验丰富的中文内容写手。根据用户给出的主题或大纲，产出结构清晰、语言流畅、可直接使用的文章或文案。用户未给具体要求时，给出一个完整初稿，并在结尾附 3 个备选标题。',
       first.model_id
FROM (SELECT model_id FROM models WHERE enabled AND 'chat' = ANY(capabilities) ORDER BY id LIMIT 1) first
WHERE NOT EXISTS (SELECT 1 FROM assistants WHERE agent_id = 'agent-writer');
INSERT INTO assistants (agent_id, name, description, system_prompt, model_id)
SELECT 'agent-support', '客服', '客服助手：先共情再解决，给出可执行步骤，不确定的不编造。',
       '你是一位友好、专业的客服助手。根据用户提供的产品背景回答客户问题：先共情、再解决，给出清晰可执行的步骤。不确定的信息要明确说明不确定，不要编造。',
       first.model_id
FROM (SELECT model_id FROM models WHERE enabled AND 'chat' = ANY(capabilities) ORDER BY id LIMIT 1) first
WHERE NOT EXISTS (SELECT 1 FROM assistants WHERE agent_id = 'agent-support');

-- P2-4: online recharge orders. Users pay CNY via a payment channel
-- (yipay / alipay / wechat) and the fixed credit_micro (micro-USD, locked
-- at order time) is credited to their balance when the notify verifies.
CREATE TABLE IF NOT EXISTS recharges (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    order_no TEXT UNIQUE NOT NULL,
    method TEXT NOT NULL,
    amount_cny BIGINT NOT NULL,
    credit_micro BIGINT NOT NULL,
    channel_trade_no TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_recharges_user ON recharges(user_id, created_at DESC);
`

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}
