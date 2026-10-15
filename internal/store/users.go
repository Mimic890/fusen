package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

// NewUser holds defaults for a user created on first contact.
type NewUser struct {
	TgID      int64
	Lang      string
	Timezone  string
	InboxName string
}

// EnsureUser returns the user, creating it together with its inbox project on
// first contact. created reports whether the user was just created.
func (s *Store) EnsureUser(ctx context.Context, nu NewUser) (u domain.User, created bool, err error) {
	u, err = s.GetUser(ctx, nu.TgID)
	if err == nil {
		return u, false, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return u, false, err
	}

	err = s.inTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().Unix()
		res, err := tx.ExecContext(ctx, `INSERT INTO users (tg_id, lang, timezone, created_at)
			VALUES (?, ?, ?, ?) ON CONFLICT (tg_id) DO NOTHING`,
			nu.TgID, nu.Lang, nu.Timezone, now)
		if err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // created concurrently
		}
		created = true
		_, err = createProject(ctx, tx, nu.TgID, nu.InboxName, true)
		return err
	})
	if err != nil {
		return u, false, err
	}
	u, err = s.GetUser(ctx, nu.TgID)
	return u, created, err
}

// GetUser loads a user by Telegram ID.
func (s *Store) GetUser(ctx context.Context, tgID int64) (domain.User, error) {
	var (
		u         domain.User
		digest    sql.NullString
		liveMsg   sql.NullInt64
		curProj   sql.NullInt64
		viewMode  string
		createdAt int64
	)
	err := s.db.QueryRowContext(ctx, `SELECT tg_id, lang, view_mode, timezone, digest_time,
			remind_before_min, live_message_id, state, current_project_id, created_at
		FROM users WHERE tg_id = ?`, tgID).
		Scan(&u.TgID, &u.Lang, &viewMode, &u.Timezone, &digest,
			&u.RemindBeforeMin, &liveMsg, &u.State, &curProj, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, domain.ErrNotFound
	}
	if err != nil {
		return u, fmt.Errorf("get user: %w", err)
	}
	u.ViewMode = domain.ViewMode(viewMode)
	u.DigestTime = digest.String
	u.LiveMessageID = int(liveMsg.Int64)
	u.CurrentProjectID = curProj.Int64
	u.CreatedAt = time.Unix(createdAt, 0)
	return u, nil
}

// SetLiveMessage stores the ID of the user's live screen message (0 clears it).
func (s *Store) SetLiveMessage(ctx context.Context, tgID int64, msgID int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET live_message_id = ? WHERE tg_id = ?`,
		sql.NullInt64{Int64: int64(msgID), Valid: msgID != 0}, tgID)
	if err != nil {
		return fmt.Errorf("set live message: %w", err)
	}
	return nil
}

// SetState stores the pending input state (empty clears it).
func (s *Store) SetState(ctx context.Context, tgID int64, state string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET state = ? WHERE tg_id = ?`, state, tgID)
	if err != nil {
		return fmt.Errorf("set state: %w", err)
	}
	return nil
}

// SetCurrentProject remembers the project the user is looking at (0 clears it).
func (s *Store) SetCurrentProject(ctx context.Context, tgID, projectID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET current_project_id = ? WHERE tg_id = ?`,
		sql.NullInt64{Int64: projectID, Valid: projectID != 0}, tgID)
	if err != nil {
		return fmt.Errorf("set current project: %w", err)
	}
	return nil
}
