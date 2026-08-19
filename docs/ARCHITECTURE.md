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

**Зависимости:** inline HTTP-клиент + конфиг (`BOT_TOKEN`, `API_BASE_URL`).

---

### 2. `cmd/api` — Platform API

**Ответственность:**
- CRUD пользователей и профилей (включая удаление анкеты);
- создание job `enrich_profile`;
- синхронный матчинг (`GET /matches`), скрытие кандидатов;
- LLM explain для top-3 и icebreaker по запросу;
- admin-статистика (города, token usage).

**Не содержит:** Telegram SDK, FSM.

**Endpoints:** см. [API.md](API.md). Миграции — SQL-файлы в `migrations/`, применяются вручную или через init Postgres.

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
├── domain/          # User, Profile, Job, Match, AdminStats
├── config/          # env-конфиг
├── storage/         # Postgres, pgvector, jobs, match_hides
├── profile/         # lifecycle профиля (api)
├── matching/
│   ├── scorer.go    # FilterCandidates, TopRetrieve, RankMatches, ScorePair
│   └── service.go   # FindMatches, Icebreaker, HideMatch
├── ai/
│   ├── client.go    # интерфейс Client + factory
│   ├── openai.go    # Extract, Embed, Explain, Icebreaker
│   ├── mock.go      # mock-реализация (default)
│   └── usage.go     # учёт токенов OpenAI
├── jobs/
│   └── processor.go # enrich_profile handler
├── api/
│   └── server.go    # HTTP handlers
├── seedgen/         # generate_seed
└── seedenrich/      # enrich_seed CLI logic
```

Промпты лежат в `prompts/` и подгружаются из `internal/ai/openai.go` (константы) и шаблонов в docs.

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
  → bot показывает summary, auto-confirm
  → api POST /profile/confirm
```

Редактирование: bot → review по полям → повторный enrich.  
Удаление: `DELETE /profile` — сброс анкеты в `draft`.

### Подбор пар

```
User → bot «Найти пары»
  → api GET /matches?limit=10
  → filters → retrieve (top-K) → rank (two-sided)
  → api: explainer для top-3 (LLM)
  → bot: карточки с score + explanation
  → bot: скрытие кандидата POST .../hide (не показывать снова)
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

match(A,B) = 0.8 * harmonic_mean(pref(A→B), pref(B→A))
           + 0.2 * cosine(embedding_A, embedding_B)
```

Итоговый score в диапазоне 0–1 (100%).

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

Postgres через [docker-compose.yml](../docker-compose.yml); Go-сервисы — на хосте (см. [README.md](../README.md) и [DOCKER.md](DOCKER.md)):

```bash
docker compose up -d
go run ./cmd/seed
go run ./cmd/api &
go run ./cmd/worker &
go run ./cmd/bot
```

---

## Статус реализации

MVP закрыт. Дополнительные утилиты: `cmd/generate_seed`, `cmd/enrich_seed`, `cmd/seed`.

| Компонент | Статус |
|-----------|--------|
| domain + migrations + storage | ✅ |
| api endpoints | ✅ |
| worker enrich_profile | ✅ |
| matching (filters + ANN + rank) | ✅ |
| ai mock + OpenAI | ✅ |
| bot FSM + matches + icebreaker | ✅ |
| demo seed (~5000 fictional) | ✅ |
| profile edit / delete, match hide | ✅ |

---

## Решения, которые можно отложить

| Вопрос | MVP | Позже |
|--------|-----|-------|
| Explain в api vs worker | api (проще UX) | worker при нагрузке |
| SQLite vs Postgres | Postgres | SQLite для CI |
| Webhook vs polling | polling | webhook на проде |
| Редактирование профиля | ✅ review + re-enrich | partial update без re-embed |
| Auth / rate limits | — | JWT, Telegram initData, admin token |
