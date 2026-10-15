package bot

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
)

func TestQuickAddToInbox(t *testing.T) {
	h, inbox, cols := boardHarness(t)
	ctx := context.Background()

	h.message("купить кабель #homelab !завтра 18:00 !!", 601)
	calls := h.api.take()
	if got := methods(calls); got != "editMessageText,deleteMessage" {
		t.Fatalf("calls: %s", got)
	}
	text, markup := lastEdit(t, calls)
	if !strings.Contains(text, "Входящие</b> › К выполнению") || !strings.Contains(text, "#homelab") ||
		!strings.Contains(text, "<tg-time") || !strings.Contains(markup, "Перенести в проект") {
		t.Fatalf("confirmation: %q %q", text, markup)
	}

	tasks, _, _ := h.store.ListColumnTasks(ctx, cols[0].ID, 0, 10)
	if len(tasks) != 1 {
		t.Fatalf("inbox tasks = %+v", tasks)
	}
	got, _ := h.store.GetTask(ctx, userID, tasks[0].ID)
	if got.Title != "купить кабель" || got.ProjectID != inbox.ID || got.Priority != domain.PriorityMed ||
		!slices.Equal(got.Tags, []string{"homelab"}) || got.DueAt == nil || got.DueAt.UTC().Hour() != 18 {
		t.Fatalf("task = %+v", got)
	}
	// Default reminder: one hour before the deadline.
	rs, _ := h.store.ListReminders(ctx, got.ID)
	if len(rs) != 1 || !rs[0].RemindAt.Equal(got.DueAt.Add(-time.Hour)) {
		t.Errorf("reminders = %+v", rs)
	}
}

func TestQuickAddToOpenProject(t *testing.T) {
	h, _, _ := boardHarness(t)
	ctx := context.Background()
	live := h.live()
	p, _ := h.store.CreateProject(ctx, userID, "Garage", nil)
	cols, _ := h.store.ListColumns(ctx, p.ID)

	h.press(live, cb.New(cb.KindProject, p.ID))
	h.message("one\ntwo !", 601)
	tasks, _, _ := h.store.ListColumnTasks(ctx, cols[0].ID, 0, 10)
	if len(tasks) != 2 || tasks[1].Title != "two" || tasks[1].Priority != domain.PriorityLow {
		t.Fatalf("project tasks = %+v", tasks)
	}

	// Back on the home screen, quick input goes to the inbox again.
	h.press(live, cb.New(cb.KindHome))
	h.message("three", 602)
	if tasks, _, _ = h.store.ListColumnTasks(ctx, cols[0].ID, 0, 10); len(tasks) != 2 {
		t.Errorf("task went to the closed project")
	}
}

func TestQuickAddErrorKeepsMessage(t *testing.T) {
	h, _, _ := boardHarness(t)
	h.message("#only #tags !!", 601)
	calls := h.api.take()
	if got := methods(calls); got != "editMessageText" {
		t.Fatalf("calls: %s (the message must not be deleted)", got)
	}
	if text, _ := lastEdit(t, calls); !strings.Contains(text, "не может быть пустым") {
		t.Errorf("notice: %q", text)
	}
}

func TestUnknownCommandIsNotATask(t *testing.T) {
	h, _, cols := boardHarness(t)
	h.message("/help", 601)
	if tasks, _, _ := h.store.ListColumnTasks(context.Background(), cols[0].ID, 0, 10); len(tasks) != 0 {
		t.Errorf("command became a task: %+v", tasks)
	}
}

func TestMoveQuickTaskToProject(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	live := h.live()
	p, _ := h.store.CreateProject(ctx, userID, "Garage", nil)
	pc, _ := h.store.ListColumns(ctx, p.ID)

	h.message("fix door", 601)
	tasks, _, _ := h.store.ListColumnTasks(ctx, cols[0].ID, 0, 10)
	id := tasks[0].ID

	h.press(live, cb.New(cb.KindTaskProject, id))
	_, markup := lastEdit(t, h.api.take())
	if !strings.Contains(markup, "Garage") || strings.Contains(markup, "Входящие") {
		t.Fatalf("picker must list other projects only: %s", markup)
	}
	h.press(live, cb.New(cb.KindTaskProject, id, p.ID))
	if got, _ := h.store.GetTask(ctx, userID, id); got.ColumnID != pc[0].ID {
		t.Fatalf("task not moved: %+v", got)
	}
}

