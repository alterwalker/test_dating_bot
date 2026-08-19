# test_dating_bot

Тестовое задание: Telegram-бот для знакомств с ИИ-разметкой профилей и двусторонним подбором пар.

## Статус

**Этап: реализация MVP.** Работают api, worker, bot, seed; mock AI по умолчанию.

## Идея

1. Пользователь заполняет короткую анкету и отвечает на три промпта в Telegram.
2. **Worker** с помощью LLM извлекает структурированный профиль (теги, оси, summary) и считает embedding.
3. **API** по жёстким фильтрам и score находит совместимых кандидатов (двусторонний fit).
4. Бот показывает топ matches; при выборе анкеты — **темы для беседы и первое сообщение** (ИИ).

## Demo mode

~5000 вымышленных анкет preloaded через seed. Для OpenAI: `go run ./cmd/enrich_seed` — см. [cmd/enrich_seed/README.md](cmd/enrich_seed/README.md).  
Подробнее: [docs/DEMO.md](docs/DEMO.md).

## Три бинарника

| Бинарник | Назначение |
|----------|------------|
| `cmd/bot` | Telegram: FSM, handlers, вызовы API |
| `cmd/api` | HTTP API, БД, матчинг, постановка jobs |
| `cmd/worker` | Фон: LLM extraction, embeddings |

Подробнее: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

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
3. **Чем занимаешься** — интересы, занятость

## Стек (план)

- Go 1.22+
- PostgreSQL + **pgvector** (`pgvector/pgvector:pg16`)
- Telegram Bot API (`go-telegram-bot-api` или `telebot`)
- OpenAI (опционально): LLM `gpt-4o-mini` + `text-embedding-3-small` при `AI_MOCK=false`
- **Default:** `AI_MOCK=true` — keyword extract + deterministic embed + template explain

## Быстрый старт (после реализации)

### 1. Postgres + pgvector (Docker)

```bash
docker compose up -d
# проверка: docker compose exec postgres pg_isready -U dating -d dating_bot
```

Подробнее: [docs/DOCKER.md](docs/DOCKER.md).

### 2. Go-сервисы (на хосте)

```bash
cp .env.example .env
go run ./cmd/seed          # вымышленные анкеты
go run ./cmd/api &         # :8080
go run ./cmd/worker &      # :8081
go run ./cmd/bot           # нужен BOT_TOKEN
```

Тесты:

```bash
docker compose up -d
go test ./...
```

## Что сознательно не входит в scope

- фото / CLIP
- свайпы, чаты, лайки
- collaborative filtering
- Redis / gRPC / микросервисы сверх трёх бинарников
