# Деплой на Ubuntu (rsync + Docker Compose)

Отдельный **Postgres + pgvector в Docker** (порт на хосте `5433` по умолчанию) — **не конфликтует** с системным PostgreSQL на `:5432`.

Сервисы: **postgres**, **api**, **worker**, **bot** — один образ Go, разные `command`.

## Архитектура

```
                    ┌─────────────────────────────────┐
                    │  docker compose (prod)          │
                    │                                 │
  Telegram ◄─────── │  bot ──HTTP──► api :8080        │
                    │                  │   worker     │
                    │                  ▼              │
                    │            postgres :5432       │
                    │            (host :5433)         │
                    └─────────────────────────────────┘
```

---

## 1. Ubuntu: Docker

```bash
sudo apt update
sudo apt install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
  https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo $VERSION_CODENAME) stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo usermod -aG docker $USER
# перелогиниться или: newgrp docker
```

---

## 2. Rsync проекта

С локальной машины:

```bash
rsync -avz \
  --exclude '.git' \
  --exclude '.env' \
  --exclude '.idea' \
  --exclude 'seed/fictional_profiles.json' \
  --exclude '/bot' \
  --exclude '/enrich_seed' \
  ./ user@YOUR_SERVER:/opt/dating_bot/
```

На сервере:

```bash
cd /opt/dating_bot
cp .env.prod.example .env
nano .env
```

**Обязательно поменять:**

| Переменная | Значение |
|------------|----------|
| `BOT_TOKEN` | от @BotFather |
| `POSTGRES_PASSWORD` | сильный пароль |
| `DATABASE_URL` | тот же пароль: `postgres://dating:PASSWORD@postgres:5432/dating_bot?sslmode=disable` |
| `OPENAI_API_KEY` | если `AI_MOCK=false` |
| `HTTP_PORT` | внешний порт api, напр. `18080` |
| `POSTGRES_PORT` | порт Postgres на хосте, напр. `5433` (не 5432, если там системный PG) |

`API_BASE_URL=http://api:8080/v1` — **не менять** (bot → api внутри compose).

---

## 3. Миграции

При **первом** запуске postgres SQL из `migrations/` применяется автоматически (`docker-entrypoint-initdb.d`).

Проверка:

```bash
docker compose -f docker-compose.prod.yml up -d postgres
docker compose -f docker-compose.prod.yml exec postgres \
  psql -U dating -d dating_bot -c "\dx vector"
```

Новые миграции после первого старта — вручную:

```bash
docker compose -f docker-compose.prod.yml exec -T postgres \
  psql -U dating -d dating_bot < migrations/00N_....sql
```

---

## 4. Seed demo-анкет (один раз)

Go на сервере **не нужен** — утилиты в образе:

```bash
docker compose -f docker-compose.prod.yml build

# сгенерировать JSON (~7 MB mock)
docker compose -f docker-compose.prod.yml run --rm --no-deps \
  -v "$(pwd)/seed:/seed" \
  --entrypoint generate_seed \
  api -out /seed/fictional_profiles.json

# загрузить в Postgres (postgres должен быть запущен)
# .env подхватывается из env_file сервиса api в compose — отдельный --env-file не нужен
docker compose -f docker-compose.prod.yml up -d postgres
docker compose -f docker-compose.prod.yml run --rm \
  -v "$(pwd)/seed:/seed" \
  --entrypoint seed \
  api -file /seed/fictional_profiles.json
```

OpenAI-enrich — локально или на сервере с ключом: [cmd/enrich_seed/README.md](../cmd/enrich_seed/README.md).

---

## 5. Запуск всего стека

```bash
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f bot
```

Проверки:

```bash
curl -s "http://127.0.0.1:18080/health"
docker compose -f docker-compose.prod.yml exec postgres pg_isready -U dating -d dating_bot
```

---

## 6. Обновление (rsync + rebuild)

```bash
cd /opt/dating_bot
docker compose -f docker-compose.prod.yml up -d --build
```

Данные Postgres сохраняются в volume `postgres_data`.

Сброс БД (осторожно):

```bash
docker compose -f docker-compose.prod.yml down -v
docker compose -f docker-compose.prod.yml up -d
# повторить seed
```

---

## 7. Firewall (ufw)

```bash
# api — только если нужен доступ извне
sudo ufw allow 18080/tcp

# Postgres НЕ открывать в интернет (доступ только localhost:5433 при необходимости)
# bot — исходящий HTTPS, входящие порты не нужны
sudo ufw enable
```

---

## Порты по умолчанию

| Сервис | На хосте Ubuntu | Внутри compose |
|--------|-----------------|----------------|
| api | `18080` → 8080 | `api:8080` |
| postgres | `5433` → 5432 | `postgres:5432` |
| worker / bot | — | только internal |

Системный PostgreSQL на `:5432` остаётся нетронутым.
