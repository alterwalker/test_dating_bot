# Модель данных

## Enriched profile (JSON)

Результат LLM extraction. Схема: [prompts/profile.schema.json](../prompts/profile.schema.json).

```json
{
  "interests": ["бег", "дизайн", "собаки"],
  "values": ["честность", "поддержка"],
  "lifestyle_axes": {
    "outgoing": 0.6,
    "family_oriented": 0.8,
    "career_focused": 0.5,
    "adventurous": 0.7,
    "homebody": 0.4
  },
  "relationship_intent": "serious",
  "communication_style": "спокойный, с лёгким юмором",
  "dealbreakers_detected": ["курение"],
  "summary": "Активная, семейная, ищет стабильные отношения.",
  "extraction_notes": "intent выведен из prompt_relationship_values"
}
```

### Оси lifestyle (0.0 – 1.0)

| Ось | Смысл |
|-----|-------|
| `outgoing` | тусовки, люди, мероприятия |
| `family_oriented` | семья, дети, дом |
| `career_focused` | работа, амбиции |
| `adventurous` | путешествия, новое |
| `homebody` | дом, уют, спокойствие |

---

## Raw profile (поля анкеты)

| Поле | Тип | Обязательное |
|------|-----|--------------|
| `name` | string | да |
| `age` | int | да |
| `city` | string | да |
| `gender` | enum | да |
| `seeking` | []enum | да |
| `age_min` | int | нет |
| `age_max` | int | нет |
| `prompt_ideal_evening` | string | да (min 20 символов) |
| `prompt_relationship_values` | string | да |
| `prompt_occupation` | string | да |

---

## SQL (Postgres)

```sql
-- migrations/001_init.sql

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
    user_id       UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    status        profile_status NOT NULL DEFAULT 'draft',
    raw           JSONB NOT NULL DEFAULT '{}',
    enriched      JSONB,
    embedding        vector(1536),
    embedding_model  TEXT,
    embedded_at      TIMESTAMPTZ,
    confirmed_at     TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_profiles_status ON profiles(status);
CREATE INDEX idx_profiles_city ON profiles((raw->>'city'));

CREATE TABLE jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type        job_type NOT NULL,
    payload     JSONB NOT NULL,
    status      job_status NOT NULL DEFAULT 'pending',
    attempts    INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error  TEXT,
    run_after   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_jobs_pending ON jobs(status, run_after) WHERE status = 'pending';
```

### Job payload (`enrich_profile`)

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

## Embedding

См. полное описание: [EMBEDDINGS.md](EMBEDDINGS.md).

- **Embedding model:** OpenAI `text-embedding-3-small` при `AI_MOCK=false`; mock hash-vector при `AI_MOCK=true`
- **Хранение:** `profiles.embedding vector(1536)` + HNSW index (pgvector)
- **Поиск:** ANN retrieval `ORDER BY embedding <=> $query LIMIT 50`
- Input: три промпта + `summary` после LLM extraction (в worker)

---

## Match breakdown (не персистится)

Вычисляется on-the-fly в `matching/scorer.go`, отдаётся в API response.
