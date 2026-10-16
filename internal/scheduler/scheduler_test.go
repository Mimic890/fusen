package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

type fakeStore struct {
	jobs     []domain.ReminderJob
	users    []domain.User
	digests  map[int64]domain.Digest
	sent     []int64
	digested map[int64]string
}

func (f *fakeStore) DueReminders(context.Context, time.Time, int) ([]domain.ReminderJob, error) {
	return f.jobs, nil
}

func (f *fakeStore) MarkReminderSent(_ context.Context, id int64, _ time.Time) error {
	f.sent = append(f.sent, id)
	return nil
}

func (f *fakeStore) DigestUsers(context.Context) ([]domain.User, error) { return f.users, nil }

func (f *fakeStore) DigestFor(_ context.Context, id int64, _ time.Time, _ *time.Location) (domain.Digest, error) {
	return f.digests[id], nil
}

func (f *fakeStore) MarkDigestSent(_ context.Context, id int64, date string) error {
	f.digested[id] = date
	return nil
}

type fakeNotifier struct {
	reminders []int64
	digests   []int64
	fail      bool
}

func (f *fakeNotifier) SendReminder(_ context.Context, j domain.ReminderJob) error {
	f.reminders = append(f.reminders, j.ID)
	if f.fail {
		return errors.New("blocked by user")
	}
	return nil
}

func (f *fakeNotifier) SendDigest(_ context.Context, u domain.User, _ domain.Digest) error {
	f.digests = append(f.digests, u.TgID)
	return nil
}

func newTest(st *fakeStore, n *fakeNotifier, now time.Time) *Scheduler {
	s := New(st, n, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.now = func() time.Time { return now }
	s.pause = func(context.Context) error { return nil }
	return s
}

func TestRemindersSkipAndMark(t *testing.T) {
	st := &fakeStore{jobs: []domain.ReminderJob{
		{Reminder: domain.Reminder{ID: 1}},
		{Reminder: domain.Reminder{ID: 2}, Skip: true},
		{Reminder: domain.Reminder{ID: 3}},
	}, digested: map[int64]string{}}
	n := &fakeNotifier{fail: true}
	newTest(st, n, time.Now()).Tick(context.Background())

	if len(n.reminders) != 2 || n.reminders[0] != 1 || n.reminders[1] != 3 {
		t.Errorf("sent = %v", n.reminders)
	}
	// Failed and skipped reminders are marked too, so they are not retried every tick.
	if len(st.sent) != 3 {
		t.Errorf("marked = %v", st.sent)
	}
}

func TestDigestDue(t *testing.T) {
	msk, _ := time.LoadLocation("Europe/Moscow")
	at := func(h, m int) time.Time { return time.Date(2026, 10, 15, h, m, 0, 0, msk) }
	u := domain.User{TgID: 1, Timezone: "Europe/Moscow", DigestTime: "09:00"}

	cases := []struct {
		name string
		u    func(domain.User) domain.User
		now  time.Time
		want bool
	}{
		{"before", nil, at(8, 59), false},
		{"on time", nil, at(9, 0), true},
		{"late but in window", nil, at(11, 59), true},
		{"after window", nil, at(12, 0), false},
		{"already sent today", func(u domain.User) domain.User { u.DigestSentOn = "2026-10-15"; return u }, at(9, 30), false},
		{"sent yesterday", func(u domain.User) domain.User { u.DigestSentOn = "2026-10-14"; return u }, at(9, 30), true},
		{"disabled", func(u domain.User) domain.User { u.DigestTime = ""; return u }, at(9, 30), false},
		// 06:30 UTC is 09:30 in Moscow.
		{"timezone", nil, time.Date(2026, 10, 15, 6, 30, 0, 0, time.UTC), true},
	}
	for _, c := range cases {
		user := u
		if c.u != nil {
			user = c.u(u)
		}
		date, ok := DigestDue(user, c.now)
		if ok != c.want || (ok && date != "2026-10-15") {
			t.Errorf("%s: got %q %v", c.name, date, ok)
		}
	}
}

func TestDigestsSendOnceAndSkipEmpty(t *testing.T) {
	msk, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 15, 9, 10, 0, 0, msk)
	item := domain.DigestItem{Task: domain.Task{Title: "x"}}
	st := &fakeStore{
		users: []domain.User{
			{TgID: 1, Timezone: "Europe/Moscow", DigestTime: "09:00"},
			{TgID: 2, Timezone: "Europe/Moscow", DigestTime: "09:00"}, // nothing to report
			{TgID: 3, Timezone: "Europe/Moscow", DigestTime: "10:00"}, // not yet
		},
		digests:  map[int64]domain.Digest{1: {Today: []domain.DigestItem{item}}, 3: {Overdue: []domain.DigestItem{item}}},
		digested: map[int64]string{},
	}
	n := &fakeNotifier{}
	newTest(st, n, now).Tick(context.Background())

	if len(n.digests) != 1 || n.digests[0] != 1 {
		t.Errorf("digests sent to %v", n.digests)
	}
	if st.digested[1] != "2026-10-15" || st.digested[2] != "2026-10-15" || st.digested[3] != "" {
		t.Errorf("marked = %v", st.digested)
	}
}
