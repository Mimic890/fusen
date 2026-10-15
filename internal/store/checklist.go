package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

// ListChecklist returns checklist items of a task in order. The caller must
// have verified task ownership.
func (s *Store) ListChecklist(ctx context.Context, taskID int64) ([]domain.ChecklistItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_id, text, done, position
		FROM checklist_items WHERE task_id = ? ORDER BY position, id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list checklist: %w", err)
	}
	defer rows.Close()
	var out []domain.ChecklistItem
	for rows.Next() {
		var it domain.ChecklistItem
		if err := rows.Scan(&it.ID, &it.TaskID, &it.Text, &it.Done, &it.Position); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// AddChecklistItems appends items to a task owned by the user.
func (s *Store) AddChecklistItems(ctx context.Context, userID, taskID int64, texts []string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := getTask(ctx, tx, userID, taskID); err != nil {
			return err
		}
		var count, pos int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(position), -1) + 1
			FROM checklist_items WHERE task_id = ?`, taskID).Scan(&count, &pos); err != nil {
			return err
		}
		if count+len(texts) > domain.MaxChecklistItems {
			return domain.ErrTooManyItems
		}
		for i, text := range texts {
			if _, err := tx.ExecContext(ctx, `INSERT INTO checklist_items (task_id, text, position)
				VALUES (?, ?, ?)`, taskID, text, pos+i); err != nil {
				return fmt.Errorf("insert checklist item: %w", err)
			}
		}
		return touchTask(ctx, tx, taskID)
	})
}

// getItem loads a checklist item of a task owned by the user.
func getItem(ctx context.Context, q querier, userID, id int64) (domain.ChecklistItem, error) {
	var it domain.ChecklistItem
	err := q.QueryRowContext(ctx, `SELECT i.id, i.task_id, i.text, i.done, i.position
		FROM checklist_items i
		JOIN tasks t ON t.id = i.task_id
		JOIN projects p ON p.id = t.project_id
		WHERE i.id = ? AND p.user_id = ?`, id, userID).
		Scan(&it.ID, &it.TaskID, &it.Text, &it.Done, &it.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return it, domain.ErrNotFound
	}
	if err != nil {
		return it, fmt.Errorf("get checklist item: %w", err)
	}
	return it, nil
}

// ToggleChecklistItem flips an item's done flag and returns its task ID.
func (s *Store) ToggleChecklistItem(ctx context.Context, userID, id int64) (int64, error) {
	var taskID int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		it, err := getItem(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		taskID = it.TaskID
		if _, err := tx.ExecContext(ctx, `UPDATE checklist_items SET done = ? WHERE id = ?`,
			boolInt(!it.Done), id); err != nil {
			return err
		}
		return touchTask(ctx, tx, taskID)
	})
	return taskID, err
}

// DeleteChecklistItem deletes an item and returns its task ID.
func (s *Store) DeleteChecklistItem(ctx context.Context, userID, id int64) (int64, error) {
	var taskID int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		it, err := getItem(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		taskID = it.TaskID
		if _, err := tx.ExecContext(ctx, `DELETE FROM checklist_items WHERE id = ?`, id); err != nil {
			return err
		}
		return touchTask(ctx, tx, taskID)
	})
	return taskID, err
}

func touchTask(ctx context.Context, q querier, taskID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = ?`, time.Now().Unix(), taskID)
	return err
}
