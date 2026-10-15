package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

// clearDeadlineReminder removes the pending deadline reminder of a task.
func clearDeadlineReminder(ctx context.Context, q querier, taskID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM reminders
		WHERE task_id = ? AND kind = ? AND sent_at IS NULL`, taskID, domain.ReminderDeadline)
	return err
}

// scheduleDeadline adds a deadline reminder if one is needed.
func scheduleDeadline(ctx context.Context, q querier, taskID int64, due time.Time, allDay bool, before int, now time.Time) error {
	at, ok := domain.DeadlineReminderAt(due, allDay, before, now)
	if !ok {
		return nil
	}
	_, err := q.ExecContext(ctx, `INSERT INTO reminders (task_id, remind_at, kind) VALUES (?, ?, ?)`,
		taskID, at.Unix(), domain.ReminderDeadline)
	if err != nil {
		return fmt.Errorf("schedule deadline reminder: %w", err)
	}
	return nil
}

// RescheduleDeadlines recomputes pending deadline reminders of all the user's
// open tasks with a future deadline, e.g. after the reminder setting changed.
func (s *Store) RescheduleDeadlines(ctx context.Context, userID int64, before int, now time.Time) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT t.id, t.due_at, t.due_all_day FROM tasks t
			JOIN projects p ON p.id = t.project_id
			WHERE p.user_id = ? AND t.done_at IS NULL AND t.due_at > ?`, userID, now.Unix())
		if err != nil {
			return err
		}
		type due struct {
			id     int64
			at     int64
			allDay bool
		}
		var list []due
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.id, &d.at, &d.allDay); err != nil {
				rows.Close()
				return err
			}
			list = append(list, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, d := range list {
			if err := clearDeadlineReminder(ctx, tx, d.id); err != nil {
				return err
			}
			if err := scheduleDeadline(ctx, tx, d.id, time.Unix(d.at, 0), d.allDay, before, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// AddReminder adds a custom reminder to a task owned by the user.
func (s *Store) AddReminder(ctx context.Context, userID, taskID int64, at time.Time) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := getTask(ctx, tx, userID, taskID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM reminders
			WHERE task_id = ? AND kind = ? AND sent_at IS NULL`, taskID, domain.ReminderCustom).Scan(&n); err != nil {
			return err
		}
		if n >= domain.MaxReminders {
			return domain.ErrTooManyReminders
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO reminders (task_id, remind_at, kind) VALUES (?, ?, ?)`,
			taskID, at.Unix(), domain.ReminderCustom)
		return err
	})
}

