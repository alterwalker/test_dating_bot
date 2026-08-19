# cmd/bot

Telegram-бот: long polling, FSM анкеты, inline-клавиатуры, HTTP-клиент к `api`.

**Реализовано:** главное меню (профиль, matches, admin-статистика), создание и редактирование анкеты по полям, matches с icebreaker, скрытие кандидата, удаление своего профиля.

Спецификация: [docs/FSM.md](../docs/FSM.md)

Запуск: `go run ./cmd/bot` (нужен `BOT_TOKEN` в `.env`).
