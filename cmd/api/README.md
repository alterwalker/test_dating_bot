# cmd/api

HTTP API: пользователи, профили, jobs, матчинг, explain/icebreaker, admin-статистика.

**Реализовано:** REST `/v1/*`, Postgres + pgvector ANN, постановка `enrich_profile`, token usage tracking.

Спецификация: [docs/API.md](../docs/API.md)

Запуск: `go run ./cmd/api` (слушает `HTTP_ADDR`, default `:8080`).