func TestEditDeadline(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	live := h.live()
	tasks, _ := h.store.CreateTasks(ctx, userID, cols[0].ID, []string{"x"})
	id := tasks[0].ID

	h.press(live, cb.New(cb.KindTaskDue, id))
	h.message("someday", 601)
	if text, _ := lastEdit(t, h.api.take()); !strings.Contains(text, "Не понял дату") {
		t.Errorf("bad date notice: %q", text)
	}
	h.message("25.12", 602)
	got, _ := h.store.GetTask(ctx, userID, id)
	if got.DueAt == nil || !got.DueAllDay || got.DueAt.In(time.UTC).Format("02.01 15:04") != "25.12 23:59" {
		t.Fatalf("all-day deadline: %+v", got)
	}
	if text, _ := lastEdit(t, h.api.take()); !strings.Contains(text, `format="wd"`) {
		t.Errorf("all-day deadline must be shown as a date: %q", text)
	}

	h.press(live, cb.New(cb.KindTaskDue, id, cb.Clear))
	if got, _ = h.store.GetTask(ctx, userID, id); got.DueAt != nil {
		t.Error("deadline not cleared")
	}
}

func TestRemindersScreen(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	live := h.live()
	tasks, _ := h.store.CreateTasks(ctx, userID, cols[0].ID, []string{"x"})
	id := tasks[0].ID

	h.press(live, cb.New(cb.KindReminders, id, cb.InHour))
	h.message("23:59", 601)
	h.message("01.01.2020 10:00", 602)
	if text, _ := lastEdit(t, h.api.take()); !strings.Contains(text, "уже прошло") {
		t.Errorf("past time notice: %q", text)
	}
	rs, _ := h.store.ListReminders(ctx, id)
	if len(rs) != 2 {
		t.Fatalf("reminders = %+v", rs)
	}
	if u, _ := h.store.GetUser(ctx, userID); u.State == "" {
		t.Error("reminders screen must keep accepting times")
	}

	h.press(live, cb.New(cb.KindReminderDelete, rs[0].ID))
	if rs, _ = h.store.ListReminders(ctx, id); len(rs) != 1 {
		t.Fatalf("after delete = %+v", rs)
	}
}

// sendDueReminder delivers the first due reminder of the user like the scheduler does.
func sendDueReminder(t *testing.T, h *harness, at time.Time) (domain.ReminderJob, int) {
	t.Helper()
	jobs, err := h.store.DueReminders(context.Background(), at, 10)
	if err != nil || len(jobs) == 0 {
		t.Fatalf("due reminders: %v %v", jobs, err)
	}
	if err := h.bot.SendReminder(context.Background(), jobs[0]); err != nil {
		t.Fatal(err)
	}
	return jobs[0], h.api.nextID
}

