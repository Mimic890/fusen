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
	// Columns of the inbox; domain.DefaultColumns when nil.
	Columns []domain.DefaultColumn
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
		res, err := tx.ExecContext(ctx, `INSERT INTO users (tg_id, lang, timezone, digest_time, created_at)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT (tg_id) DO NOTHING`,
			nu.TgID, nu.Lang, nu.Timezone, domain.DefaultDigestTime, now)
		if err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // created concurrently
		}
		created = true
		_, err = createProject(ctx, tx, nu.TgID, nu.InboxName, true, nu.Columns)
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

// SetViewMode stores the user's board view.
func (s *Store) SetViewMode(ctx context.Context, tgID int64, v domain.ViewMode) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET view_mode = ? WHERE tg_id = ?`, string(v), tgID)
	if err != nil {
		return fmt.Errorf("set view mode: %w", err)
	}
	return nil
}

// SetLang stores the UI language. If the inbox still has the default name of
// the previous language, it is renamed to the default of the new one.
func (s *Store) SetLang(ctx context.Context, tgID int64, lang, oldInbox, newInbox string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET lang = ? WHERE tg_id = ?`, lang, tgID); err != nil {
			return fmt.Errorf("set lang: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET name = ?
			WHERE user_id = ? AND is_inbox = 1 AND name = ?`, newInbox, tgID, oldInbox); err != nil {
			return fmt.Errorf("rename inbox: %w", err)
		}
		return nil
	})
}

// SetTimezone stores the user's IANA timezone name.
func (s *Store) SetTimezone(ctx context.Context, tgID int64, tz string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET timezone = ? WHERE tg_id = ?`, tz, tgID)
	if err != nil {
		return fmt.Errorf("set timezone: %w", err)
	}
	return nil
}

// SetDigestTime stores the morning digest time ("HH:MM"); empty disables the digest.
// The sent marker is reset so a new time can still fire today.
func (s *Store) SetDigestTime(ctx context.Context, tgID int64, clock string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET digest_time = ?, digest_sent_on = NULL WHERE tg_id = ?`,
		sql.NullString{String: clock, Valid: clock != ""}, tgID)
	if err != nil {
		return fmt.Errorf("set digest time: %w", err)
	}
	return nil
}

// SetRemindBefore stores how many minutes before a deadline to remind; 0 disables.
func (s *Store) SetRemindBefore(ctx context.Context, tgID int64, minutes int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET remind_before_min = ? WHERE tg_id = ?`, minutes, tgID)
	if err != nil {
		return fmt.Errorf("set remind before: %w", err)
	}
	return nil
}
