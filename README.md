# test_dating_bot

Тестовое задание: Telegram-бот для знакомств с ИИ-разметкой профилей и двусторонним подбором пар.

## Статус

**MVP реализован.** Работают `api`, `worker`, `bot`, `seed`, `enrich_seed`; mock AI по умолчанию.

## Идея

1. Пользователь заполняет короткую анкету и отвечает на три промпта в Telegram.
2. **Worker** с помощью LLM извлекает структурированный профиль (теги, оси, summary) и считает embedding.
3. **API** по жёстким фильтрам и score находит совместимых кандидатов (двусторонний fit).
4. Бот показывает топ matches; при выборе анкеты — **темы для беседы и первое сообщение** (ИИ).

## Demo mode

~5000 вымышленных анкет загружаются через `cmd/seed`. Файл `seed/fictional_profiles.json` **не в git** (генерируется локально, после OpenAI-enrich — сотни MB). Сначала: `go run ./cmd/generate_seed`. OpenAI-enrich: `go run ./cmd/enrich_seed` — см. [cmd/enrich_seed/README.md](cmd/enrich_seed/README.md).  
Подробнее: [docs/DEMO.md](docs/DEMO.md).

## Три бинарника

| Бинарник | Назначение |
|----------|------------|
| `cmd/bot` | Telegram: FSM, handlers, вызовы API |
| `cmd/api` | HTTP API, БД, матчинг, постановка jobs |
| `cmd/worker` | Фон: LLM extraction, embeddings |

Дополнительно: `cmd/seed`, `cmd/generate_seed`, `cmd/enrich_seed` — генерация и обогащение demo-анкет.

Подробнее: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

### Масштабирование и новые каналы

Архитектура изначально **channel-agnostic**: вся бизнес-логика в `api` + `worker`, а `cmd/bot` — тонкий адаптер Telegram (FSM → HTTP). Подключить сайт, мобильное приложение или другой мессенджер **несложно**: новый gateway (`cmd/bot_vk`, web/mobile-клиент) вызывает те же REST-эндпоинты; меняется только UX-слой, не матчинг и не enrich.

| Направление | Сложность | Как |
|-------------|-----------|-----|
| **Новые источники** | низкая | Отдельный бинарник/frontend на тот же `api`; в `users` уже есть `user_kind` — можно добавить `web`, `ios` и внешний id канала |
| **Горизонтальное масштабирование** | низкая–средняя | `api` и `bot` — stateless, N реплик за load balancer. `worker` — N инстансов, jobs забираются через `FOR UPDATE SKIP LOCKED` (конкуренция уже учтена). Узкое место — Postgres и OpenAI rate limits |
| **pgvector → Qdrant** | средняя | Вынести ANN из SQL в `internal/matching/retrieval` с интерфейсом; Postgres остаётся source of truth для профилей, Qdrant — индекс embedding'ов с payload-фильтрами (город, пол, возраст). Миграция: backfill коллекций, dual-write, переключение `RETRIEVAL_MODE` |
| **OpenSearch** | средняя | Имеет смысл при гибридном поиске (BM25 по тексту анкеты + vector), faceted-фильтрах и аналитике. Не заменяет Postgres для транзакций; дублирует профиль для search-слоя |
| **Шардирование** | средняя–высокая | Жёсткие фильтры уже режут выборку — логичный ключ шарда: **`city` + `gender` + возрастной bucket** (например 5-летние диапазоны). Каждый шард — отдельная коллекция Qdrant / партиция Postgres / индекс OpenSearch. Кросс-шардовый match редок (разные города), запросы маршрутизируются по профилю viewer'а |
| **Брокер сообщений** | средняя | Сейчас очередь — таблица `jobs` + poll в worker. Kafka / RabbitMQ / NATS — следующий шаг при росте lag; см. ниже |

**Оценка по нагрузке:** до ~100k confirmed-профилей и сотен RPS на `GET /matches` текущей схемы (Postgres + pgvector HNSW + фильтры в памяти) обычно достаточно. Дальше — read replicas Postgres, вынос ANN в Qdrant, кэш top-K по `(viewer_id, shard)`, очередь jobs в отдельный broker при росте worker-лагa.

#### Брокер сообщений — куда внедрять

Сейчас асинхронность только через Postgres (`POST /profile/enrich` → `jobs` → worker poll). Брокер не обязателен в MVP, но точки расширения уже понятны:

| Место | Сейчас | С брокером | Зачем |
|-------|--------|------------|-------|
| **`enrich_profile`** | `api` пишет job в Postgres, worker poll | `api` публикует `profile.enrich.requested` → worker consume | Первый кандидат: decouple api от worker, backpressure, DLQ, масштаб worker независимо от БД |
| **Outbox → broker** | — | Транзакция в Postgres (профиль + outbox) → relay шлёт в топик | At-least-once без потери событий при падении api между commit и publish |
| **Индексация Qdrant / OpenSearch** | embedding пишется в Postgres | `profile.enriched` → indexer consumer обновляет внешний индекс | Search-слой отстаёт от OLTP; reindex и backfill без блокировки api |
| **Explain / icebreaker** | синхронно в `api` | `match.explain.requested`, `icebreaker.requested` → LLM-worker | Снять latency и rate-limit OpenAI с HTTP; bot показывает «генерирую…» |
| **Уведомления каналов** | bot poll'ит `GET /profile/status` | `profile.ready` → push consumer в bot/web | Несколько gateway без опроса api |
| **Аналитика и аудит** | не реализовано | fire-and-forget: `match.shown`, `icebreaker.opened` | DWH, prompt tuning, offline-оценка подбора |
| **Webhook Telegram** | long polling в одном `cmd/bot` | ingress → `telegram.updates` → N bot-worker | Горизонтальное масштабирование bot при webhook |

