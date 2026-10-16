package bot

import (
	"context"
	"time"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/parser"
	"github.com/Mimic890/fusen/internal/views"
)

// morningHour is used for "tomorrow" presets and dates typed without a time.
const morningHour = 9

func atClock(day time.Time, hour, minute int, loc *time.Location) time.Time {
	d := day.In(loc)
	return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, loc)
}

func (b *Bot) remindersView(ctx context.Context, r *request) func(views.Card) views.Screen {
	return func(c views.Card) views.Screen {
		rs, err := b.store.ListReminders(ctx, c.Task.ID)
		if err != nil {
			b.log.Error("list reminders", "task_id", c.Task.ID, "err", err)
		}
		return views.Reminders(r.l, c, rs)
	}
}

// onReminders shows the reminders of a task; it also awaits a typed time.
func (b *Bot) onReminders(ctx context.Context, r *request) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	if _, err := b.store.GetTask(ctx, r.user.TgID, id); err != nil {
		return err
	}
	if preset := r.data.Arg(1); preset != "" {
		now := time.Now()
		var at time.Time
		switch preset {
		case cb.InHour:
			at = now.Add(time.Hour)
		case cb.In3Hours:
			at = now.Add(3 * time.Hour)
		case cb.Tomorrow:
			at = atClock(now.AddDate(0, 0, 1), morningHour, 0, r.user.Location())
		default:
			return domain.ErrNotFound
		}
		if err := b.store.AddReminder(ctx, r.user.TgID, id, at); err != nil {
			return err
		}
		r.toast = r.l.T("reminders.added")
	}
	if err := b.setState(ctx, r, cb.New(cb.KindReminders, id)); err != nil {
		return err
	}
	return b.showTask(ctx, r, id, "", b.remindersView(ctx, r))
}

func (b *Bot) inputReminder(ctx context.Context, r *request, text string) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	now, loc := time.Now(), r.user.Location()
	at, allDay, err := parser.ParseDue(text, now, loc)
	if err == nil {
		if allDay {
			at = atClock(at, morningHour, 0, loc)
		}
		if !at.After(now) {
			err = domain.ErrPastTime
		}
	}
	if err == nil {
		err = b.store.AddReminder(ctx, r.user.TgID, id, at)
	}
	notice := r.l.T("reminders.added")
	if isInputError(err) {
		notice = b.userError(r, err)
	} else if err != nil {
		return err
	}
	// The screen stays open for more reminders.
	return b.showTask(ctx, r, id, notice, b.remindersView(ctx, r))
}

func (b *Bot) onReminderDelete(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	taskID, err := b.store.DeleteReminder(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	r.toast = r.l.T("reminders.deleted")
	if err := b.setState(ctx, r, cb.New(cb.KindReminders, taskID)); err != nil {
		return err
	}
	return b.showTask(ctx, r, taskID, "", b.remindersView(ctx, r))
}

// Notifications.

// SendReminder implements scheduler.Notifier.
func (b *Bot) SendReminder(ctx context.Context, j domain.ReminderJob) error {
	l := b.i18n.For(j.User.Lang)
	_, err := b.sendScreen(ctx, j.User.TgID, views.ReminderNotice(l, j, j.User.Location()))
	return err
}

// SendDigest implements scheduler.Notifier.
func (b *Bot) SendDigest(ctx context.Context, u domain.User, d domain.Digest) error {
	_, err := b.sendScreen(ctx, u.TgID, views.DigestNotice(b.i18n.For(u.Lang), d, u.Location()))
	return err
}

// onNotifyOpen opens a task from a notification as a new live screen at the
// bottom of the chat, where the user is.
func (b *Bot) onNotifyOpen(ctx context.Context, r *request) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	c, err := b.loadCard(ctx, r, id)
	if err != nil {
		return err
	}
	if err := b.clearState(ctx, r); err != nil {
		return err
	}
	return b.openLive(ctx, r, views.TaskCard(r.l, c))
}

// onReminderAction handles the buttons of a reminder message. For a deadline
// reminder, "+1 h" and "Tomorrow" move the deadline; for a custom reminder
// they remind again later. The message is then replaced with the outcome.
func (b *Bot) onReminderAction(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	rem, err := b.store.GetReminder(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	t, err := b.store.GetTask(ctx, r.user.TgID, rem.TaskID)
	if err != nil {
		return err
	}
	now, loc := time.Now(), r.user.Location()
	title := views.Escape(t.Title)

	var text string
	switch act := r.data.Arg(1); {
	case act == cb.ActDone:
		c, err := b.store.CompleteTask(ctx, r.user.TgID, t.ID, now)
		if err != nil {
			return err
		}
		text = r.l.T("notify.result_done", "title", title, "column", views.Escape(c.Name))

	case (act == cb.ActHour || act == cb.ActTomorrow) && rem.Kind == domain.ReminderDeadline && t.DueAt != nil:
		due := postpone(*t.DueAt, act, now, loc)
		if err := b.store.SetTaskDue(ctx, r.user.TgID, t.ID, &due, false, r.user.RemindBeforeMin, now); err != nil {
			return err
		}
		text = r.l.T("notify.result_postponed", "title", title, "date", views.TgTime(due, loc))

	case act == cb.ActHour || act == cb.ActTomorrow:
		at := postpone(rem.RemindAt, act, now, loc)
		if err := b.store.AddReminder(ctx, r.user.TgID, t.ID, at); err != nil {
			return err
		}
		text = r.l.T("notify.result_snoozed", "title", title, "date", views.TgTime(at, loc))

	default:
		return domain.ErrNotFound
	}
	return b.editScreen(ctx, r.user.TgID, r.msgID, views.NoticeResult(r.l, text, t.ID))
}

// postpone shifts t by an hour (counting from now if t has passed) or to the
// same time of day tomorrow.
func postpone(t time.Time, act string, now time.Time, loc *time.Location) time.Time {
	if act == cb.ActHour {
		if t.Before(now) {
			t = now
		}
		return t.Add(time.Hour)
	}
	l := t.In(loc)
	return atClock(now.AddDate(0, 0, 1), l.Hour(), l.Minute(), loc)
}
