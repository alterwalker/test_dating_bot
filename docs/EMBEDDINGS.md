# Embeddings и ANN

Как создаём векторы, какую модель используем, где храним и как ищем похожих через pgvector.

---

## Две разные модели — не путать

| Задача | Модель | Тип | Где |
|--------|--------|-----|-----|
| Разметка профиля (теги, оси, summary) | `gpt-4o-mini` | LLM (chat) | worker → extractor |
| Семантический вектор профиля | `text-embedding-3-small` | **Embedding model** | worker → embedder |

**LLM** понимает текст и возвращает JSON.  
**Embedding model** превращает текст в фиксированный вектор чисел — «координаты смысла» в 1536-мерном пространстве.

Похожие по смыслу тексты → близкие векторы (высокий cosine similarity).

---

## Выбор embedding-модели

### Основная: OpenAI `text-embedding-3-small`

| Параметр | Значение |
|----------|----------|
| Размерность | **1536** |
| Языки | хорошо работает с русским |
| API | `POST /v1/embeddings` |
| Стоимость | дёшево (~$0.02 / 1M tokens) |
| Конфиг | `EMBEDDING_MODEL=text-embedding-3-small` |

**Почему small, а не large:** для коротких профилей (3 промпта + summary) small достаточно. Large (3072 dims) — избыточен и дороже по памяти в pgvector.

### Альтернативы (не в MVP)

| Модель | Когда |
|--------|-------|
| `text-embedding-3-large` | если quality недостаточен |

**Mock и OpenAI:** см. [AI.md](AI.md). Default — mock (`AI_MOCK=true`), та же размерность 1536.

**Важно:** размерность и модель должны быть **одинаковыми для всех профилей** в базе. Смена модели = пересчёт всех embeddings.

---

## Из чего собирается текст для embedding

Embedding считается **после** LLM extraction, в worker:

```
1. LLM extract → enriched (interests, values, summary, …)
2. buildEmbeddingText(raw, enriched) → string
3. embedder.Embed(string) → []float32 len=1536
4. SAVE profiles.embedding
```

### Функция `buildEmbeddingText`

```go
func buildEmbeddingText(raw RawProfile, enriched EnrichedProfile) string {
    var b strings.Builder
    b.WriteString("Идеальный вечер: ")
    b.WriteString(raw.PromptIdealEvening)
    b.WriteString("\nЦенности: ")
    b.WriteString(raw.PromptRelationshipValues)
    b.WriteString("\nЗанятость: ")
    b.WriteString(raw.PromptOccupation)
    b.WriteString("\nSummary: ")
    b.WriteString(enriched.Summary)
    return b.String()
}
```

### Что сознательно НЕ кладём в embedding

| Поле | Почему |
|------|--------|
| имя, возраст, пол | фильтры, не семантика |
| город | фильтр |
| interests/values как JSON | уже в тексте промптов; дублирование для Jaccard отдельно |
| lifestyle_axes | числа; для них axis_similarity в ranker |

---

## Пайплайн в worker (`enrich_profile`)

```
┌─────────────────────────────────────────────────────────┐
│ job enrich_profile                                      │
├─────────────────────────────────────────────────────────┤
│ 1. Load raw profile from DB                             │
│ 2. ai.Extract(raw)           → enriched JSON            │
│ 3. text := buildEmbeddingText(raw, enriched)            │
│ 4. vec := ai.Embed(text)      → []float32[1536]         │
│ 5. UPDATE profiles SET                                  │
│      enriched = $1,                                      │
│      embedding = $2,                                     │
│      embedding_model = $3,                               │
│      embedded_at = now(),                                │
│      status = 'ready'                                    │
└─────────────────────────────────────────────────────────┘
```

