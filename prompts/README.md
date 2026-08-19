# Prompts

| Файл | Назначение |
|------|------------|
| [extractor.system.md](extractor.system.md) | System prompt для извлечения профиля |
| [extractor.user.template.md](extractor.user.template.md) | User message + пример |
| [explainer.system.md](explainer.system.md) | System prompt для объяснения match |
| [explainer.user.template.md](explainer.user.template.md) | User message + пример |
| [normalizer.system.md](normalizer.system.md) | Нормализация тегов (опционально) |
| [profile.schema.json](profile.schema.json) | JSON Schema для structured output |

## Когда используются

| Режим | Extractor / Explainer prompts |
|-------|------------------------------|
| `AI_MOCK=true` (default) | **не вызываются** — mock-логика в Go |
| `AI_MOCK=false` | загружаются через `//go:embed` |

## OpenAI structured output

При `AI_MOCK=false` — chat completions с `response_format`:

```json
{
  "type": "json_schema",
  "json_schema": {
    "name": "enriched_profile",
    "strict": true,
    "schema": { "... содержимое profile.schema.json ..." }
  }
}
```

## Загрузка в Go

```go
//go:embed prompts/*.md prompts/*.json
var PromptFS embed.FS
```

Или чтение с диска относительно `PROMPTS_DIR` для hot-reload в dev.

См. также [docs/AI.md](../docs/AI.md).
