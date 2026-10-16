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

// Fallback layouts for date_time entities on clients that cannot render them.
const (
	dateLayout = "02.01.2006 15:04"
	dayLayout  = "02.01.2006"
)

// TgTime renders a date_time entity: Telegram shows it in the viewer's locale
// and timezone. The fallback text uses loc for clients without support.
func TgTime(t time.Time, loc *time.Location) string {
	return `<tg-time unix="` + strconv.FormatInt(t.Unix(), 10) + `" format="wdt">` +
		t.In(loc).Format(dateLayout) + `</tg-time>`
}

// TgDate renders a date without a time of day.
func TgDate(t time.Time, loc *time.Location) string {
	return `<tg-time unix="` + strconv.FormatInt(t.Unix(), 10) + `" format="wd">` +
		t.In(loc).Format(dayLayout) + `</tg-time>`
}

// TgRelative renders a time relative to now ("in 1 hour").
func TgRelative(t time.Time, loc *time.Location) string {
	return `<tg-time unix="` + strconv.FormatInt(t.Unix(), 10) + `" format="r">` +
		t.In(loc).Format(dateLayout) + `</tg-time>`
}

// TgDue renders a task deadline: a date for all-day deadlines, otherwise date and time.
func TgDue(t domain.Task, loc *time.Location) string {
	if t.DueAllDay {
		return TgDate(*t.DueAt, loc)
	}
	return TgTime(*t.DueAt, loc)
}

// Card is the data for a task card and its sub-screens.
type Card struct {
	Task    domain.Task
	Project domain.Project
	Column  domain.Column
	// Prev and Next are the neighbouring columns, nil at the board edges.
	Prev, Next *domain.Column
	Items      []domain.ChecklistItem
	Loc        *time.Location
	Now        time.Time
}

func priorityName(l i18n.Localizer, p domain.Priority) string {
	switch p {
	case domain.PriorityLow:
		return l.T("priority.low")
	case domain.PriorityMed:
		return l.T("priority.med")
	case domain.PriorityHigh:
		return l.T("priority.high")
	}
	return l.T("priority.none")
}

func checklistProgress(items []domain.ChecklistItem) string {
	done := 0
	for _, it := range items {
		if it.Done {
			done++
		}
	}
	return strconv.Itoa(done) + "/" + strconv.Itoa(len(items))
}

func checkbox(done bool) string {
	if done {
		return "☑"
	}
	return "☐"
}

// cardHeader is the task title with its location on the board.
func cardHeader(c Card) string {
	return "<b>" + priorityIcon(c.Task.Priority) + Escape(c.Task.Title) + "</b>\n" +
		Escape(c.Project.Name) + " › " + Escape(c.Column.Name)
}

func cardText(l i18n.Localizer, c Card) string {
	t := c.Task
	var sb strings.Builder
	sb.WriteString(cardHeader(c))
	sb.WriteString("\n")

	if t.DueAt != nil {
		sb.WriteString("\n" + l.T("task.due", "date", TgDue(t, c.Loc)))
		if t.IsOverdue(c.Now) {
			sb.WriteString(" ⏰ " + l.T("task.overdue"))
		}
	}
	if t.Priority != domain.PriorityNone {
		sb.WriteString("\n" + l.T("task.priority", "priority", priorityName(l, t.Priority)))
	}
	if len(t.Tags) > 0 {
		tags := make([]string, len(t.Tags))
		for i, tag := range t.Tags {
			tags[i] = "#" + Escape(tag)
		}
		sb.WriteString("\n" + l.T("task.tags", "tags", strings.Join(tags, " ")))
	}
	if t.Description != "" {
		sb.WriteString("\n\n<blockquote expandable>" + Escape(t.Description) + "</blockquote>")
	}
	if len(c.Items) > 0 {
		sb.WriteString("\n\n" + l.T("task.checklist", "progress", checklistProgress(c.Items)))
		for _, it := range c.Items {
			sb.WriteString("\n" + checkbox(it.Done) + " " + Escape(it.Text))
		}
	}
	return sb.String()
}

