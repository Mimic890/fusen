# fusen

Personal kanban boards in Telegram: [@fusenboard_bot](https://t.me/fusenboard_bot).

fusen keeps your tasks in projects with kanban columns and lets you work with
them without leaving the chat: one "live" message is edited as you navigate,
your own messages are removed once handled, and reminders arrive as separate
notifications.

- Projects with an **Inbox**, customizable columns and an archive.
- Three board views: **buttons**, a rich **table** and a rich **article**.
- Task cards with deadline, priority, tags, description and a checklist.
- **Quick input**: send `buy cable #homelab !tomorrow 18:00 !!` and it becomes a task.
- Deadline reminders, custom reminders and a **morning digest**, in your timezone.
- English and Russian.

Solo use for now; collaboration is out of scope of the MVP.

## Using the bot

Send `/start` to open your projects. `/settings` opens the settings.

### Boards

Each project is a board. Pick how boards look in settings or with the switcher
under every board:

| View    | What you see                                                                                    |
|---------|-------------------------------------------------------------------------------------------------|
| Buttons | One column at a time with a button per task, paging and `‹ previous / next ›` column buttons     |
| Table   | All columns side by side; each cell is a task button                                             |
| Article | A section per column with task lists, deadlines and tags; done columns are collapsed             |

The header `Project › Column (3 / 10)` shows the tasks in the column and in the
whole project. Long columns in the table and article views end with a
"+N more" button that opens the column in the buttons view.

Tap a task to open its card: move it to the neighbouring column with `←` / `→`,
edit the title, description, deadline, priority, tags or project, manage the
checklist, set reminders or delete it.

Under `⚙ Project` you can rename, archive or delete a project and configure its
columns: add, rename, reorder, mark as a "done" column (tasks there count as
completed) or delete empty ones. The Inbox can only be renamed.

### Quick input

Any message that is not a command becomes a task in the first column of the
project you have open, or in the Inbox. Several lines create several tasks.

```
buy cable #homelab !tomorrow 18:00 !!
```

| Token                                    | Meaning                                                                 |
|------------------------------------------|-------------------------------------------------------------------------|
| `#tag`                                   | adds a tag                                                              |
| `!` / `!!` / `!!!`                       | low / medium / high priority                                            |
| `!today`, `!tomorrow`, `!сегодня`, `!завтра`, `!послезавтра` | deadline on that day                                |
| `!mon` … `!sun`, `!пн` … `!вс`, `!friday`, `!пятница`        | the next such weekday after today                   |
| `!25.12`, `!25.12.2026`, `!2026-12-25`   | a date; a day and month already past this year means next year          |
| `!<date> 18:00`                          | a date with a time                                                      |
| `!18:00`                                 | today at that time, or tomorrow if it has passed                        |

Russian and English words work regardless of the interface language. A date
without a time is an all-day deadline: it becomes overdue after the day ends
and is covered by the morning digest instead of a reminder. Anything that does
not parse stays in the title. The same syntax works in the `+ Task` prompt.

### Reminders and digest

- **Deadline reminders** arrive a set time before the deadline (1 hour by
  default; change it in settings). If the deadline is closer than that, the
  reminder comes at the deadline itself.
- **Custom reminders** are set from the task card (`⏰ Remind`): in 1 hour,
  in 3 hours, tomorrow at 9:00, or any time you type (`завтра 9:00`, `18:30`).
- A reminder has **✅ Done**, **+1 h**, **Tomorrow** and **Open** buttons.
  Done moves the task to the done column; +1 h reminds again in an hour;
  Tomorrow moves a deadline to the next day (or reminds again tomorrow for a
  custom reminder).
- The **morning digest** lists overdue tasks and tasks due today, once a day
  at your digest time (09:00 by default). It is skipped when there is nothing
  to report.

### Settings

Language (English / Русский), board view, timezone (a preset, an IANA name
such as `Asia/Tokyo`, or an offset such as `+3`), digest time and the deadline
reminder.

## Running with Docker

Requirements: Docker with Compose v2.

```fish
cp .env.example .env    # then set BOT_TOKEN
mkdir -p data
docker compose up -d --build
docker compose logs -f
```

The SQLite database lives in `./data/fusen.db` (a bind mount; no named volumes).
The container runs as UID/GID 1000 with a read-only root filesystem and no
capabilities; it writes only to `/data`.

Common tasks:

```fish
# update to the latest code
git pull
docker compose up -d --build

# build with a version (shown by `fusen -version` and in the startup log)
env FUSEN_VERSION=(git describe --tags --always) docker compose up -d --build

# stop
docker compose down
```

### Backup

Stop the bot so that SQLite writes everything into the main file, then copy it:

```fish
docker compose stop
mkdir -p backups
cp data/fusen.db backups/fusen-(date +%F).db
docker compose start
```

To restore, stop the bot, put the backup back as `data/fusen.db` (remove any
`data/fusen.db-wal` and `data/fusen.db-shm`) and start it again.

## Running on a server

CI publishes the image to GitHub Container Registry on every push to `main`
(`edge`) and for every version tag (`v0.1.0` → `0.1.0`, `0.1` and `latest`).
Images are built for `linux/amd64` and `linux/arm64`. The server only needs
Docker; no source code or Go toolchain.

Create a directory with `docker-compose.yml`, `.env` and `data/`:

```yaml
services:
  fusen:
    image: ghcr.io/mimic890/fusen:latest
    restart: unless-stopped
    env_file: .env
    environment:
      DB_PATH: /data/fusen.db
    # Run as the owner of ./data so the bind mount is writable.
    user: "${FUSEN_UID:-1000}:${FUSEN_GID:-1000}"
    volumes:
      - ./data:/data
    read_only: true
    tmpfs:
      - /tmp
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    stop_grace_period: 30s
    logging:
      driver: json-file
      options:
        max-size: 10m
        max-file: "3"
```

```fish
mkdir -p fusen/data
cd fusen
# put docker-compose.yml here, then create .env with BOT_TOKEN (see .env.example)
docker compose pull
docker compose up -d
```

Update to the latest image:

```fish
docker compose pull
docker compose up -d
```

To pin a version, use a tag such as `ghcr.io/mimic890/fusen:0.1.0` instead of
`latest`; `edge` follows `main`.

## Configuration

Environment variables (set them in `.env`):

| Variable           | Default         | Description                                                    |
|--------------------|-----------------|----------------------------------------------------------------|
| `BOT_TOKEN`        | —               | Bot token from @BotFather (required)                           |
| `DB_PATH`          | `data/fusen.db` | SQLite database path; Compose sets `/data/fusen.db`            |
| `DEFAULT_TZ`       | `UTC`           | IANA timezone assigned to new users                            |
| `LOG_LEVEL`        | `info`          | `debug`, `info`, `warn` or `error`                             |
| `ALLOWED_USER_IDS` | empty           | Comma-separated Telegram user IDs; empty means open to everyone; others get "access denied" |

Compose also reads these from the shell or `.env`:

| Variable                  | Default | Description                                  |
|---------------------------|---------|----------------------------------------------|
| `FUSEN_UID` / `FUSEN_GID` | `1000`  | User and group the container runs as         |
| `FUSEN_VERSION`           | `dev`   | Version baked into the image                 |

## Troubleshooting

### `./data` is not writable

If `./data` does not exist when the container starts, Docker creates it owned by
root, and the bot (running as UID 1000) cannot write there. It stops with:

```
store: database location is not writable: /data is not writable by uid 1000, gid 1000 ...
```

Fix the owner on the host and restart:

```fish
sudo chown -R 1000:1000 data
docker compose up -d
```

Or run the container as the current owner of `./data` (`id -u` / `id -g` print your own IDs):

```fish
env FUSEN_UID=(id -u) FUSEN_GID=(id -g) docker compose up -d
```

### `Conflict: terminated by other getUpdates request`

Two instances use the same token, e.g. a container and `go run`. Telegram
delivers updates to only one of them; stop the other.

### `Unauthorized` on start

The token is wrong. In `.env`, quotes around the value are fine for Compose,
but strip them if you copy the value into a shell variable.

## Development

Requirements: Go 1.27 (no CGO needed: SQLite is `modernc.org/sqlite`).

```fish
set -x BOT_TOKEN <token>
go run ./cmd/fusen
```

### Tests

```fish
go test ./...
go test -race ./...    # needs CGO and a C compiler
```

The tests run the bot against a fake Bot API server: every callback route is
rendered and checked against another user's objects, and flows such as quick
input, reminders and the live screen are covered. CI (GitHub Actions) runs
gofmt, `go vet`, the race tests and a Docker build.

A live smoke test renders every screen and sends it to a chat through the real
Bot API, edits it between plain and rich content, and deletes the messages.
It does not poll for updates, so it can run next to a deployed bot:

```fish
env BOT_TOKEN=<token> FUSEN_LIVE_CHAT_ID=<your chat id> go test -tags live -run Live ./internal/bot/
```

### Releases

Push a version tag; the release workflow builds and publishes the image:

```fish
git tag v0.1.0
git push origin v0.1.0
```

### Layout

```
cmd/fusen            entry point: config, store, bot, scheduler
internal/config      environment configuration
internal/domain      entities and business rules
internal/store       SQLite repositories and the migration runner
internal/parser      quick-input parser
internal/callback    compact callback_data codec (64-byte limit)
internal/views       screens: text, rich HTML and inline keyboards
internal/bot         update handling, live screen, Telegram calls
internal/scheduler   reminders and morning digest delivery
internal/i18n        translations loader
migrations/          embedded SQL migrations
locales/             embedded en/ru translations (JSON)
```

## License

[AGPL-3.0](LICENSE)
