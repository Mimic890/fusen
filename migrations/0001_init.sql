-- Initial schema. Timestamps are unix seconds (UTC).

CREATE TABLE users (
    tg_id              INTEGER PRIMARY KEY,
    lang               TEXT    NOT NULL DEFAULT 'en' CHECK (lang IN ('en', 'ru')),
    view_mode          TEXT    NOT NULL DEFAULT 'buttons' CHECK (view_mode IN ('buttons', 'table', 'article')),
    timezone           TEXT    NOT NULL DEFAULT 'UTC',
    digest_time        TEXT,                -- 'HH:MM' in user's timezone, NULL = disabled
    digest_sent_on     TEXT,                -- 'YYYY-MM-DD' of the last sent digest
    remind_before_min  INTEGER NOT NULL DEFAULT 60,
    live_message_id    INTEGER,
    state              TEXT    NOT NULL DEFAULT '',
    current_project_id INTEGER REFERENCES projects (id) ON DELETE SET NULL,
    created_at         INTEGER NOT NULL
);

CREATE TABLE projects (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (tg_id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    position   INTEGER NOT NULL,
    archived   INTEGER NOT NULL DEFAULT 0,
    is_inbox   INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE INDEX projects_user_idx ON projects (user_id, archived, position);
CREATE UNIQUE INDEX projects_one_inbox_idx ON projects (user_id) WHERE is_inbox = 1;

CREATE TABLE columns (
    id         INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    position   INTEGER NOT NULL,
    is_done    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX columns_project_idx ON columns (project_id, position);

CREATE TABLE tasks (
    id          INTEGER PRIMARY KEY,
    project_id  INTEGER NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    column_id   INTEGER NOT NULL REFERENCES columns (id),
    title       TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    priority    INTEGER NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 3), -- none/low/med/high
    due_at      INTEGER,
    position    INTEGER NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    done_at     INTEGER
);

CREATE INDEX tasks_column_idx ON tasks (column_id, position);
CREATE INDEX tasks_project_idx ON tasks (project_id);
CREATE INDEX tasks_due_idx ON tasks (due_at) WHERE due_at IS NOT NULL;

CREATE TABLE checklist_items (
    id       INTEGER PRIMARY KEY,
    task_id  INTEGER NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    text     TEXT    NOT NULL,
    done     INTEGER NOT NULL DEFAULT 0,
    position INTEGER NOT NULL
);

CREATE INDEX checklist_items_task_idx ON checklist_items (task_id, position);

CREATE TABLE tags (
    id      INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users (tg_id) ON DELETE CASCADE,
    name    TEXT    NOT NULL,
    UNIQUE (user_id, name)
);

CREATE TABLE task_tags (
    task_id INTEGER NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, tag_id)
);

CREATE INDEX task_tags_tag_idx ON task_tags (tag_id);

CREATE TABLE reminders (
    id        INTEGER PRIMARY KEY,
    task_id   INTEGER NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    remind_at INTEGER NOT NULL,
    sent_at   INTEGER,
    kind      TEXT    NOT NULL CHECK (kind IN ('deadline', 'custom'))
);

CREATE INDEX reminders_task_idx ON reminders (task_id);
CREATE INDEX reminders_pending_idx ON reminders (remind_at) WHERE sent_at IS NULL;
