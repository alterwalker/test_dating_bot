# enrich_seed — OpenAI enrich для вымышленных анкет

Пересчитывает **enriched** и **embedding** в `seed/fictional_profiles.json` через реальный OpenAI (или mock с `-allow-mock`).

## Зачем

`generate_seed` создаёт тексты и mock-enriched. Для семантического матчинга все 5000 анкет должны быть embed'нуты **одной моделью** — той же, что использует worker для живых пользователей.

## Что делает команда

Для каждой записи в JSON:

1. **Extract** — `raw` → `enriched` (gpt-4o-mini, JSON)
2. **Embed** — текст профиля → вектор 1536 (text-embedding-3-small)
3. Сохраняет `embedding`, `embedding_model`, обновлённый `enriched` в JSON
4. `relationship_intent` из `raw` сохраняется как задан пользователем/генератором

Опционально (`-load-db`): сразу **upsert** в Postgres (`InsertFictionalProfile`).

## Требования

```env
AI_MOCK=false
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o-mini
EMBEDDING_MODEL=text-embedding-3-small
```

## Примеры

```bash
# полный прогон 5000 анкет, перезапись файла, 5 параллельных запросов
AI_MOCK=false go run ./cmd/enrich_seed -in-place -workers 5

# тест на 10 анкетах в отдельный файл
AI_MOCK=false go run ./cmd/enrich_seed -limit 10 -out seed/sample.openai.json

# продолжить после сбоя — по умолчанию пропускает записи с embedding
AI_MOCK=false go run ./cmd/enrich_seed -in-place -workers 5

# пересчитать всё заново (включая уже обогащённые)
AI_MOCK=false go run ./cmd/enrich_seed -in-place -skip-existing=false -workers 5

# enrich + сразу в БД
AI_MOCK=false go run ./cmd/enrich_seed -in-place -workers 5 -load-db
```

## Флаги

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `-file` | `seed/fictional_profiles.json` | входной JSON |
| `-out` | — | выходной JSON (если не `-in-place`) |
| `-in-place` | false | перезаписать `-file` |
| `-offset` | 0 | с какого индекса начать (resume) |
| `-limit` | 0 | сколько обработать (0 = до конца) |
| `-workers` | 3 | параллельных запросов к OpenAI |
| `-load-db` | false | upsert в Postgres после каждого профиля |
| `-allow-mock` | false | разрешить `AI_MOCK=true` (тесты) |
| `-skip-existing` | **true** | пропускать записи с непустым `embedding` |

## Поведение при сбоях

- до **3 retry** с backoff на каждый профиль
- checkpoint каждые **25** успешных профилей — промежуточная запись JSON
- `Ctrl+C` — контекст отменяется, уже записанный checkpoint сохраняется

## Стоимость (ориентир)

~5000 профилей ≈ **10 000** API-вызовов (extract + embed).  
При gpt-4o-mini + embedding-3-small — порядка **$2–5** (зависит от длины текстов).

## Полный pipeline OpenAI-demo

```bash
go run ./cmd/generate_seed          # тексты + mock enriched
AI_MOCK=false go run ./cmd/enrich_seed -in-place -workers 5 -load-db
# или без -load-db:
go run ./cmd/seed

# api + worker тоже с AI_MOCK=false
go run ./cmd/api &
go run ./cmd/worker &
```

**Важно:** не смешивайте mock-embeddings и OpenAI-embeddings в одной БД. При смене режима — `docker compose down -v` и полный re-seed.
