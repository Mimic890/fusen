package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

func ptr(t time.Time) *time.Time { return &t }

func TestAddTasksWithMeta(t *testing.T) {
	s, _, cols := setupBoard(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	due := now.Add(3 * time.Hour)

	tasks, err := s.AddTasks(ctx, 1, cols[0].ID, []domain.NewTask{
		{Title: "a", Priority: domain.PriorityHigh, Tags: []string{"x", "y"}, DueAt: &due},
		{Title: "b", DueAt: ptr(domain.EndOfDay(now, time.UTC)), DueAllDay: true},
		{Title: "c"},
	}, 60, now)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTask(ctx, 1, tasks[0].ID)
	if got.Priority != domain.PriorityHigh || !slices.Equal(got.Tags, []string{"x", "y"}) || !got.DueAt.Equal(due) || got.DueAllDay {
		t.Fatalf("task = %+v", got)
	}
	if b, _ := s.GetTask(ctx, 1, tasks[1].ID); !b.DueAllDay {
		t.Error("all-day flag lost")
	}

	// Only the timed deadline gets a reminder, 60 minutes before.
	rs, _ := s.ListReminders(ctx, tasks[0].ID)
	if len(rs) != 1 || !rs[0].RemindAt.Equal(due.Add(-time.Hour)) || rs[0].Kind != domain.ReminderDeadline {
		t.Fatalf("reminders = %+v", rs)
	}
	for _, id := range []int64{tasks[1].ID, tasks[2].ID} {
		if rs, _ := s.ListReminders(ctx, id); len(rs) != 0 {
			t.Errorf("task %d: unexpected reminders %+v", id, rs)
		}
	}
}

func TestSetTaskDueReschedules(t *testing.T) {
	s, _, cols := setupBoard(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	tasks, _ := s.CreateTasks(ctx, 1, cols[0].ID, []string{"a"})
	id := tasks[0].ID

	if err := s.SetTaskDue(ctx, 1, id, ptr(now.Add(5*time.Hour)), false, 60, now); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTaskDue(ctx, 1, id, ptr(now.Add(10*time.Hour)), false, 60, now); err != nil {
		t.Fatal(err)
	}
	rs, _ := s.ListReminders(ctx, id)
	if len(rs) != 1 || !rs[0].RemindAt.Equal(now.Add(9*time.Hour)) {
		t.Fatalf("reminders after change = %+v", rs)
	}

	// Custom reminders survive deadline changes; clearing the deadline drops its reminder.
	if err := s.AddReminder(ctx, 1, id, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTaskDue(ctx, 1, id, nil, false, 60, now); err != nil {
		t.Fatal(err)
	}
	rs, _ = s.ListReminders(ctx, id)
	if len(rs) != 1 || rs[0].Kind != domain.ReminderCustom {
		t.Fatalf("reminders after clearing = %+v", rs)
	}
	if got, _ := s.GetTask(ctx, 1, id); got.DueAt != nil {
		t.Error("deadline not cleared")
	}

	if err := s.SetTaskDue(ctx, 2, id, nil, false, 60, now); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign task: %v", err)
	}
}

func TestRescheduleDeadlines(t *testing.T) {
	s, _, cols := setupBoard(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	due := now.Add(5 * time.Hour)
	tasks, _ := s.AddTasks(ctx, 1, cols[0].ID, []domain.NewTask{{Title: "a", DueAt: &due}}, 60, now)

	if err := s.RescheduleDeadlines(ctx, 1, 180, now); err != nil {
		t.Fatal(err)
	}
	rs, _ := s.ListReminders(ctx, tasks[0].ID)
	if len(rs) != 1 || !rs[0].RemindAt.Equal(due.Add(-3*time.Hour)) {
		t.Fatalf("reminders = %+v", rs)
	}
	if err := s.RescheduleDeadlines(ctx, 1, 0, now); err != nil {
		t.Fatal(err)
	}
	if rs, _ = s.ListReminders(ctx, tasks[0].ID); len(rs) != 0 {
		t.Fatalf("disabled reminders kept: %+v", rs)
	}
}

func TestDueRemindersAndSkip(t *testing.T) {
	s, p, cols := setupBoard(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	tasks, _ := s.CreateTasks(ctx, 1, cols[0].ID, []string{"open", "done"})
	_ = s.AddReminder(ctx, 1, tasks[0].ID, now.Add(-time.Minute))
	_ = s.AddReminder(ctx, 1, tasks[1].ID, now.Add(-2*time.Minute))
	_ = s.AddReminder(ctx, 1, tasks[0].ID, now.Add(time.Hour)) // not due yet
	if _, err := s.CompleteTask(ctx, 1, tasks[1].ID, now); err != nil {
		t.Fatal(err)
	}

	jobs, err := s.DueReminders(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[0].Task.Title != "done" || !jobs[0].Skip || jobs[1].Skip ||
		jobs[1].ProjectName != p.Name || jobs[1].User.TgID != 1 || jobs[1].User.Lang != "ru" {
		t.Fatalf("jobs = %+v", jobs)
	}
	for _, j := range jobs {
		_ = s.MarkReminderSent(ctx, j.ID, now)
	}
	if jobs, _ = s.DueReminders(ctx, now, 10); len(jobs) != 0 {
		t.Errorf("sent reminders returned again: %+v", jobs)
	}
}

func TestCustomReminders(t *testing.T) {
	s, _, cols := setupBoard(t)
	ctx := context.Background()
	now := time.Now()
	tasks, _ := s.CreateTasks(ctx, 1, cols[0].ID, []string{"a"})
	id := tasks[0].ID

	for i := range domain.MaxReminders {
		if err := s.AddReminder(ctx, 1, id, now.Add(time.Duration(i+1)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddReminder(ctx, 1, id, now); !errors.Is(err, domain.ErrTooManyReminders) {
		t.Errorf("limit: %v", err)
	}
	if err := s.AddReminder(ctx, 2, id, now); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign add: %v", err)
	}
	rs, _ := s.ListReminders(ctx, id)
	if _, err := s.DeleteReminder(ctx, 2, rs[0].ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign delete: %v", err)
	}
	if tid, err := s.DeleteReminder(ctx, 1, rs[0].ID); err != nil || tid != id {
		t.Fatalf("delete: %d %v", tid, err)
	}
}

func TestCompleteAndMoveToProject(t *testing.T) {
	s, p, cols := setupBoard(t)
	ctx := context.Background()
	now := time.Now()
	tasks, _ := s.CreateTasks(ctx, 1, cols[0].ID, []string{"a"})
	id := tasks[0].ID

	c, err := s.CompleteTask(ctx, 1, id, now)
	if err != nil || c.ID != cols[2].ID {
		t.Fatalf("complete: %+v %v", c, err)
	}
	if got, _ := s.GetTask(ctx, 1, id); got.DoneAt == nil {
		t.Error("done_at not set")
	}
	if c, err = s.CompleteTask(ctx, 1, id, now); err != nil || c.ID != cols[2].ID {
		t.Errorf("completing a done task: %+v %v", c, err)
	}

	other, _ := s.CreateProject(ctx, 1, "B", nil)
	oc, _ := s.ListColumns(ctx, other.ID)
	c, err = s.MoveTaskToProject(ctx, 1, id, other.ID, now)
	if err != nil || c.ID != oc[0].ID {
		t.Fatalf("move: %+v %v", c, err)
	}
	got, _ := s.GetTask(ctx, 1, id)
	if got.ProjectID != other.ID || got.ColumnID != oc[0].ID || got.DoneAt != nil {
		t.Fatalf("moved task = %+v", got)
	}
	if _, err := s.MoveTaskToProject(ctx, 2, id, p.ID, now); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign move: %v", err)
	}
	foreign, _ := s.ListProjects(ctx, 2, false)
	if _, err := s.MoveTaskToProject(ctx, 1, id, foreign[0].ID, now); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("move into a foreign project: %v", err)
	}

	// No done column: completing is impossible.
	for _, col := range oc {
		if col.IsDone {
			_ = s.SetColumnDone(ctx, 1, col.ID, false)
		}
	}
	if _, err := s.CompleteTask(ctx, 1, id, now); !errors.Is(err, domain.ErrNoDoneColumn) {
		t.Errorf("no done column: %v", err)
	}
}

func TestDigest(t *testing.T) {
	s, p, cols := setupBoard(t)
	ctx := context.Background()
	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, loc)
	at := func(h int) *time.Time { return ptr(now.Add(time.Duration(h) * time.Hour)) }

	_, _ = s.AddTasks(ctx, 1, cols[0].ID, []domain.NewTask{
		{Title: "overdue", DueAt: at(-30)},
		{Title: "today", DueAt: at(5)},
		{Title: "all day", DueAt: ptr(domain.EndOfDay(now, loc)), DueAllDay: true},
		{Title: "tomorrow", DueAt: at(20)},
		{Title: "no date"},
	}, 0, now)
	_, _ = s.AddTasks(ctx, 1, cols[2].ID, []domain.NewTask{{Title: "done", DueAt: at(-1)}}, 0, now)

	d, err := s.DigestFor(ctx, 1, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	titles := func(items []domain.DigestItem) []string {
		var out []string
		for _, i := range items {
			out = append(out, i.Task.Title)
		}
		return out
	}
	if !slices.Equal(titles(d.Overdue), []string{"overdue"}) || !slices.Equal(titles(d.Today), []string{"today", "all day"}) {
		t.Fatalf("digest: overdue %v, today %v", titles(d.Overdue), titles(d.Today))
	}
	if d.Today[0].ProjectName != p.Name {
		t.Errorf("project name = %q", d.Today[0].ProjectName)
	}

	users, _ := s.DigestUsers(ctx)
	if len(users) != 2 {
		t.Fatalf("digest users = %d", len(users))
	}
	_ = s.MarkDigestSent(ctx, 1, "2026-10-15")
	if u, _ := s.GetUser(ctx, 1); u.DigestSentOn != "2026-10-15" {
		t.Errorf("sent on = %q", u.DigestSentOn)
	}
	_ = s.SetDigestTime(ctx, 2, "")
	if users, _ = s.DigestUsers(ctx); len(users) != 1 {
		t.Errorf("disabled digest still listed")
	}
}
