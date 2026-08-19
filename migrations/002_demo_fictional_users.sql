-- 002_demo_fictional_users.sql
-- Вымышленные анкеты для demo mode (external_id вместо telegram_id)

CREATE TYPE user_kind AS ENUM ('telegram', 'fictional');

ALTER TABLE users
    ADD COLUMN user_kind user_kind NOT NULL DEFAULT 'telegram',
    ADD COLUMN external_id TEXT UNIQUE,
    ALTER COLUMN telegram_id DROP NOT NULL;

ALTER TABLE users
    ADD CONSTRAINT users_identity_check CHECK (
        (user_kind = 'telegram' AND telegram_id IS NOT NULL AND external_id IS NULL)
        OR
        (user_kind = 'fictional' AND external_id IS NOT NULL AND telegram_id IS NULL)
    );

CREATE INDEX idx_users_kind ON users(user_kind);
CREATE INDEX idx_users_external_id ON users(external_id) WHERE external_id IS NOT NULL;

-- Job type для batch seed re-enrich (опционально)
ALTER TYPE job_type ADD VALUE IF NOT EXISTS 'reembed_profile';