func backToCard(l i18n.Localizer, t domain.Task) models.InlineKeyboardButton {
	return btn(l.T("btn.card"), cb.New(cb.KindTask, t.ID))
}

// TaskCard renders the task card.
func TaskCard(l i18n.Localizer, c Card) Screen {
	t := c.Task
	var kb [][]models.InlineKeyboardButton

	var move []models.InlineKeyboardButton
	if c.Prev != nil {
		move = append(move, btn("← "+c.Prev.Name, cb.New(cb.KindTaskMove, t.ID, cb.DirLeft)))
	}
	if c.Next != nil {
		move = append(move, btn(c.Next.Name+" →", cb.New(cb.KindTaskMove, t.ID, cb.DirRight)))
	}
	if len(move) > 0 {
		kb = append(kb, move)
	}

	kb = append(kb,
		row(
			btn(l.T("task.edit"), cb.New(cb.KindTaskEdit, t.ID)),
			btn(l.T("task.checklist_btn"), cb.New(cb.KindChecklist, t.ID)),
			btn(l.T("task.reminders_btn"), cb.New(cb.KindReminders, t.ID)),
		),
		row(
			btn(l.T("task.delete"), cb.New(cb.KindTaskDelete, t.ID)),
			btn(l.T("btn.board"), cb.New(cb.KindBoard, t.ColumnID)),
		),
	)
	return Screen{Text: cardText(l, c), Keyboard: kb}
}

// TaskEdit renders the card with the edit menu.
func TaskEdit(l i18n.Localizer, c Card) Screen {
	id := c.Task.ID
	return Screen{Text: cardText(l, c), Keyboard: [][]models.InlineKeyboardButton{
		row(
			btn(l.T("task.edit_title"), cb.New(cb.KindTaskTitle, id)),
			btn(l.T("task.edit_desc"), cb.New(cb.KindTaskDesc, id)),
		),
		row(
			btn(l.T("task.edit_due"), cb.New(cb.KindTaskDue, id)),
			btn(l.T("task.edit_priority"), cb.New(cb.KindTaskPriority, id)),
		),
		row(
			btn(l.T("task.edit_tags"), cb.New(cb.KindTaskTags, id)),
			btn(l.T("task.edit_project"), cb.New(cb.KindTaskProject, id)),
		),
		row(backToCard(l, c.Task)),
	}}
}

// TaskPriority renders the priority picker.
func TaskPriority(l i18n.Localizer, c Card) Screen {
	var opts []models.InlineKeyboardButton
	for _, p := range []domain.Priority{domain.PriorityNone, domain.PriorityLow, domain.PriorityMed, domain.PriorityHigh} {
		label := priorityIcon(p) + priorityName(l, p)
		if p == c.Task.Priority {
			label = "• " + label
		}
		opts = append(opts, btn(label, cb.New(cb.KindTaskPriority, c.Task.ID, int(p))))
	}
	return Screen{Text: cardText(l, c), Keyboard: [][]models.InlineKeyboardButton{
		opts[:2], opts[2:],
		row(btn(l.T("btn.back"), cb.New(cb.KindTaskEdit, c.Task.ID))),
	}}
}

// Checklist renders checklist items as toggle buttons, or as delete buttons in delete mode.
func Checklist(l i18n.Localizer, c Card, deleteMode bool) Screen {
	id := c.Task.ID
	text := cardHeader(c) + "\n\n" + l.T("task.checklist", "progress", checklistProgress(c.Items))
	switch {
	case len(c.Items) == 0:
		text += "\n" + l.T("checklist.empty")
	case deleteMode:
		text += "\n" + l.T("checklist.delete_hint")
	}

	var kb [][]models.InlineKeyboardButton
	for _, it := range c.Items {
		if deleteMode {
			kb = append(kb, row(btn("🗑 "+it.Text, cb.New(cb.KindItemDelete, it.ID))))
		} else {
			kb = append(kb, row(btn(checkbox(it.Done)+" "+it.Text, cb.New(cb.KindItemToggle, it.ID))))
		}
	}

	actions := row(btn(l.T("checklist.add"), cb.New(cb.KindChecklistAdd, id)))
	switch {
	case deleteMode:
		actions = append(actions, btn(l.T("checklist.delete_done"), cb.New(cb.KindChecklist, id)))
	case len(c.Items) > 0:
		actions = append(actions, btn(l.T("checklist.delete_mode"), cb.New(cb.KindChecklist, id, cb.DeleteMode)))
	}
	kb = append(kb, actions, row(backToCard(l, c.Task)))
	return Screen{Text: text, Keyboard: kb}
}

