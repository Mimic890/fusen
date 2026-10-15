# fusen

A personal kanban board in Telegram: [@fusenboard_bot](https://t.me/fusenboard_bot).

> Work in progress. Full documentation will follow.

## Run with Docker

```fish
cp .env.example .env    # then set BOT_TOKEN
mkdir -p data
docker compose up -d --build
```

The SQLite database is stored in `./data/fusen.db` (bind mount). The container
runs as UID/GID 1000 by default; override with `FUSEN_UID` / `FUSEN_GID` if
`./data` is owned by another user.

## Run locally

```fish
set -x BOT_TOKEN <token>
go run ./cmd/fusen
```

## Configuration

| Variable           | Default         | Description                                         |
|--------------------|-----------------|-----------------------------------------------------|
| `BOT_TOKEN`        | —               | Bot token from @BotFather (required)                |
| `DB_PATH`          | `data/fusen.db` | SQLite database path                                |
| `DEFAULT_TZ`       | `UTC`           | Timezone assigned to new users                      |
| `LOG_LEVEL`        | `info`          | `debug`, `info`, `warn`, `error`                    |
| `ALLOWED_USER_IDS` | empty           | Comma-separated user IDs; empty = open to everyone  |

## Tests

```fish
go test ./...
```

A live smoke test renders every board view and sends it to a chat through the
real Bot API, then deletes the messages:

```fish
env BOT_TOKEN=<token> FUSEN_LIVE_CHAT_ID=<your chat id> go test -tags live -run Live ./internal/bot/
```

## License

AGPL-3.0
