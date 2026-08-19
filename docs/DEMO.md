# Demo mode (демонстрационный режим)

Режим для тестового задания и демо без живой пользовательской базы: **большой пул вымышленных анкет** + **icebreaker** при выборе match.

---

## Цели

1. Показать матчинг сразу после создания своего профиля — в ленте есть кого подобрать.
2. Не требовать десятков реальных Telegram-пользователей.
3. Продемонстрировать **второй ход ИИ** — темы для беседы и первое сообщение по общим интересам.

---

## Вымышленные анкеты

### Идентификатор

У вымышленных пользователей **нет** `telegram_id`. Вместо него:

```
вымышленный_00001
вымышленный_00002
…
вымышленный_00100
```

Формат: префикс `вымышленный_` + zero-padded номер (5 цифр).

### Отличие от реальных пользователей

| | Реальный пользователь | Вымышленная анкета |
|---|----------------------|-------------------|
| `user_kind` | `telegram` | `fictional` |
| `telegram_id` | BIGINT | `NULL` |
| `external_id` | `NULL` | `вымышленный_00042` |
| Создание | bot `/start` | seed / `cmd/seed` |
| FSM анкеты | да | нет |
| Статус профиля | draft → … → confirmed | сразу `confirmed` |
| Участие в matches | как viewer и candidate | **только candidate** |

### Объём seed

| Параметр | Значение |
|----------|----------|
| Количество | **80–100** анкет (минимум 50 для demo) |
| Города | Москва (70%), Санкт-Петербург (20%), другие (10%) |
| Пол | ~50/50 |
| enriched + embedding | precomputed (`skip_llm: true` в seed) |

Генерация текстов: один раз через OpenAI или скрипт + ручная правка → зафиксировать в `seed/fictional_profiles.json`.

---

## Конфигурация

```env
DEMO_MODE=true              # включить вымышленных в matches (default true для test)
DEMO_FICTIONAL_PREFIX=вымышленный_
DEMO_SEED_FILE=seed/fictional_profiles.json
```

При `DEMO_MODE=false` вымышленные профили остаются в БД, но **исключаются** из retrieval (`user_kind = fictional` filter out).

---

## Матчинг в demo mode

Логика **не меняется** — fictional profiles это обычные строки в `profiles` со status `confirmed`.

```sql
-- retrieval: только confirmed, viewer != candidate
WHERE p.status = 'confirmed'
  AND p.user_id != $viewer_id
  AND ($demo_mode OR u.user_kind != 'fictional')  -- inverted: when demo, include fictional
```

На практике проще:

```
if DEMO_MODE:
  candidates = all confirmed except self
else:
  candidates = confirmed AND user_kind = 'telegram' except self
```

---

## Icebreaker — темы и первое сообщение

Когда пользователь **выбирает** одну из предложенных анкет (inline-кнопка «💬 Начать общение»):

```
User tap → bot → POST /matches/{candidate_id}/icebreaker
              → api: load viewer + candidate profiles
              → ai.Icebreaker(shared interests, values, summaries)
              → bot: показать 3–5 тем + 1 готовое сообщение
```

### Ответ API

```json
{
  "candidate_id": "...",
  "candidate_name": "Кирилл",
  "shared_interests": ["бег", "собаки"],
  "shared_values": ["честность"],
  "conversation_topics": [
    "Какие маршруты для пробежек предпочитаете?",
    "Собака — лабrador. А у вас какая порода?",
    "Как совмещаете бег с плотным графиком?"
  ],
  "opener_message": "Привет! Заметил, что ты тоже бегаешь — как часто выходишь на дистанцию?"
}
```

### Mock (`AI_MOCK=true`)

Шаблоны без LLM — см. [AI.md](AI.md#mock-icebreaker).

### OpenAI (`AI_MOCK=false`)

Промпты: [prompts/icebreaker.system.md](../prompts/icebreaker.system.md).

---

## Bot UX (дополнение к FSM)

После списка matches каждая карточка:

```
👤 Кирилл, 29 · ⭐ 82%
{explanation}

[💬 Начать общение]
```

После нажатия — экран icebreaker:

```
💡 Темы для разговора:
1. …
2. …
3. …

✉️ Можно начать так:
«…»

[📋 Скопировать]  [🔍 Другие пары]
```

`Скопировать` — отправить текст сообщения отдельным сообщением (удобно copy-paste).

---

## Схема БД

Миграция `002_demo_fictional_users.sql`:

- `user_kind` enum: `telegram` | `fictional`
- `telegram_id` → nullable
- `external_id TEXT UNIQUE`
- check constraint: ровно один идентификатор по kind

---

## Seed loader

```bash
go run ./cmd/seed --file seed/fictional_profiles.json
```

Для каждой записи:

1. `INSERT users (user_kind, external_id)`
2. `INSERT profiles (raw, enriched, embedding, status=confirmed, confirmed_at=now())`
3. без job `enrich_profile`

---

## Чеклист реализации

- [ ] migration 002 — `user_kind`, `external_id`
- [ ] `seed/fictional_profiles.json` — 80–100 анкет
- [ ] `cmd/seed` — загрузчик
- [ ] matching — `DEMO_MODE` filter
- [ ] `POST .../icebreaker` endpoint
- [ ] `internal/ai/icebreaker.go` — mock + openai
- [ ] bot FSM — `MatchSelected` → icebreaker screen
- [ ] prompts/icebreaker.*
