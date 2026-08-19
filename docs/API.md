# HTTP API

Base URL: `http://localhost:8080/v1`

Все ответы — JSON. Ошибки: `{ "error": "message", "code": "NOT_FOUND" }`.

---

## Users

### `POST /users`

Регистрация или получение пользователя по Telegram ID.

**Request:**
```json
{
  "telegram_id": 123456789,
  "username": "ivan_petrov"
}
```

**Response 200:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "user_kind": "telegram",
  "telegram_id": 123456789,
  "external_id": null,
  "created_at": "2026-08-19T12:00:00Z"
}
```

Вымышленные пользователи (seed) имеют `user_kind: "fictional"`, `external_id: "вымышленный_00001"`, `telegram_id: null`. См. [DEMO.md](DEMO.md).

---

## Profile — raw (анкета)

### `GET /users/{user_id}/profile`

**Response 200:**
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "draft",
  "raw": {
    "name": "Анна",
    "age": 28,
    "city": "Москва",
    "gender": "female",
    "seeking": ["male"],
    "age_min": 25,
    "age_max": 35,
    "prompt_ideal_evening": "Пробежка в парке, потом ужин дома с книгой.",
    "prompt_relationship_values": "Честность, поддержка, общие планы на будущее.",
    "prompt_occupation": "Product designer, бегаю марафоны, есть собака."
  },
  "enriched": null,
  "confirmed_at": null
}
```

`status`: `draft` | `processing` | `ready` | `confirmed`

---

### `PUT /users/{user_id}/profile/raw`

Сохранить или обновить сырые поля. Частичное обновление допустимо.

**Request:**
```json
{
  "name": "Анна",
  "age": 28,
  "city": "Москва",
  "gender": "female",
  "seeking": ["male"],
  "age_min": 25,
  "age_max": 35,
  "prompt_ideal_evening": "...",
  "prompt_relationship_values": "...",
  "prompt_occupation": "..."
}
```

**Response 200:** полный объект профиля (как GET).

---

### `POST /users/{user_id}/profile/enrich`

Поставить задачу на ИИ-разметку. Требует заполненные три промпта.

**Response 202:**
```json
{
  "status": "processing",
  "job_id": "job-uuid"
}
```

**Response 409:** профиль уже `processing` или нет обязательных полей.

---

### `GET /users/{user_id}/profile/status`

Лёгкий polling для бота.

**Response 200:**
```json
{
  "status": "ready",
  "job_id": "job-uuid",
  "error": null
}
```

При `failed`: `error` содержит текст для логов (бот показывает generic message).

---

### `POST /users/{user_id}/profile/confirm`

Пользователь принял разметку.

**Response 200:**
```json
{
  "status": "confirmed",
  "confirmed_at": "2026-08-19T12:05:00Z"
}
```

---

## Matches

### `GET /users/{user_id}/matches`

**Query:**
- `limit` (default 10, max 20)

**Response 200:**
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "matches": [
    {
      "candidate_id": "...",
      "candidate_name": "Кирилл",
      "candidate_age": 29,
      "is_fictional": true,
      "external_id": "вымышленный_00012",
      "score": 0.82,
      "breakdown": {
        "pref_a_to_b": 0.79,
        "pref_b_to_a": 0.85,
        "harmonic": 0.82,
        "embedding_similarity": 0.71,
        "shared_interests": ["бег", "собаки"],
        "shared_values": ["честность"]
      },
      "summary": "Активный, ценит честность...",
      "explanation": "Вам обоим важна честность и активный образ жизни..."
    }
  ],
  "total_candidates_after_filters": 12
}
```

**Response 409:** профиль не в статусе `confirmed`.

**Query (optional):**
- `include_fictional` (default: значение `DEMO_MODE` на сервере)

---

### `POST /users/{user_id}/matches/{candidate_id}/icebreaker`

Пользователь выбрал анкету из matches — сгенерировать темы для беседы и первое сообщение.

**Response 200:**
```json
{
  "viewer_id": "550e8400-e29b-41d4-a716-446655440000",
  "candidate_id": "...",
  "candidate_name": "Кирилл",
  "shared_interests": ["бег", "собаки"],
  "shared_values": ["честность"],
  "conversation_topics": [
    "Какие дистанции бегаете чаще — 5 км или длиннее?",
    "Как совмещаете пробежки с работой?",
    "Есть любимый парк для бега?"
  ],
  "opener_message": "Привет! Заметил, что ты тоже бегаешь — как давно этим занимаешься?"
}
```

**Response 404:** candidate не найден или не был в допустимом pool matches.  
**Response 409:** viewer profile не `confirmed`.

Синхронный вызов LLM в api (как explain). Промпты: `prompts/icebreaker.*`.

---

## Health

### `GET /health`

```json
{ "status": "ok" }
```

Worker: `GET :8081/health` — аналогично.

---

## Enum values

**gender:** `male` | `female` | `other`

**seeking:** массив из `male` | `female` | `other`

**relationship_intent** (из enriched): `serious` | `casual` | `friendship` | `unsure`