Стартовые топики: **`profile.enrich`** (priority) + **`search.index`** (best-effort). Миграция: dual-write (Postgres jobs + publish) → consumer-only → убрать poll.

```
                    ┌─ cmd/bot (Telegram)
                    ├─ cmd/bot_* (другие каналы)
  Клиенты ──────────┼─ Web / Mobile app
                    └─ HTTP/JSON
                           │
                      cmd/api × N
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
         PostgreSQL    Message       Qdrant /
         (профили,     broker         OpenSearch
          outbox)     (Kafka /       (опционально)
              │        RabbitMQ)
              │            │
              │     ┌──────┴──────┬──────────────┐
              │     ▼             ▼              ▼
              │  enrich        index         analytics
              │  workers       workers       consumers
              └──────┬─────────┘
                     ▼
              cmd/worker × N  →  OpenAI
```

## Документация

**AI по умолчанию:** `AI_MOCK=true` — без OpenAI. Для OpenAI: `AI_MOCK=false` + ключ — см. [docs/AI.md](docs/AI.md).

| Файл | Содержание |
|------|------------|
| [docs/AI.md](docs/AI.md) | Mock vs OpenAI, интерфейсы, icebreaker |
| [docs/DEMO.md](docs/DEMO.md) | Demo mode, вымышленные анкеты, icebreaker |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Модули, границы, потоки данных, деплой |
| [docs/API.md](docs/API.md) | HTTP-контракты между bot ↔ api |
| [docs/FSM.md](docs/FSM.md) | Сценарий диалога, icebreaker flow |
| [docs/DATA_MODEL.md](docs/DATA_MODEL.md) | Сущности, JSON профиля, схема БД |
| [docs/DOCKER.md](docs/DOCKER.md) | Postgres + pgvector через docker compose |
| [prompts/](prompts/) | System/user промпты и JSON Schema |

## Промпты анкеты (для пользователя)

1. **Идеальный вечер** — lifestyle, темп жизни
2. **Что для тебя важно в отношениях** — ценности, intent
3. **Ваши интересы помимо работы** — хобби, увлечения

## Стек

- **Go 1.25+** (версия в [go.mod](go.mod))
- PostgreSQL + **pgvector** (`pgvector/pgvector:pg16`)
- Telegram Bot API (long polling, прямые HTTP-запросы)
- OpenAI (опционально): LLM `gpt-4o-mini` + `text-embedding-3-small` при `AI_MOCK=false`
- **Default:** `AI_MOCK=true` — keyword extract + deterministic embed + template explain

## Безопасность (MVP)

Для тестового MVP **намеренно не реализованы** auth, проверка владельца `user_id` и защита admin-эндпоинтов. Любой клиент с UUID может вызывать API; `GET /v1/admin/stats/cities` публичен. Это осознанный компромисс скорости разработки — в production-плане: JWT или Telegram `initData`, rate limits, закрытый admin.

## Быстрый старт

### 1. Postgres + pgvector (Docker)

```bash
docker compose up -d
# проверка: docker compose exec postgres pg_isready -U dating -d dating_bot
```

Подробнее: [docs/DOCKER.md](docs/DOCKER.md) (локальный Postgres).  
**Сервер (Ubuntu):** [docs/DEPLOY.md](docs/DEPLOY.md) — rsync, `docker-compose.prod.yml` (Postgres + api + worker + bot в Docker).

### 2. Go-сервисы (на хосте)

```bash
cp .env.example .env   # BOT_TOKEN, при необходимости OPENAI_API_KEY
go run ./cmd/generate_seed   # seed/fictional_profiles.json (~5000, mock)
go run ./cmd/seed              # загрузка в Postgres
go run ./cmd/api &     # :8080
go run ./cmd/worker &  # :8081
go run ./cmd/bot       # нужен BOT_TOKEN
```

Тесты:

```bash
docker compose up -d
go test ./...
```

## Что сознательно не входит в scope

| Исключено | Почему |
|-----------|--------|
| **Фото / CLIP** | Матчинг только по тексту анкеты и embedding; анализ изображений — отдельная задача |
| **Свайпы, чаты, лайки** | Нет механики взаимодействия между пользователями — только подбор и icebreaker |
| **Collaborative filtering** | Подбор по **содержимому профиля** (теги, оси, embedding), а не по поведению толпы. Collaborative filtering — это «пользователям, похожим на вас, понравился X» (как рекомендации Netflix/Spotify); для него нужны массовые лайки/матчи/клики, которых в MVP нет |
| **Redis / gRPC / лишние микросервисы** | Очередь jobs — в Postgres; достаточно трёх бинарников + БД |
| **Системные метрики** | Нет Prometheus/Grafana, алертов, трейсинга и дашбордов по latency/ошибкам api, worker, bot |
| **Бизнес-метрики** | Нет воронок (регистрация → анкета → match → icebreaker), retention, конверсий и A/B-срезов |
| **Логирование матчей и действий** | Не пишется история показов, кликов, выбранных кандидатов и feedback — нельзя автоматически ловить ошибки подбора и итеративно подкручивать промпты по данным |
| **Auth и authorization API** | См. [Безопасность (MVP)](#безопасность-mvp) — отложено, но заложено в roadmap масштабирования |
