# AI: Mock и OpenAI

Стратегия работы с ИИ в проекте: **по умолчанию mock**, переключение на **OpenAI одним флагом**.

---

## Решение

| Режим | Когда | API key |
|-------|-------|---------|
| **Mock** (`AI_MOCK=true`) | локальная разработка, CI, демо без интернета | не нужен |
| **OpenAI** (`AI_MOCK=false`) | prod-like прогон, сдача тестового, quality check | `OPENAI_API_KEY` обязателен |

Оба режима реализуют **одинаковые Go-интерфейсы** — worker и api не знают, какой backend внутри.

---

## Переключение

```env
# Локально / CI — default
AI_MOCK=true

# OpenAI
AI_MOCK=false
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o-mini
EMBEDDING_MODEL=text-embedding-3-small
EMBEDDING_DIMENSIONS=1536
```

### Логика при старте (`internal/ai/factory.go`)

```
if AI_MOCK == true  → NewMockClient()
else if OPENAI_API_KEY != "" → NewOpenAIClient()
else → fatal: "AI_MOCK=false but OPENAI_API_KEY is empty"
```

Mock **не** переключается автоматически при наличии ключа — только явный `AI_MOCK=false`.

---

## Четыре AI-операции

| Операция | Mock | OpenAI | Где |
|----------|------|--------|-----|
| **Extract** (raw → enriched JSON) | эвристики + keywords | `gpt-4o-mini` + JSON schema | worker |
| **Embed** (text → vector) | hash → vector[1536], L2 norm | `text-embedding-3-small` | worker |
| **Explain** (pair → текст) | шаблон из shared tags | `gpt-4o-mini`, plain text | api, matches |
| **Icebreaker** (pair → темы + opener) | шаблоны по shared tags | `gpt-4o-mini` + JSON schema | api, on match select |

Extract и Embed — **worker** (`enrich_profile`).  
Explain — **api** (`GET /matches`, top-3).  
Icebreaker — **api** (`POST /matches/{id}/icebreaker`), по действию пользователя.

---

## Mock: Extract

Файл: `internal/ai/mock_extractor.go`

Без LLM. Правила по русскому тексту:

1. **interests** — словарь ключевых слов → тег:
   - бег, марафон → `бег`
   - собак, labrador → `собаки`
   - путешеств → `путешествия`
   - готов, кулинар → `готовка`
   - дизайн, IT → `дизайн` / `it`
   - … (~30–40 entries в `mock_keywords.go`)

2. **values** — по фразам:
   - честн → `честность`
   - семь → `семья`
   - юмор → `юмор`
   - поддерж → `поддержка`

3. **lifestyle_axes** — эвристики:
   - «дома», «книг», «уют» → `homebody` ↑
   - «тусов», «друз», «бар» → `outgoing` ↑
   - «дети», «семь» → `family_oriented` ↑
   - default 0.5 где нет сигналов

4. **relationship_intent**:
   - «серьёзн», «надолго», «семь» → `serious`
   - «лёгк», «без обязательств» → `casual`
   - иначе → `unsure`

5. **dealbreakers_detected** — «не переношу курен» → `курение`

6. **summary** — шаблон:
   ```
   «{name}, {age}. Интересы: {interests}. Ценит {values}.»
   ```

7. **extraction_notes** — `"mock: keyword extraction"`

Качество ниже LLM, но pipeline и UI работают предсказуемо на seed-данных.

---

## Mock: Embed

Файл: `internal/ai/mock_embedder.go`

```go
// Псевдокод
hash := sha256(text)
vec := expandHashToFloats(hash, 1536)  // значения в [-1, 1]
vec = l2Normalize(vec)
```

- **Размерность 1536** — как у OpenAI, одна схема БД и pgvector index.
- **Детерминированность:** один и тот же текст → один и тот же вектор.
- **Похожие тексты** в mock **не** семантически близки (только exact match ≈ identical hash input). ANN работает технически; semantic quality — только с OpenAI.

`embedding_model` в БД: `"mock-deterministic-v1"`.

---

## Mock: Explain

Файл: `internal/ai/mock_explainer.go`

Шаблон без LLM:

```
«Вам близки: {shared_interests}. Общие ценности: {shared_values}.
{intent_line}»
```

- `intent_line`: если intent совпадает → «Вы оба ищете {intent_ru}.»
- если shared пусто → «Профили частично совместимы по параметрам анкеты.»

---

## Mock: Icebreaker

Файл: `internal/ai/mock_icebreaker.go`

