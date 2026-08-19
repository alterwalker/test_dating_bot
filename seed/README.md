# Seed: вымышленные анкеты (demo mode)

~5000 precomputed профилей для демонстрации матчинга. См. [docs/DEMO.md](../docs/DEMO.md).

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
      "relationship_intent": "serious",
      "prompt_ideal_evening": "Пробежка в парке, потом готовлю ужин дома.",
      "prompt_relationship_values": "Честность, юмор и желание строить что-то вместе.",
      "prompt_occupation": "Увлекаюсь бегом и готовкой, часто хожу в горы."
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
    "embedding": null,
    "embedding_model": "mock-deterministic-v1",
    "skip_llm": true
  }
]
```

**Важно:**
- `external_id` — `вымышленный_NNNNN` (5 цифр), **не** `telegram_id`
- `embedding: null` — seed loader вычисляет mock-embedding при загрузке
- `skip_llm: true` — seed loader не ставит job `enrich_profile`

## Объём и разнообразие

| Параметр | Значение |
|----------|----------|
| Количество | **5000** (4920 узкий диапазон + 80 широкий 18–55) |
| ID | `вымышленный_00001` … `вымышленный_05000` |
| Пол | ~50/50 |
| Города | Москва 70%, СПб 20%, прочие 10% |
| Intent mix | serious 60%, casual 20%, unsure 20% |

## Загрузка

```bash
go run ./cmd/seed --file seed/fictional_profiles.json
```

Загрузка ~5000 профилей занимает 1–2 минуты (вычисление mock-embeddings).

## Генерация

**Шаг 1 — тексты (mock enriched, без OpenAI):**

```bash
go run ./cmd/generate_seed -count 4920 -wide 80 -seed 42 -out seed/fictional_profiles.json
```

**Шаг 2 — OpenAI enrich + embeddings (перед prod/demo с реальной моделью):**

```bash
AI_MOCK=false OPENAI_API_KEY=sk-... \
  go run ./cmd/enrich_seed -in-place -workers 5
```

См. [cmd/enrich_seed/README.md](../cmd/enrich_seed/README.md).

**Шаг 3 — загрузка в Postgres:**

```bash
go run ./cmd/seed
```

Флаг `-load-db` у `enrich_seed` объединяет шаги 2 и 3.

## Legacy

`profiles.json` с `telegram_id: 900001` — **deprecated**, заменён на `fictional_profiles.json`.
