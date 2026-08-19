# Архитектура

## Обзор

Система состоит из **трёх Go-бинарников** и **одной БД**. Связь bot → api по HTTP/JSON. Асинхронная работа с LLM — через таблицу `jobs` (без Redis).

```
┌─────────────┐     HTTP/JSON      ┌─────────────┐
│   cmd/bot   │ ─────────────────► │   cmd/api   │
│  Telegram   │ ◄───────────────── │  REST + DB  │
└─────────────┘                    └──────┬──────┘
                                          │
                                    jobs table
                                          │
                                          ▼
                                   ┌─────────────┐
                                   │ cmd/worker  │
                                   │ LLM + embed │
                                   └──────┬──────┘
                                          │
                    ┌─────────────────────┼─────────────────────┐
                    ▼                     ▼                     ▼
              PostgreSQL            OpenAI API            (mock AI)
```

## Бинарники

### 1. `cmd/bot` — Telegram Gateway

**Ответственность:**
- приём updates (long polling или webhook);
- FSM-сценарий анкеты;
- inline/reply клавиатуры;
- HTTP-клиент к `api`;
- polling статуса профиля (`processing` → `ready`).

**Не содержит:** SQL, промпты, scoring, вызовы OpenAI.

**Зависимости:** только `pkg/apiclient` (или inline HTTP) + конфиг (`BOT_TOKEN`, `API_BASE_URL`).

---

### 2. `cmd/api` — Platform API

**Ответственность:**
- CRUD пользователей и профилей;
- создание job `enrich_profile`;
- синхронный матчинг (`GET /matches`);
- опционально: LLM explain для top-3 (можно вынести в worker позже);
- миграции БД при старте (или отдельная команда).

**Не содержит:** Telegram SDK, FSM.

**Endpoints:** см. [API.md](API.md).

---

### 3. `cmd/worker` — Background Worker

**Ответственность:**
- poll jobs: `SELECT ... FOR UPDATE SKIP LOCKED`;
- job type `enrich_profile`:
  1. загрузить raw profile;
  2. вызвать LLM extractor (structured JSON);
  3. нормализовать теги;
  4. получить embedding;
  5. сохранить enriched profile, status = `ready`;
- retry с backoff, max attempts;
- healthcheck `:8081/health`.

**Не содержит:** Telegram, HTTP API для бота (кроме health).

---

## Пакеты `internal/`

```
internal/
├── domain/       # User, Profile, RawProfile, EnrichedProfile, Job, Match
├── storage/      # репозитории, SQL
├── profile/      # сервис профилей (используется api + worker)
├── matching/
│   ├── filters.go    # жёсткие фильтры
│   ├── retrieval.go  # top-K по грубому score
│   └── scorer.go     # двусторонний rank
├── ai/
│   ├── client.go     # OpenAI / mock
│   ├── extractor.go
│   ├── embedder.go
│   ├── explainer.go
│   └── prompts.go    # загрузка файлов из prompts/
└── jobs/
    ├── repository.go
    └── handler.go    # dispatch по job type
```

### Границы ответственности

| Пакет | Знает | Не знает |
|-------|-------|----------|
| `domain` | типы, enum | infra |
| `storage` | SQL | Telegram, промпты |
| `profile` | lifecycle профиля | FSM |
| `matching` | filters, score | LLM prompts |
| `ai` | LLM, embeddings | HTTP routes, Telegram |
| `jobs` | очередь | UI |

---

## Потоки данных

### Создание профиля

```
User → bot (FSM, 3 промпта)
  → api PUT /profile/raw
  → api POST /profile/enrich  →  job в БД
  → bot показывает «⏳ Обрабатываю…»
  → worker: LLM extract + embed → enriched profile
  → bot poll GET /profile/status → ready
  → bot показывает разметку + [✅ Верно] [✏️ Исправить]
  → api POST /profile/confirm
```

### Подбор пар

```
User → bot «Найти пары»
  → api GET /matches?limit=10
  → filters → retrieve (top-K) → rank (two-sided)
  → api: explainer для top-3 (LLM)
  → bot: карточки с score + explanation
```