1. **conversation_topics** — по 1 вопросу на каждый `shared_interest` (max 5):
   - `бег` → «Как часто выходите на пробежку?»
   - `собаки` → «Расскажите про вашего питомца — какая порода?»
   - fallback → «Чем увлекаетесь в свободное время?»

2. **opener_message** — шаблон:
   ```
   «Привет! Увидел, что нам обоим близки {interest_1} и {interest_2}. Как давно этим занимаетесь?»
   ```
   Если один interest — только он. Если ноль — нейтральный opener без выдуманных хобби.

---

## OpenAI: Extract

Файл: `internal/ai/openai_extractor.go`

```
POST /v1/chat/completions
model: OPENAI_MODEL (gpt-4o-mini)
response_format: json_schema (prompts/profile.schema.json)
messages:
  - system: prompts/extractor.system.md
  - user:   build from prompts/extractor.user.template.md
```

- `temperature: 0.2` — меньше галлюцинаций
- retry: 3 attempts, exponential backoff
- validate JSON against schema before save

---

## OpenAI: Embed

Файл: `internal/ai/openai_embedder.go`

```
POST /v1/embeddings
model: text-embedding-3-small
input: buildEmbeddingText(raw, enriched)
```

См. [EMBEDDINGS.md](EMBEDDINGS.md).

`embedding_model` в БД: `"text-embedding-3-small"`.

---

## OpenAI: Explain

Файл: `internal/ai/openai_explainer.go`

```
POST /v1/chat/completions
model: gpt-4o-mini
temperature: 0.5
messages:
  - system: prompts/explainer.system.md
  - user:   build from explainer.user.template.md
```

Без JSON — plain text, max 300 tokens.

---

## OpenAI: Icebreaker

Файл: `internal/ai/openai_icebreaker.go`

```
POST /v1/chat/completions
model: gpt-4o-mini
temperature: 0.6
response_format: json_schema (prompts/icebreaker.schema.json)
messages:
  - system: prompts/icebreaker.system.md
  - user:   build from icebreaker.user.template.md
```

---

## Go-интерфейсы

```go
// internal/ai/client.go

type Extractor interface {
    Extract(ctx context.Context, raw domain.RawProfile) (domain.EnrichedProfile, error)
}

type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Model() string
    Dimensions() int
}

type Explainer interface {
    Explain(ctx context.Context, req ExplainRequest) (string, error)
}

type Icebreaker interface {
    Icebreaker(ctx context.Context, req IcebreakerRequest) (IcebreakerResult, error)
}

type Client interface {
    Extractor
    Embedder
    Explainer
    Icebreaker
    Mode() string // "mock" | "openai"
}
```

Фабрика:

```go
func NewClient(cfg Config) (Client, error) {
    if cfg.AIMock {
        return NewMockClient(cfg), nil
    }
    return NewOpenAIClient(cfg)
}
```

---

## Смешивать mock и OpenAI нельзя

В одной базе все профили должны быть embed'нуты **одной моделью**.

| Допустимо | Недопустимо |
|-----------|-------------|
| все профили mock | часть mock, часть OpenAI embeddings |
| все профили OpenAI | переключить режим без re-embed |

При смене `AI_MOCK` → `false`: truncate seed или job `reembed_all` (не в MVP).

---

## Seed-данные

`seed/profiles.json` содержит **precomputed enriched + embedding**, посчитанные в mock-режиме.

Для OpenAI-demo: отдельный `seed/profiles.openai.json` или перегенерация скриптом:

```bash
AI_MOCK=false go run ./cmd/seed --regenerate-embeddings
```

(скрипт — при реализации)

---

## Тестирование

| Тест | Режим |
|------|-------|
| unit (extractor, embedder, scorer) | mock, без сети |
| integration (worker job) | mock + testcontainers postgres |
| manual quality check | `AI_MOCK=false` + real key |

CI: всегда `AI_MOCK=true`.

---

## Зависимости

```
github.com/sashabaranov/go-openai   # только при AI_MOCK=false (или всегда, mock не импортирует)
```

OpenAI SDK подключается в `openai_*.go`; mock-файлы без внешних AI-зависимостей.

---

## Чеклист реализации

- [ ] `internal/ai/factory.go` — выбор mock / openai
- [ ] `mock_extractor.go`, `mock_embedder.go`, `mock_explainer.go`, `mock_icebreaker.go`
- [ ] `openai_extractor.go`, `openai_embedder.go`, `openai_explainer.go`, `openai_icebreaker.go`
- [ ] `//go:embed` для prompts/*
- [ ] `cmd/seed` + `seed/fictional_profiles.json` (80–100 анкет)
- [ ] `POST .../icebreaker` в api
- [ ] bot: MatchList → Icebreaker FSM
