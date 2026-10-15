package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

const taskCols = `t.id, t.project_id, t.column_id, t.title, t.description, t.priority,
	t.due_at, t.position, t.created_at, t.updated_at, t.done_at`

func scanTask(r rowScanner, extra ...any) (domain.Task, error) {
	var (
		t                    domain.Task
		dueAt, doneAt        sql.NullInt64
		createdAt, updatedAt int64
	)
	dest := append([]any{&t.ID, &t.ProjectID, &t.ColumnID, &t.Title, &t.Description, &t.Priority,
		&dueAt, &t.Position, &createdAt, &updatedAt, &doneAt}, extra...)
	if err := r.Scan(dest...); err != nil {
		return t, err
	}
	t.DueAt = unixPtr(dueAt)
	t.DoneAt = unixPtr(doneAt)
	t.CreatedAt = time.Unix(createdAt, 0)
	t.UpdatedAt = time.Unix(updatedAt, 0)
	return t, nil
}

func unixPtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.Unix(v.Int64, 0)
	return &t
}

// GetTask loads a task of a project owned by the user, with its tags.
func (s *Store) GetTask(ctx context.Context, userID, id int64) (domain.Task, error) {
	t, err := getTask(ctx, s.db, userID, id)
	if err != nil {
		return t, err
	}
	t.Tags, err = s.taskTags(ctx, t.ID)
	return t, err
}

func getTask(ctx context.Context, q querier, userID, id int64) (domain.Task, error) {
	t, err := scanTask(q.QueryRowContext(ctx, `SELECT `+taskCols+`
		FROM tasks t JOIN projects p ON p.id = t.project_id
		WHERE t.id = ? AND p.user_id = ?`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return t, domain.ErrNotFound
	}
	if err != nil {
		return t, fmt.Errorf("get task: %w", err)
	}
	return t, nil
}

func (s *Store) taskTags(ctx context.Context, taskID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.name FROM tags g
		JOIN task_tags tt ON tt.tag_id = g.id WHERE tt.task_id = ? ORDER BY g.name`, taskID)
	if err != nil {
		return nil, fmt.Errorf("task tags: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// ListColumnTasks returns a page of tasks in a column and the column's total task count.
// The caller must have verified column ownership.
func (s *Store) ListColumnTasks(ctx context.Context, columnID int64, offset, limit int) ([]domain.TaskSummary, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE column_id = ?`, columnID).
		Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT `+taskCols+`,
			(SELECT COUNT(*) FROM checklist_items i WHERE i.task_id = t.id AND i.done = 1),
			(SELECT COUNT(*) FROM checklist_items i WHERE i.task_id = t.id)
		FROM tasks t WHERE t.column_id = ? ORDER BY t.position, t.id LIMIT ? OFFSET ?`,
		columnID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var out []domain.TaskSummary
	for rows.Next() {
		var ts domain.TaskSummary
		t, err := scanTask(rows, &ts.ChecklistDone, &ts.ChecklistTotal)
		if err != nil {
			return nil, 0, fmt.Errorf("scan task: %w", err)
		}
		ts.Task = t
		out = append(out, ts)
	}
	return out, total, rows.Err()
}

// CreateTasks appends tasks with the given titles to the end of a column owned by the user.
func (s *Store) CreateTasks(ctx context.Context, userID, columnID int64, titles []string) ([]domain.Task, error) {
	var out []domain.Task
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		c, err := getColumn(ctx, tx, userID, columnID)
		if err != nil {
			return err
		}
		var pos int
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(position), -1) + 1 FROM tasks WHERE column_id = ?`, c.ID).Scan(&pos); err != nil {
			return fmt.Errorf("next task position: %w", err)
		}

		now := time.Now()
		var doneAt any
		if c.IsDone {
			doneAt = now.Unix()
		}
		for i, title := range titles {
			res, err := tx.ExecContext(ctx, `INSERT INTO tasks
				(project_id, column_id, title, position, created_at, updated_at, done_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`, c.ProjectID, c.ID, title, pos+i, now.Unix(), now.Unix(), doneAt)
			if err != nil {
				return fmt.Errorf("insert task: %w", err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return err
			}
			out = append(out, domain.Task{ID: id, ProjectID: c.ProjectID, ColumnID: c.ID, Title: title, Position: pos + i})
		}
		return nil
	})
	return out, err
}

