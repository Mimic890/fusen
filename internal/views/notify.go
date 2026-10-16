package views

import (
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/i18n"
)

// digestListLimit caps tasks listed per digest section; digestButtons caps task buttons.
const (
	digestListLimit = 15
	digestButtons   = 6
)

func openButton(l i18n.Localizer, taskID int64) models.InlineKeyboardButton {
	return btn(l.T("btn.open"), cb.New(cb.KindNotifyOpen, taskID))
}

// ReminderNotice is a reminder message. Its buttons act on the reminder
// directly, so they work no matter where the live screen is.
func ReminderNotice(l i18n.Localizer, j domain.ReminderJob, loc *time.Location) Screen {
	t := j.Task
	key := "notify.reminder"
	if j.Kind == domain.ReminderDeadline {
		key = "notify.deadline"
	}
	text := l.T(key, "title", Escape(priorityIcon(t.Priority)+t.Title)) + "\n" + Escape(j.ProjectName)
	if t.DueAt != nil {
		text += "\n" + l.T("task.due", "date", TgDue(t, loc))
		if !t.DueAllDay {
			text += " (" + TgRelative(*t.DueAt, loc) + ")"
		}
	}
	return Screen{Text: text, Keyboard: [][]models.InlineKeyboardButton{
		row(
			btn(l.T("notify.done"), cb.New(cb.KindReminderAction, j.ID, cb.ActDone)),
			btn(l.T("notify.hour"), cb.New(cb.KindReminderAction, j.ID, cb.ActHour)),
			btn(l.T("notify.tomorrow"), cb.New(cb.KindReminderAction, j.ID, cb.ActTomorrow)),
		),
		row(openButton(l, t.ID)),
	}}
}

// NoticeResult replaces a reminder message after an action; only Open is kept.
func NoticeResult(l i18n.Localizer, text string, taskID int64) Screen {
	return Screen{Text: text, Keyboard: [][]models.InlineKeyboardButton{row(openButton(l, taskID))}}
}

func digestSection(l i18n.Localizer, sb *strings.Builder, key string, items []domain.DigestItem, loc *time.Location) {
	if len(items) == 0 {
		return
	}
	sb.WriteString("\n\n" + l.T(key, "count", len(items)))
	for i, it := range items {
		if i == digestListLimit {
			sb.WriteString("\n" + l.T("digest.more", "count", len(items)-i))
			break
		}
		sb.WriteString("\n• " + Escape(priorityIcon(it.Task.Priority)+it.Task.Title) + " · " +
			Escape(it.ProjectName) + " · " + TgDue(it.Task, loc))
	}
}

// DigestNotice is the morning digest: overdue tasks first, then today's.
func DigestNotice(l i18n.Localizer, d domain.Digest, loc *time.Location) Screen {
	var sb strings.Builder
	sb.WriteString(l.T("digest.greeting"))
	digestSection(l, &sb, "digest.overdue", d.Overdue, loc)
	digestSection(l, &sb, "digest.today", d.Today, loc)

	var kb [][]models.InlineKeyboardButton
	for i, it := range append(append([]domain.DigestItem{}, d.Overdue...), d.Today...) {
		if i == digestButtons {
			break
		}
		kb = append(kb, row(btn(shorten(priorityIcon(it.Task.Priority)+it.Task.Title, RichLabelLen),
			cb.New(cb.KindNotifyOpen, it.Task.ID))))
	}
	kb = append(kb, row(btn(l.T("digest.open"), cb.New(cb.KindReopen))))
	return Screen{Text: sb.String(), Keyboard: kb}
}

// QuickAdded confirms tasks created from quick input.
func QuickAdded(l i18n.Localizer, tasks []domain.Task, p domain.Project, c domain.Column, loc *time.Location) Screen {
	var sb strings.Builder
	sb.WriteString(l.T("quick.added", "count", strconv.Itoa(len(tasks)),
		"place", "<b>"+Escape(p.Name)+"</b> › "+Escape(c.Name)))
	for _, t := range tasks {
		line := "\n• " + Escape(priorityIcon(t.Priority)+t.Title)
		if t.DueAt != nil {
			line += " · " + TgDue(t, loc)
		}
		for _, tag := range t.Tags {
			line += " #" + Escape(tag)
		}
		sb.WriteString(line)
	}

	var kb [][]models.InlineKeyboardButton
	if len(tasks) == 1 {
		kb = append(kb, row(
			btn(l.T("btn.open"), cb.New(cb.KindTask, tasks[0].ID)),
			btn(l.T("quick.move"), cb.New(cb.KindTaskProject, tasks[0].ID)),
		))
	}
	kb = append(kb, row(
		btn(l.T("btn.projects"), cb.New(cb.KindHome)),
		btn(l.T("btn.board"), cb.New(cb.KindBoard, c.ID)),
	))
	return Screen{Text: sb.String(), Keyboard: kb}
}