### Go-интерфейс (`internal/ai/embedder.go`)

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Model() string // "text-embedding-3-small"
    Dimensions() int // 1536
}
```

### OpenAI-реализация

```go
resp, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
    Model: openai.SmallEmbedding3,
    Input: text,
})
// resp.Data[0].Embedding → []float32
```

### Mock-реализация (`AI_MOCK=true`, default)

Детерминированный псевдо-вектор из SHA256 текста → 1536 floats, L2-normalized.  
Размерность **совпадает с OpenAI** — одна схема БД. Semantic similarity в mock слабая; ANN тестируется технически.  
Подробнее: [AI.md](AI.md).

---

## Где храним

**Postgres + pgvector**, колонка `profiles.embedding`.

```sql
embedding         vector(1536) NOT NULL,
embedding_model   TEXT NOT NULL DEFAULT 'text-embedding-3-small',
embedded_at       TIMESTAMPTZ,
```

Один вектор на пользователя. При изменении текстов промптов — новый job → overwrite.

### HNSW-индекс для ANN

```sql
CREATE INDEX idx_profiles_embedding_hnsw
    ON profiles USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
```

| Параметр | Значение | Смысл |
|----------|----------|-------|
| `vector_cosine_ops` | cosine distance | `<=>` в SQL |
| `m = 16` | связей на узел | баланс speed/recall |
| `ef_construction = 64` | точность build | для маленькой базы достаточно |

На dev с 10 профилями индекс работает, но выигрыш по скорости незаметен. Архитектура готова к росту.

---

## ANN retrieval в матчинге

### Этап 2: Retrieval (pgvector ANN)

```sql
SELECT
    p.user_id,
    1 - (p.embedding <=> $1::vector) AS embedding_similarity
FROM profiles p
WHERE p.status = 'confirmed'
  AND p.user_id != $2
  AND p.raw->>'city' = $3
  AND ... -- gender, age, intent filters
ORDER BY p.embedding <=> $1::vector
LIMIT 50;
```

- `$1` — embedding запрашивающего пользователя
- `<=>` — cosine **distance** (0 = идентичны, 2 = противоположны)
- `1 - distance` ≈ similarity

### Гибридный retrieval score

ANN даёт top-50 по embedding. Дополнительно в Go можно доскорить:

```
retrieve_score = 0.5 * embedding_similarity
               + 0.3 * jaccard(interests)
               + 0.2 * jaccard(values)
```

И взять top-20 для ranker.

### Fallback `RETRIEVAL_MODE=brute`

Для unit-тестов без Postgres pgvector — перебор в Go.  
В prod и docker-compose — всегда `ann`.

---

## Docker: Postgres с pgvector

```yaml
postgres:
  image: pgvector/pgvector:pg16
  environment:
    POSTGRES_USER: dating
    POSTGRES_PASSWORD: dating
    POSTGRES_DB: dating_bot
```

Образ `pgvector/pgvector` уже содержит расширение — `CREATE EXTENSION vector` в миграции.

---

## Конфигурация

| Переменная | Default | Кто |
|------------|---------|-----|
| `EMBEDDING_MODEL` | `text-embedding-3-small` | worker |
| `EMBEDDING_DIMENSIONS` | `1536` | worker, migrations |
| `OPENAI_API_KEY` | — | worker |
| `AI_MOCK` | `true` | worker, api |
| `RETRIEVAL_MODE` | `ann` | api (`ann` \| `brute`) |
| `RETRIEVAL_TOP_K` | `50` | api |

---

## Пересчёт embeddings

| Событие | Действие |
|---------|----------|
| Новый профиль | embed в `enrich_profile` |
| Изменены промпты | новый `enrich_profile` |
| Смена `EMBEDDING_MODEL` | batch job `reembed_all` (не в MVP) |
| Seed | precomputed vectors в seed loader |

---

## Зависимости Go (при реализации)

```
github.com/sashabaranov/go-openai     # Embeddings API
github.com/pgvector/pgvector-go       # scan/write vector(1536) in pgx
github.com/jackc/pgx/v5
```

---

## Чеклист для реализации

- [ ] `CREATE EXTENSION vector` в миграции
- [ ] `embedding vector(1536)` + HNSW index
- [ ] `internal/ai/embedder.go` — OpenAI + Mock
- [ ] `buildEmbeddingText()` в worker handler
- [ ] `internal/matching/retrieval_ann.go` — SQL с `<=>`
- [ ] `RETRIEVAL_MODE=brute` для тестов
- [ ] seed: embeddings precomputed тем же текстом и моделью
