-- A deadline without a time of day. due_at then holds the last second of that
-- day in the user's timezone, so overdue and "today" checks need no special case.
ALTER TABLE tasks ADD COLUMN due_all_day INTEGER NOT NULL DEFAULT 0;
