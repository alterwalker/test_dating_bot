-- 003_ai_token_usage.sql — cumulative OpenAI token counters

CREATE TABLE ai_token_usage (
    model              TEXT NOT NULL,
    operation          TEXT NOT NULL,
    source             TEXT NOT NULL,
    prompt_tokens      BIGINT NOT NULL DEFAULT 0,
    completion_tokens  BIGINT NOT NULL DEFAULT 0,
    total_tokens       BIGINT NOT NULL DEFAULT 0,
    request_count      BIGINT NOT NULL DEFAULT 0,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (model, operation, source)
);

CREATE INDEX idx_ai_token_usage_model ON ai_token_usage (model);
