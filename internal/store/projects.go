package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

const projectCols = `p.id, p.user_id, p.name, p.position, p.archived, p.is_inbox, p.created_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProject(r rowScanner, extra ...any) (domain.Project, error) {
	var (
		p         domain.Project
		createdAt int64
	)
	dest := append([]any{&p.ID, &p.UserID, &p.Name, &p.Position, &p.Archived, &p.IsInbox, &createdAt}, extra...)
	if err := r.Scan(dest...); err != nil {
		return p, err
	}
	p.CreatedAt = time.Unix(createdAt, 0)
	return p, nil
}

// ListProjects returns the user's projects with open task counts, inbox first.
func (s *Store) ListProjects(ctx context.Context, userID int64, archived bool) ([]domain.ProjectSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+projectCols+`,
			(SELECT COUNT(*) FROM tasks t JOIN columns c ON c.id = t.column_id
			 WHERE t.project_id = p.id AND c.is_done = 0)
		FROM projects p
		WHERE p.user_id = ? AND p.archived = ?
		ORDER BY p.is_inbox DESC, p.position, p.id`, userID, boolInt(archived))
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var out []domain.ProjectSummary
	for rows.Next() {
		var ps domain.ProjectSummary
		p, err := scanProject(rows, &ps.OpenTasks)
		if err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		ps.Project = p
		out = append(out, ps)
	}
	return out, rows.Err()
}

// CountArchived returns the number of archived projects of the user.
func (s *Store) CountArchived(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM projects WHERE user_id = ? AND archived = 1`, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count archived: %w", err)
	}
	return n, nil
}

// GetProject loads a project owned by the user.
func (s *Store) GetProject(ctx context.Context, userID, id int64) (domain.Project, error) {
	return getProject(ctx, s.db, userID, id)
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func getProject(ctx context.Context, q querier, userID, id int64) (domain.Project, error) {
	p, err := scanProject(q.QueryRowContext(ctx,
		`SELECT `+projectCols+` FROM projects p WHERE p.id = ? AND p.user_id = ?`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return p, domain.ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

// CreateProject creates a project with default columns.
func (s *Store) CreateProject(ctx context.Context, userID int64, name string) (domain.Project, error) {
	var p domain.Project
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		p, err = createProject(ctx, tx, userID, name, false)
		return err
	})
	return p, err
}

func createProject(ctx context.Context, tx *sql.Tx, userID int64, name string, inbox bool) (domain.Project, error) {
	now := time.Now()
	p := domain.Project{UserID: userID, Name: name, IsInbox: inbox, CreatedAt: time.Unix(now.Unix(), 0)}

	if !inbox {
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(position), 0) + 1 FROM projects WHERE user_id = ?`, userID).
			Scan(&p.Position); err != nil {
			return p, fmt.Errorf("next project position: %w", err)
		}
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO projects (user_id, name, position, is_inbox, created_at)
		VALUES (?, ?, ?, ?, ?)`, userID, name, p.Position, boolInt(inbox), now.Unix())
	if err != nil {
		return p, fmt.Errorf("insert project: %w", err)
	}
	if p.ID, err = res.LastInsertId(); err != nil {
		return p, err
	}

	for i, c := range domain.DefaultColumns {
		if _, err := tx.ExecContext(ctx, `INSERT INTO columns (project_id, name, position, is_done)
			VALUES (?, ?, ?, ?)`, p.ID, c.Name, i, boolInt(c.IsDone)); err != nil {
			return p, fmt.Errorf("insert column: %w", err)
		}
	}
	return p, nil
}

// RenameProject renames a project owned by the user.
func (s *Store) RenameProject(ctx context.Context, userID, id int64, name string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET name = ? WHERE id = ? AND user_id = ?`, name, id, userID)
	if err != nil {
		return fmt.Errorf("rename project: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SetProjectArchived archives or restores a project. The inbox cannot be archived.
func (s *Store) SetProjectArchived(ctx context.Context, userID, id int64, archived bool) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := ensureNotInbox(ctx, tx, userID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE projects SET archived = ? WHERE id = ?`, boolInt(archived), id); err != nil {
			return fmt.Errorf("archive project: %w", err)
		}
		if archived {
			if _, err := tx.ExecContext(ctx, `UPDATE users SET current_project_id = NULL
				WHERE tg_id = ? AND current_project_id = ?`, userID, id); err != nil {
				return fmt.Errorf("reset current project: %w", err)
			}
		}
		return nil
	})
}

// DeleteProject deletes a project with all its columns and tasks. The inbox cannot be deleted.
func (s *Store) DeleteProject(ctx context.Context, userID, id int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := ensureNotInbox(ctx, tx, userID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id); err != nil {
			return fmt.Errorf("delete project: %w", err)
		}
		return nil
	})
}

func ensureNotInbox(ctx context.Context, q querier, userID, id int64) error {
	p, err := getProject(ctx, q, userID, id)
	if err != nil {
		return err
	}
	if p.IsInbox {
		return domain.ErrInboxProtected
	}
	return nil
}

// DueSummary counts the user's open tasks that are overdue or due later today.
// Tasks in archived projects are ignored.
func (s *Store) DueSummary(ctx context.Context, userID int64, now time.Time, loc *time.Location) (domain.DueSummary, error) {
	local := now.In(loc)
	endOfDay := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc)

	var ds domain.DueSummary
	err := s.db.QueryRowContext(ctx, `SELECT
			COALESCE(SUM(t.due_at < ?1), 0),
			COALESCE(SUM(t.due_at >= ?1 AND t.due_at < ?2), 0)
		FROM tasks t
		JOIN columns c ON c.id = t.column_id
		JOIN projects p ON p.id = t.project_id
		WHERE p.user_id = ?3 AND p.archived = 0 AND c.is_done = 0 AND t.due_at IS NOT NULL`,
		now.Unix(), endOfDay.Unix(), userID).Scan(&ds.Overdue, &ds.Today)
	if err != nil {
		return ds, fmt.Errorf("due summary: %w", err)
	}
	return ds, nil
}
