# Seed: вымышленные анкеты (demo mode)

80–100 precomputed профилей для демонстрации матчинга. См. [docs/DEMO.md](../docs/DEMO.md).

## Формат `fictional_profiles.json`

```json
[
  {
    "external_id": "вымышленный_00001",
    "raw": {
      "name": "Кирилл",
      "age": 29,
      "city": "Москва",
      "gender": "male",
      "seeking": ["female"],
      "age_min": 24,
      "age_max": 34,
      "prompt_ideal_evening": "Пробежка в парке, потом готовлю ужин дома.",
      "prompt_relationship_values": "Честность, юмор и желание строить что-то вместе.",
      "prompt_occupation": "Backend-разработчик, бегаю полумарафоны, люблю готовить."
    },
    "enriched": {
      "interests": ["бег", "готовка", "it"],
      "values": ["честность", "юмор"],
      "lifestyle_axes": {
        "outgoing": 0.4,
        "family_oriented": 0.7,
        "career_focused": 0.6,
        "adventurous": 0.5,
        "homebody": 0.65
      },
      "relationship_intent": "serious",
      "communication_style": "спокойный, с юмором",
      "dealbreakers_detected": [],
      "summary": "Разработчик, бегает и готовит дома. Ищет серьёзные отношения."
    },
    "embedding": [0.012, -0.034],
    "embedding_model": "mock-deterministic-v1",
    "skip_llm": true
  }
]
```

**Важно:**
- `external_id` — `вымышленный_NNNNN` (5 цифр), **не** `telegram_id`
- `embedding` в seed — полный массив 1536 float (в примере сокращён)
- `skip_llm: true` — seed loader не вызывает worker

## Объём и разнообразие

| Параметр | Значение |
|----------|----------|
| Количество | 80–100 |
| ID | `вымышленный_00001` … `вымышленный_00100` |
| Пол | ~50/50 |
| Города | Москва 70%, СПб 20%, прочие 10% |
| Intent mix | serious 60%, casual 20%, unsure 20% |

## Загрузка

```bash
go run ./cmd/seed --file seed/fictional_profiles.json
```

## Генерация (один раз)

1. Написать raw-тексты (скрипт / LLM batch / вручную)
2. Прогнать через `AI_MOCK=true` enrich локально → сохранить enriched + embedding
3. Зафиксировать JSON в репозитории

При `docker compose up` seed идемпотентен: `ON CONFLICT (external_id) DO NOTHING`.

## Legacy

`profiles.json` с `telegram_id: 900001` — **deprecated**, заменён на `fictional_profiles.json`.