// ChecklistAdd prompts for new checklist items, one per line.
func ChecklistAdd(l i18n.Localizer, c Card) Screen {
	text := cardHeader(c) + "\n\n" + l.T("task.checklist", "progress", checklistProgress(c.Items))
	for _, it := range c.Items {
		text += "\n" + checkbox(it.Done) + " " + Escape(it.Text)
	}
	text += "\n\n" + l.T("checklist.prompt")
	return Screen{Text: text, Keyboard: [][]models.InlineKeyboardButton{
		row(btn(l.T("btn.done"), cb.New(cb.KindChecklist, c.Task.ID))),
	}}
}

// ProjectPicker lists the other active projects to move a task to.
func ProjectPicker(l i18n.Localizer, c Card, projects []domain.ProjectSummary) Screen {
	text := cardHeader(c) + "\n\n" + l.T("task.move_to")
	var kb [][]models.InlineKeyboardButton
	for _, p := range projects {
		if p.ID == c.Task.ProjectID {
			continue
		}
		kb = append(kb, row(btn(projectLabel(p), cb.New(cb.KindTaskProject, c.Task.ID, p.ID))))
	}
	kb = append(kb, row(btn(l.T("btn.back"), cb.New(cb.KindTaskEdit, c.Task.ID))))
	return Screen{Text: text, Keyboard: kb}
}

// Reminders lists the pending reminders of a task with presets to add one.
// The user can also type a time while this screen is shown.
func Reminders(l i18n.Localizer, c Card, reminders []domain.Reminder) Screen {
	var sb strings.Builder
	sb.WriteString(cardHeader(c) + "\n\n" + l.T("reminders.title"))
	if len(reminders) == 0 {
		sb.WriteString("\n" + l.T("reminders.empty"))
	}
	for _, r := range reminders {
		line := "🔔 " + TgTime(r.RemindAt, c.Loc)
		if r.Kind == domain.ReminderDeadline {
			line += " · " + l.T("reminders.deadline")
		}
		sb.WriteString("\n" + line)
	}
	sb.WriteString("\n\n" + l.T("reminders.prompt"))

	id := c.Task.ID
	kb := [][]models.InlineKeyboardButton{row(
		btn(l.T("reminders.in_hour"), cb.New(cb.KindReminders, id, cb.InHour)),
		btn(l.T("reminders.in_3_hours"), cb.New(cb.KindReminders, id, cb.In3Hours)),
		btn(l.T("reminders.tomorrow"), cb.New(cb.KindReminders, id, cb.Tomorrow)),
	)}
	for _, r := range reminders {
		if r.Kind == domain.ReminderCustom {
			kb = append(kb, row(btn("🗑 "+r.RemindAt.In(c.Loc).Format(dateLayout), cb.New(cb.KindReminderDelete, r.ID))))
		}
	}
	kb = append(kb, row(backToCard(l, c.Task)))
	return Screen{Text: sb.String(), Keyboard: kb}
}

// ConfirmDeleteTask asks to confirm task deletion.
func ConfirmDeleteTask(l i18n.Localizer, t domain.Task) Screen {
	return confirm(l, l.T("task.confirm_delete", "title", Escape(t.Title)),
		cb.New(cb.KindTaskDelete, t.ID, cb.Confirm), cb.New(cb.KindTask, t.ID))
}
