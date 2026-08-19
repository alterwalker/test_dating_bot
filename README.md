# test_dating_bot

Тестовое задание: Telegram-бот для знакомств с ИИ-разметкой профилей и двусторонним подбором пар.

## Статус

**Этап: спецификация.** Код не реализован. Зафиксированы архитектура, API-контракты, FSM бота и промпты.

## Идея

1. Пользователь заполняет короткую анкету и отвечает на три промпта в Telegram.
2. **Worker** с помощью LLM извлекает структурированный профиль (теги, оси, summary) и считает embedding.
3. **API** по жёстким фильтрам и score находит совместимых кандидатов (двусторонний fit).
4. Бот показывает топ matches с breakdown и кратким объяснением от LLM.

## Три бинарника

| Бинарник | Назначение |
|----------|------------|
| `cmd/bot` | Telegram: FSM, handlers, вызовы API |
| `cmd/api` | HTTP API, БД, матчинг, постановка jobs |
| `cmd/worker` | Фон: LLM extraction, embeddings |

Подробнее: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

## Документация

## AI-режим

**По умолчанию:** `AI_MOCK=true` — работает без OpenAI и без интернета.  
**OpenAI:** `AI_MOCK=false` + `OPENAI_API_KEY` — см. [docs/AI.md](docs/AI.md).

| Файл | Содержание |
|------|------------|
| [docs/AI.md](docs/AI.md) | Mock vs OpenAI, интерфейсы, переключение |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Модули, границы, потоки данных, деплой |
| [docs/API.md](docs/API.md) | HTTP-контракты между bot ↔ api |
| [docs/FSM.md](docs/FSM.md) | Сценарий диалога в Telegram |
| [docs/DATA_MODEL.md](docs/DATA_MODEL.md) | Сущности, JSON профиля, схема БД |
| [docs/EMBEDDINGS.md](docs/EMBEDDINGS.md) | Embedding model, pgvector, ANN |
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

```bash
cp .env.example .env
# AI_MOCK=true по умолчанию — OpenAI не нужен
# для OpenAI: AI_MOCK=false и OPENAI_API_KEY=sk-...
# BOT_TOKEN — обязателен для bot

go run ./cmd/api
go run ./cmd/worker
go run ./cmd/bot
```

## Что сознательно не входит в scope

- фото / CLIP
- свайпы, чаты, лайки
- collaborative filtering
- Redis / gRPC / микросервисы сверх трёх бинарников