---

## Матчинг (упрощённый)

### Этап 1: Hard filters

- совместимость `gender` / `seeking`;
- возраст в диапазоне (если задан);
- город (на MVP — точное совпадение или один город в seed);
- `relationship_intent` совместим;
- dealbreakers не пересекаются.

### Этап 2: Retrieval (ANN, top-K = 50)

**pgvector HNSW** — approximate nearest neighbor по `profiles.embedding`:

```sql
ORDER BY embedding <=> $query_vector
LIMIT 50
```

После ANN — гибридный доскоринг в Go:

```
retrieve = 0.5 * embedding_similarity
         + 0.3 * jaccard(interests)
         + 0.2 * jaccard(values)
```

Подробнее: [EMBEDDINGS.md](EMBEDDINGS.md).  
Fallback для тестов: `RETRIEVAL_MODE=brute` (перебор в памяти).

### Этап 3: Rank (финальный)

```
pref(A→B) = w1*intent_match + w2*jaccard(values) + w3*axis_similarity + w4*interest_overlap
          - penalty(dealbreakers)

match(A,B) = harmonic_mean(pref(A→B), pref(B→A))
           + 0.2 * cosine(embedding_A, embedding_B)
```

`harmonic_mean(a,b) = 2ab / (a+b)` — слабая односторонняя совместимость тянет score вниз.

### Explain

Только для top-3: LLM получает **факты** (теги, оси, breakdown), генерирует 2–3 предложения на русском. Промпт запрещает выдумывать то, чего нет в данных.

---

## Очередь jobs

Таблица `jobs` в Postgres — без Redis.

```sql
-- см. migrations/001_init.sql
```

Worker:

1. `SELECT id FROM jobs WHERE status='pending' AND run_after <= now() ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED`
2. `status = running`
3. выполнить handler
4. `status = done` или `failed` (+ `attempts++`, retry через `run_after`)

---

## Конфигурация

| Переменная | Кто использует |
|------------|----------------|
| `BOT_TOKEN` | bot |
| `API_BASE_URL` | bot |
| `DATABASE_URL` | api, worker |
| `OPENAI_API_KEY` | worker, api (только при `AI_MOCK=false`) |
| `OPENAI_MODEL` | worker, api (default: `gpt-4o-mini`) |
| `EMBEDDING_MODEL` | worker (default: `text-embedding-3-small`) |
| `EMBEDDING_DIMENSIONS` | worker (default: `1536`; mock тоже 1536) |
| `AI_MOCK` | все AI-клиенты (default: **`true`**) |
| `RETRIEVAL_MODE` | api (`ann` \| `brute`, default `ann`) |
| `RETRIEVAL_TOP_K` | api (default `50`) |
| `HTTP_ADDR` | api (`:8080`) |
| `WORKER_POLL_INTERVAL` | worker (`2s`) |

---

## Деплой (MVP)

Три процесса + Postgres:

```yaml
# docker-compose.yml — добавить при реализации
services:
  postgres:
    image: pgvector/pgvector:pg16
  api: ...
  worker: ...
  bot: ...
```

Для локальной разработки достаточно SQLite в `DATABASE_URL` (если реализовать драйвер).

---

## Этапы реализации

1. **domain + migrations + storage** — users, profiles, jobs
2. **api** — endpoints без AI
3. **worker** — enrich_profile с mock AI
4. **matching** — filters + scorer
5. **ai** — OpenAI extractor + embedder
6. **bot** — FSM + интеграция с api
7. **explainer** — top-3 explanations
8. **seed** — 10–15 профилей для демо

---

## Решения, которые можно отложить

| Вопрос | MVP | Позже |
|--------|-----|-------|
| Explain в api vs worker | api (проще UX) | worker при нагрузке |
| SQLite vs Postgres | Postgres | SQLite для CI |
| Webhook vs polling | polling | webhook на проде |
| Редактирование профиля | повтор enrich | partial update |
