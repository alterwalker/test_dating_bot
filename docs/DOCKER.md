# Docker — local Postgres only (Go runs on host)

## Start

```bash
docker compose up -d
docker compose ps
```

Wait until `healthy`:

```bash
docker compose exec postgres pg_isready -U dating -d dating_bot
```

## Connection (host)

```
postgres://dating:dating@localhost:5432/dating_bot?sslmode=disable
```

Same as `DATABASE_URL` in `.env.example`.

If port 5432 is busy on the host:

```bash
POSTGRES_PORT=5433 docker compose up -d
# DATABASE_URL=postgres://dating:dating@localhost:5433/dating_bot?sslmode=disable
```

## Migrations

SQL from `migrations/` is applied **automatically on first** `docker compose up`
(when volume `postgres_data` is empty), via `/docker-entrypoint-initdb.d`.

To re-run migrations from scratch:

```bash
docker compose down -v   # ⚠️ deletes data
docker compose up -d
```

Later schema changes (after first boot) — apply manually or via future `cmd/migrate`:

```bash
docker compose exec -T postgres psql -U dating -d dating_bot < migrations/003_....sql
```

## Verify pgvector

```bash
docker compose exec postgres psql -U dating -d dating_bot -c "\dx vector"
docker compose exec postgres psql -U dating -d dating_bot -c "\d profiles"
```

Expected: extension `vector`, column `embedding vector(1536)`, index `idx_profiles_embedding_hnsw`.

## Stop

```bash
docker compose down        # keep data
docker compose down -v     # remove volume
```