func TestReminderNotificationActions(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	now := time.Now()
	due := now.Add(30 * time.Minute)
	tasks, _ := h.store.AddTasks(ctx, userID, cols[0].ID, []domain.NewTask{{Title: "call", DueAt: &due}}, 60, now)
	id := tasks[0].ID
	live := h.live()
	h.api.take()

	// "+1 h" on a deadline reminder moves the deadline.
	_, msg := sendDueReminder(t, h, due)
	calls := h.api.take()
	if text := lastText(calls, "sendMessage"); !strings.Contains(text, "Скоро дедлайн") || !strings.Contains(text, `format="r"`) {
		t.Errorf("notice: %q", text)
	}
	h.press(msg, cb.New(cb.KindReminderAction, mustReminder(t, h, id).ID, cb.ActHour))
	calls = h.api.take()
	edit := lastCall(t, calls, "editMessageText")
	if edit.params["message_id"] != itoa(msg) || !strings.Contains(edit.params["text"], "перенесён") {
		t.Fatalf("result edit: %+v", edit.params)
	}
	got, _ := h.store.GetTask(ctx, userID, id)
	if !got.DueAt.Equal(due.Truncate(time.Second).Add(time.Hour)) {
		t.Errorf("deadline = %v, want %v", got.DueAt, due.Add(time.Hour))
	}
	if h.live() != live {
		t.Error("notification actions must not touch the live screen")
	}

	// "Done" completes the task.
	h.press(msg, cb.New(cb.KindReminderAction, mustReminder(t, h, id).ID, cb.ActDone))
	if got, _ = h.store.GetTask(ctx, userID, id); got.ColumnID != cols[2].ID {
		t.Errorf("task not done: %+v", got)
	}

	// "Open" brings the card as a new live screen at the bottom of the chat.
	h.api.take()
	h.press(msg, cb.New(cb.KindNotifyOpen, id))
	calls = h.api.take()
	if !strings.HasPrefix(methods(calls), "sendMessage,deleteMessage") || h.live() == live {
		t.Errorf("open from notification: %s", methods(calls))
	}
}

func TestCustomReminderSnooze(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	tasks, _ := h.store.CreateTasks(ctx, userID, cols[0].ID, []string{"stretch"})
	id := tasks[0].ID
	at := time.Now().Add(-time.Minute)
	_ = h.store.AddReminder(ctx, userID, id, at)

	job, msg := sendDueReminder(t, h, time.Now())
	_ = h.store.MarkReminderSent(ctx, job.ID, time.Now())
	h.press(msg, cb.New(cb.KindReminderAction, job.ID, cb.ActTomorrow))
	rs, _ := h.store.ListReminders(ctx, id)
	if len(rs) != 1 || rs[0].RemindAt.Sub(time.Now()) < 20*time.Hour {
		t.Fatalf("snoozed reminders = %+v", rs)
	}
	if !strings.Contains(lastCall(t, h.api.take(), "editMessageText").params["text"], "напомню") {
		t.Error("snooze result text")
	}
}

func TestDigestNotification(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	now := time.Now()
	past := now.Add(-48 * time.Hour)
	_, _ = h.store.AddTasks(ctx, userID, cols[0].ID, []domain.NewTask{{Title: "late", DueAt: &past}}, 0, now)
	u, _ := h.store.GetUser(ctx, userID)
	d, _ := h.store.DigestFor(ctx, userID, now, u.Location())

	if err := h.bot.SendDigest(ctx, u, d); err != nil {
		t.Fatal(err)
	}
	call := lastCall(t, h.api.take(), "sendMessage")
	if !strings.Contains(call.params["text"], "Просрочено · 1") || !strings.Contains(call.params["reply_markup"], `"no:`) {
		t.Errorf("digest: %+v", call.params)
	}
}

func TestRemindSettingReschedules(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	now := time.Now()
	due := now.Add(10 * time.Hour)
	tasks, _ := h.store.AddTasks(ctx, userID, cols[0].ID, []domain.NewTask{{Title: "x", DueAt: &due}}, 60, now)

	h.press(h.live(), cb.New(cb.KindRemind, 180))
	rs, _ := h.store.ListReminders(ctx, tasks[0].ID)
	if len(rs) != 1 || !rs[0].RemindAt.Equal(due.Truncate(time.Second).Add(-3*time.Hour)) {
		t.Fatalf("reminders = %+v", rs)
	}
}

func mustReminder(t *testing.T, h *harness, taskID int64) domain.Reminder {
	t.Helper()
	rows, err := h.store.ListReminders(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	// Sent reminders are not listed; look them up through the latest one.
	if len(rows) > 0 {
		return rows[0]
	}
	jobs, _ := h.store.DueReminders(context.Background(), time.Now().Add(48*time.Hour), 10)
	for _, j := range jobs {
		if j.TaskID == taskID {
			return j.Reminder
		}
	}
	t.Fatalf("no reminder for task %d", taskID)
	return domain.Reminder{}
}
