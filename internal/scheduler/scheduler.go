// Package scheduler delivers due reminders and morning digests.
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

const (
	// Interval between checks.
	Interval = 30 * time.Second
	// DigestWindow is how long after the digest time a missed digest is still
	// sent, e.g. after a restart. Later it is skipped until the next day.
	DigestWindow = 3 * time.Hour
	// batchSize limits reminders delivered per tick.
	batchSize = 100
	// sendPause spaces out messages to stay within Telegram flood limits.
	sendPause = 50 * time.Millisecond
)

// Store is the data the scheduler needs.
type Store interface {
	DueReminders(ctx context.Context, now time.Time, limit int) ([]domain.ReminderJob, error)
	MarkReminderSent(ctx context.Context, id int64, now time.Time) error
	DigestUsers(ctx context.Context) ([]domain.User, error)
	DigestFor(ctx context.Context, userID int64, now time.Time, loc *time.Location) (domain.Digest, error)
	MarkDigestSent(ctx context.Context, userID int64, date string) error
}

// Notifier delivers messages to users.
type Notifier interface {
	SendReminder(ctx context.Context, job domain.ReminderJob) error
	SendDigest(ctx context.Context, u domain.User, d domain.Digest) error
}

// Scheduler checks for due reminders and digests periodically.
type Scheduler struct {
	store  Store
	notify Notifier
	log    *slog.Logger
	now    func() time.Time
	pause  func(ctx context.Context) error
}

// New creates a scheduler.
func New(st Store, n Notifier, log *slog.Logger) *Scheduler {
	return &Scheduler{store: st, notify: n, log: log, now: time.Now, pause: func(ctx context.Context) error {
		t := time.NewTimer(sendPause)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}}
}

// Run checks every Interval until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(Interval)
	defer t.Stop()
	for {
		s.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick delivers everything that is due now.
func (s *Scheduler) Tick(ctx context.Context) {
	s.reminders(ctx)
	s.digests(ctx)
}

func (s *Scheduler) reminders(ctx context.Context) {
	now := s.now()
	jobs, err := s.store.DueReminders(ctx, now, batchSize)
	if err != nil {
		s.log.Error("load due reminders", "err", err)
		return
	}
	for _, j := range jobs {
		if !j.Skip {
			if err := s.notify.SendReminder(ctx, j); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				// Not retried: a failing reminder must not be resent every tick.
				s.log.Warn("send reminder", "reminder_id", j.ID, "user_id", j.User.TgID, "err", err)
			}
		}
		if err := s.store.MarkReminderSent(ctx, j.ID, now); err != nil {
			s.log.Error("mark reminder sent", "reminder_id", j.ID, "err", err)
			return
		}
		if !j.Skip && s.pause(ctx) != nil {
			return
		}
	}
}

func (s *Scheduler) digests(ctx context.Context) {
	now := s.now()
	users, err := s.store.DigestUsers(ctx)
	if err != nil {
		s.log.Error("load digest users", "err", err)
		return
	}
	for _, u := range users {
		date, ok := DigestDue(u, now)
		if !ok {
			continue
		}
		loc := u.Location()
		d, err := s.store.DigestFor(ctx, u.TgID, now, loc)
		if err != nil {
			s.log.Error("load digest", "user_id", u.TgID, "err", err)
			continue
		}
		if !d.Empty() {
			if err := s.notify.SendDigest(ctx, u, d); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				s.log.Warn("send digest", "user_id", u.TgID, "err", err)
			}
		}
		// Marked even when empty or failed, so it is attempted once a day.
		if err := s.store.MarkDigestSent(ctx, u.TgID, date); err != nil {
			s.log.Error("mark digest sent", "user_id", u.TgID, "err", err)
			continue
		}
		if !d.Empty() && s.pause(ctx) != nil {
			return
		}
	}
}

// DigestDue reports whether the user's digest should be sent at now and
// returns the local date to record it under.
func DigestDue(u domain.User, now time.Time) (string, bool) {
	if u.DigestTime == "" {
		return "", false
	}
	clock, err := time.Parse("15:04", u.DigestTime)
	if err != nil {
		return "", false
	}
	local := now.In(u.Location())
	date := local.Format(time.DateOnly)
	if u.DigestSentOn == date {
		return "", false
	}
	start := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, local.Location())
	if local.Before(start) || !local.Before(start.Add(DigestWindow)) {
		return "", false
	}
	return date, true
}