// ListReminders returns the pending reminders of a task in time order. The
// caller must have verified task ownership.
func (s *Store) ListReminders(ctx context.Context, taskID int64) ([]domain.Reminder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_id, remind_at, kind FROM reminders
		WHERE task_id = ? AND sent_at IS NULL ORDER BY remind_at, id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list reminders: %w", err)
	}
	defer rows.Close()
	var out []domain.Reminder
	for rows.Next() {
		var (
			r  domain.Reminder
			at int64
		)
		if err := rows.Scan(&r.ID, &r.TaskID, &at, &r.Kind); err != nil {
			return nil, err
		}
		r.RemindAt = time.Unix(at, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetReminder loads a reminder of a task owned by the user.
func (s *Store) GetReminder(ctx context.Context, userID, id int64) (domain.Reminder, error) {
	var (
		r      domain.Reminder
		at     int64
		sentAt sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx, `SELECT r.id, r.task_id, r.remind_at, r.sent_at, r.kind
		FROM reminders r JOIN tasks t ON t.id = r.task_id JOIN projects p ON p.id = t.project_id
		WHERE r.id = ? AND p.user_id = ?`, id, userID).Scan(&r.ID, &r.TaskID, &at, &sentAt, &r.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		return r, domain.ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("get reminder: %w", err)
	}
	r.RemindAt = time.Unix(at, 0)
	r.SentAt = unixPtr(sentAt)
	return r, nil
}

// DeleteReminder deletes a pending custom reminder and returns its task ID.
func (s *Store) DeleteReminder(ctx context.Context, userID, id int64) (int64, error) {
	r, err := s.GetReminder(ctx, userID, id)
	if err != nil {
		return 0, err
	}
	if r.Kind != domain.ReminderCustom {
		return 0, domain.ErrNotFound
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM reminders WHERE id = ?`, id)
	return r.TaskID, err
}

// DueReminders returns unsent reminders due at now, oldest first.
func (s *Store) DueReminders(ctx context.Context, now time.Time, limit int) ([]domain.ReminderJob, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.task_id, r.remind_at, r.kind, `+taskCols+`,
			p.name, p.archived, c.is_done, u.tg_id, u.lang, u.timezone
		FROM reminders r
		JOIN tasks t ON t.id = r.task_id
		JOIN columns c ON c.id = t.column_id
		JOIN projects p ON p.id = t.project_id
		JOIN users u ON u.tg_id = p.user_id
		WHERE r.sent_at IS NULL AND r.remind_at <= ?
		ORDER BY r.remind_at, r.id LIMIT ?`, now.Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("due reminders: %w", err)
	}
	defer rows.Close()

	var out []domain.ReminderJob
	for rows.Next() {
		var (
			j                 domain.ReminderJob
			at                int64
			archived, colDone bool
			ids               [2]int64
		)
		t, err := scanTaskAfter(rows, []any{&ids[0], &ids[1], &at, &j.Kind},
			&j.ProjectName, &archived, &colDone, &j.User.TgID, &j.User.Lang, &j.User.Timezone)
		if err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		j.ID, j.TaskID, j.RemindAt = ids[0], ids[1], time.Unix(at, 0)
		j.Task = t
		j.Skip = archived || colDone || t.DoneAt != nil
		out = append(out, j)
	}
	return out, rows.Err()
}

// MarkReminderSent records that a reminder was delivered (or skipped).
func (s *Store) MarkReminderSent(ctx context.Context, id int64, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reminders SET sent_at = ? WHERE id = ?`, now.Unix(), id)
	return err
}

// DigestUsers returns users with the morning digest enabled.
func (s *Store) DigestUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tg_id FROM users WHERE digest_time IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("digest users: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]domain.User, 0, len(ids))
	for _, id := range ids {
		u, err := s.GetUser(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// DigestFor returns the user's open tasks that are overdue or due later today.
// Tasks in archived projects are ignored.
func (s *Store) DigestFor(ctx context.Context, userID int64, now time.Time, loc *time.Location) (domain.Digest, error) {
	endOfDay := domain.EndOfDay(now, loc)
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskCols+`, p.name
		FROM tasks t
		JOIN columns c ON c.id = t.column_id
		JOIN projects p ON p.id = t.project_id
		WHERE p.user_id = ? AND p.archived = 0 AND c.is_done = 0 AND t.due_at IS NOT NULL AND t.due_at <= ?
		ORDER BY t.due_at, t.id`, userID, endOfDay.Unix())
	if err != nil {
		return domain.Digest{}, fmt.Errorf("digest: %w", err)
	}
	defer rows.Close()

	var d domain.Digest
	for rows.Next() {
		var item domain.DigestItem
		t, err := scanTask(rows, &item.ProjectName)
		if err != nil {
			return d, err
		}
		item.Task = t
		if t.DueAt.Before(now) {
			d.Overdue = append(d.Overdue, item)
		} else {
			d.Today = append(d.Today, item)
		}
	}
	return d, rows.Err()
}

// MarkDigestSent records the local date of the last sent digest.
func (s *Store) MarkDigestSent(ctx context.Context, userID int64, date string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET digest_sent_on = ? WHERE tg_id = ?`, date, userID)
	return err
}
