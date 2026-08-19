# cmd/worker

Фоновый worker: job `enrich_profile` (LLM extract + embedding).

**Реализовано:** poll таблицы `jobs` (`FOR UPDATE SKIP LOCKED`), retry с backoff, healthcheck `:8081/health`.

Промпты: [prompts/](../prompts/) · AI: [docs/AI.md](../docs/AI.md)

Запуск: `go run ./cmd/worker`
