-- 001_init.sql
-- Postgres + pgvector (ANN retrieval)

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TYPE profile_status AS ENUM ('draft', 'processing', 'ready', 'confirmed');
CREATE TYPE job_status AS ENUM ('pending', 'running', 'done', 'failed');
CREATE TYPE job_type AS ENUM ('enrich_profile');

CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_id BIGINT UNIQUE NOT NULL,
    username    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE profiles (
    user_id          UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    status           profile_status NOT NULL DEFAULT 'draft',
    raw              JSONB NOT NULL DEFAULT '{}',
    enriched         JSONB,
    embedding        vector(1536),  -- NULL until enrich_profile completes
    embedding_model  TEXT,
    embedded_at      TIMESTAMPTZ,
    confirmed_at     TIMESTAMPTZ,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_profiles_status ON profiles(status);
CREATE INDEX idx_profiles_city ON profiles((raw->>'city'));

-- ANN index (HNSW, cosine distance). Safe on empty table.
CREATE INDEX idx_profiles_embedding_hnsw
    ON profiles USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

CREATE TABLE jobs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type         job_type NOT NULL,
    payload      JSONB NOT NULL,
    status       job_status NOT NULL DEFAULT 'pending',
    attempts     INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error   TEXT,
    run_after    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_jobs_pending ON jobs(status, run_after) WHERE status = 'pending';