// updateTask runs an UPDATE on a task owned by the user and bumps updated_at.
func (s *Store) updateTask(ctx context.Context, userID, id int64, set string, args ...any) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := getTask(ctx, tx, userID, id); err != nil {
			return err
		}
		args = append(args, time.Now().Unix(), id)
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET `+set+`, updated_at = ? WHERE id = ?`, args...); err != nil {
			return fmt.Errorf("update task: %w", err)
		}
		return nil
	})
}

// SetTaskTitle renames a task.
func (s *Store) SetTaskTitle(ctx context.Context, userID, id int64, title string) error {
	return s.updateTask(ctx, userID, id, `title = ?`, title)
}

// SetTaskDescription sets or clears (empty string) a task description.
func (s *Store) SetTaskDescription(ctx context.Context, userID, id int64, desc string) error {
	return s.updateTask(ctx, userID, id, `description = ?`, desc)
}

// SetTaskPriority sets a task priority.
func (s *Store) SetTaskPriority(ctx context.Context, userID, id int64, p domain.Priority) error {
	return s.updateTask(ctx, userID, id, `priority = ?`, int(p))
}

// SetTaskTags replaces the tags of a task.
func (s *Store) SetTaskTags(ctx context.Context, userID, id int64, tags []string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := getTask(ctx, tx, userID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM task_tags WHERE task_id = ?`, id); err != nil {
			return err
		}
		for _, tag := range tags {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO tags (user_id, name) VALUES (?, ?) ON CONFLICT (user_id, name) DO NOTHING`,
				userID, tag); err != nil {
				return fmt.Errorf("insert tag: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO task_tags (task_id, tag_id)
				SELECT ?, id FROM tags WHERE user_id = ? AND name = ?`, id, userID, tag); err != nil {
				return fmt.Errorf("link tag: %w", err)
			}
		}
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = ?`, time.Now().Unix(), id)
		return err
	})
}

// MoveTask moves a task to the end of the neighbouring column. At the board
// edge it returns ErrNotFound. done_at follows the target column's kind.
func (s *Store) MoveTask(ctx context.Context, userID, id int64, dir domain.Direction) (domain.Column, error) {
	var target domain.Column
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		t, err := getTask(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		cur, err := getColumn(ctx, tx, userID, t.ColumnID)
		if err != nil {
			return err
		}
		if target, err = neighbourColumn(ctx, tx, cur, dir); err != nil {
			return err
		}
		var pos int
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(position), -1) + 1 FROM tasks WHERE column_id = ?`, target.ID).Scan(&pos); err != nil {
			return err
		}
		now := time.Now().Unix()
		var doneAt any
		if target.IsDone {
			doneAt = now
			if t.DoneAt != nil {
				doneAt = t.DoneAt.Unix()
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE tasks SET column_id = ?, position = ?, done_at = ?, updated_at = ?
			WHERE id = ?`, target.ID, pos, doneAt, now, id)
		return err
	})
	return target, err
}

// DeleteTask deletes a task with its checklist, tags links and reminders.
func (s *Store) DeleteTask(ctx context.Context, userID, id int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := getTask(ctx, tx, userID, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
		return err
	})
}

// ListProjectTasks returns all tasks of a project in board order (column, then
// position) with tags and checklist progress. The caller must have verified
// project ownership.
func (s *Store) ListProjectTasks(ctx context.Context, projectID int64) ([]domain.TaskSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskCols+`,
			(SELECT COUNT(*) FROM checklist_items i WHERE i.task_id = t.id AND i.done = 1),
			(SELECT COUNT(*) FROM checklist_items i WHERE i.task_id = t.id)
		FROM tasks t JOIN columns c ON c.id = t.column_id
		WHERE t.project_id = ? ORDER BY c.position, c.id, t.position, t.id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project tasks: %w", err)
	}
	defer rows.Close()

	var out []domain.TaskSummary
	byID := map[int64]int{}
	for rows.Next() {
		var ts domain.TaskSummary
		t, err := scanTask(rows, &ts.ChecklistDone, &ts.ChecklistTotal)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		ts.Task = t
		byID[t.ID] = len(out)
		out = append(out, ts)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tagRows, err := s.db.QueryContext(ctx, `SELECT tt.task_id, g.name FROM task_tags tt
		JOIN tags g ON g.id = tt.tag_id JOIN tasks t ON t.id = tt.task_id
		WHERE t.project_id = ? ORDER BY g.name`, projectID)
	if err != nil {
		return nil, fmt.Errorf("project tags: %w", err)
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var (
			id   int64
			name string
		)
		if err := tagRows.Scan(&id, &name); err != nil {
			return nil, err
		}
		if i, ok := byID[id]; ok {
			out[i].Tags = append(out[i].Tags, name)
		}
	}
	return out, tagRows.Err()
}
