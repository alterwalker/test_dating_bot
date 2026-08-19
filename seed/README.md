# Seed data

10–15 синтетических профилей для демо матчинга без живых пользователей.

## Формат `profiles.json`

```json
[
  {
    "telegram_id": 900001,
    "raw": {
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
    },
    "enriched": { "...": "см. docs/DATA_MODEL.md" },
    "skip_llm": true
  }
]
```

## План

- 6 женских, 6 мужских профилей, один город (Москва)
- 2–3 пары с высоким expected match для проверки scorer
- 1–2 профиля с явными dealbreakers для проверки filters
- `cmd/seed` или `go run ./cmd/seed` — загрузка через API или напрямую в БД

## При реализации

1. Сгенерировать enriched JSON (можно один раз через OpenAI и сохранить)
2. Precompute embeddings и положить в seed
3. Seed не вызывает LLM при каждом `docker compose up`
