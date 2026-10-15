package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

const columnCols = `c.id, c.project_id, c.name, c.position, c.is_done`

func scanColumn(r rowScanner, extra ...any) (domain.Column, error) {
	var c domain.Column
	err := r.Scan(append([]any{&c.ID, &c.ProjectID, &c.Name, &c.Position, &c.IsDone}, extra...)...)
	return c, err
}

// ListColumns returns project columns in board order with task counts.
func (s *Store) ListColumns(ctx context.Context, projectID int64) ([]domain.ColumnSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columnCols+`,
			(SELECT COUNT(*) FROM tasks t WHERE t.column_id = c.id)
		FROM columns c WHERE c.project_id = ? ORDER BY c.position, c.id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}
	defer rows.Close()

	var out []domain.ColumnSummary
	for rows.Next() {
		var cs domain.ColumnSummary
		c, err := scanColumn(rows, &cs.Tasks)
		if err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		cs.Column = c
		out = append(out, cs)
	}
	return out, rows.Err()
}

// GetColumn loads a column of a project owned by the user.
func (s *Store) GetColumn(ctx context.Context, userID, id int64) (domain.Column, error) {
	return getColumn(ctx, s.db, userID, id)
}

func getColumn(ctx context.Context, q querier, userID, id int64) (domain.Column, error) {
	c, err := scanColumn(q.QueryRowContext(ctx, `SELECT `+columnCols+`
		FROM columns c JOIN projects p ON p.id = c.project_id
		WHERE c.id = ? AND p.user_id = ?`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return c, domain.ErrNotFound
	}
	if err != nil {
		return c, fmt.Errorf("get column: %w", err)
	}
	return c, nil
}

// CreateColumn appends a column to a project owned by the user.
func (s *Store) CreateColumn(ctx context.Context, userID, projectID int64, name string) (domain.Column, error) {
	c := domain.Column{ProjectID: projectID, Name: name}
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := getProject(ctx, tx, userID, projectID); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*), COALESCE(MAX(position), -1) + 1 FROM columns WHERE project_id = ?`, projectID).
			Scan(&count, &c.Position); err != nil {
			return fmt.Errorf("next column position: %w", err)
		}
		if count >= domain.MaxColumns {
			return domain.ErrTooManyColumns
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO columns (project_id, name, position) VALUES (?, ?, ?)`,
			projectID, name, c.Position)
		if err != nil {
			return fmt.Errorf("insert column: %w", err)
		}
		c.ID, err = res.LastInsertId()
		return err
	})
	return c, err
}

// RenameColumn renames a column owned by the user.
func (s *Store) RenameColumn(ctx context.Context, userID, id int64, name string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := getColumn(ctx, tx, userID, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE columns SET name = ? WHERE id = ?`, name, id)
		return err
	})
}

// MoveColumn swaps a column with its neighbour in the given direction.
// Moving past the edge is a no-op.
func (s *Store) MoveColumn(ctx context.Context, userID, id int64, dir domain.Direction) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		c, err := getColumn(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		n, err := neighbourColumn(ctx, tx, c, dir)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE columns SET position = ? WHERE id = ?`, n.Position, c.ID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE columns SET position = ? WHERE id = ?`, c.Position, n.ID)
		return err
	})
}

// neighbourColumn returns the column next to c in board order.
func neighbourColumn(ctx context.Context, q querier, c domain.Column, dir domain.Direction) (domain.Column, error) {
	query := `SELECT ` + columnCols + ` FROM columns c WHERE c.project_id = ?1
		AND (c.position > ?2 OR (c.position = ?2 AND c.id > ?3)) ORDER BY c.position, c.id LIMIT 1`
	if dir == domain.Left {
		query = `SELECT ` + columnCols + ` FROM columns c WHERE c.project_id = ?1
		AND (c.position < ?2 OR (c.position = ?2 AND c.id < ?3)) ORDER BY c.position DESC, c.id DESC LIMIT 1`
	}
	n, err := scanColumn(q.QueryRowContext(ctx, query, c.ProjectID, c.Position, c.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return n, domain.ErrNotFound
	}
	return n, err
}

// SetColumnDone marks a column as the "done" kind; tasks in it get done_at set or cleared.
func (s *Store) SetColumnDone(ctx context.Context, userID, id int64, done bool) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := getColumn(ctx, tx, userID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE columns SET is_done = ? WHERE id = ?`, boolInt(done), id); err != nil {
			return err
		}
		var doneAt any
		if done {
			doneAt = time.Now().Unix()
		}
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET done_at = ? WHERE column_id = ?`, doneAt, id)
		return err
	})
}

// DeleteColumn deletes an empty column. A project keeps at least one column.
func (s *Store) DeleteColumn(ctx context.Context, userID, id int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		c, err := getColumn(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		var tasks, cols int
		if err := tx.QueryRowContext(ctx, `SELECT
				(SELECT COUNT(*) FROM tasks WHERE column_id = ?),
				(SELECT COUNT(*) FROM columns WHERE project_id = ?)`, c.ID, c.ProjectID).
			Scan(&tasks, &cols); err != nil {
			return err
		}
		if tasks > 0 {
			return domain.ErrColumnNotEmpty
		}
		if cols <= 1 {
			return domain.ErrLastColumn
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM columns WHERE id = ?`, c.ID)
		return err
	})
}
