// Package views renders bot screens as text and inline keyboards.
package views

import (
	"html"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/i18n"
)

// Screen is a rendered message: HTML text plus an inline keyboard.
type Screen struct {
	Text     string
	Keyboard [][]models.InlineKeyboardButton
}

// Markup returns the inline keyboard markup for the screen.
func (s Screen) Markup() *models.InlineKeyboardMarkup {
	kb := s.Keyboard
	if kb == nil {
		kb = [][]models.InlineKeyboardButton{}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: kb}
}

// WithNotice prepends a short status line to the screen.
func (s Screen) WithNotice(notice string) Screen {
	if notice != "" {
		s.Text = "<i>" + html.EscapeString(notice) + "</i>\n\n" + s.Text
	}
	return s
}

func btn(text string, data cb.Data) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: text, CallbackData: data.String()}
}

func row(buttons ...models.InlineKeyboardButton) []models.InlineKeyboardButton {
	return buttons
}

// Escape escapes user-provided text for HTML parse mode.
func Escape(s string) string { return html.EscapeString(s) }

func projectLabel(p domain.ProjectSummary) string {
	name := p.Name
	if p.IsInbox {
		name = "📥 " + name
	}
	return name + " · " + strconv.Itoa(p.OpenTasks)
}

// Home renders the project list with the due summary.
func Home(l i18n.Localizer, projects []domain.ProjectSummary, due domain.DueSummary, archived int) Screen {
	var sb strings.Builder
	sb.WriteString(l.T("home.title"))
	sb.WriteString("\n")
	if due.Overdue == 0 && due.Today == 0 {
		sb.WriteString(l.T("home.summary_empty"))
	} else {
		sb.WriteString(l.T("home.summary", "overdue", due.Overdue, "today", due.Today))
	}

	var kb [][]models.InlineKeyboardButton
	for _, p := range projects {
		kb = append(kb, row(btn(projectLabel(p), cb.New(cb.KindProject, p.ID))))
	}
	last := row(btn(l.T("home.new_project"), cb.New(cb.KindProjectNew)))
	if archived > 0 {
		last = append(last, btn(l.T("home.archive", "count", archived), cb.New(cb.KindArchive)))
	}
	kb = append(kb, last)
	return Screen{Text: sb.String(), Keyboard: kb}
}

// Archive renders the list of archived projects.
func Archive(l i18n.Localizer, projects []domain.ProjectSummary) Screen {
	text := l.T("archive.title")
	if len(projects) == 0 {
		text += "\n" + l.T("archive.empty")
	}
	var kb [][]models.InlineKeyboardButton
	for _, p := range projects {
		kb = append(kb, row(btn(projectLabel(p), cb.New(cb.KindProject, p.ID))))
	}
	kb = append(kb, row(btn(l.T("btn.projects"), cb.New(cb.KindHome))))
	return Screen{Text: text, Keyboard: kb}
}

// Project renders the project management screen.
func Project(l i18n.Localizer, p domain.Project, cols []domain.Column) Screen {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = Escape(c.Name)
	}

	var sb strings.Builder
	sb.WriteString("<b>" + Escape(p.Name) + "</b>\n")
	if p.Archived {
		sb.WriteString(l.T("project.archived_note") + "\n")
	}
	sb.WriteString(l.T("project.columns", "columns", strings.Join(names, " · ")))

	kb := [][]models.InlineKeyboardButton{
		row(btn(l.T("project.rename"), cb.New(cb.KindProjectRename, p.ID))),
	}
	back := cb.New(cb.KindHome)
	if !p.IsInbox {
		archive := btn(l.T("project.archive"), cb.New(cb.KindProjectArchive, p.ID, "1"))
		if p.Archived {
			archive = btn(l.T("project.unarchive"), cb.New(cb.KindProjectArchive, p.ID, "0"))
			back = cb.New(cb.KindArchive)
		}
		kb = append(kb, row(archive, btn(l.T("project.delete"), cb.New(cb.KindProjectDelete, p.ID))))
	}
	kb = append(kb, row(btn(l.T("btn.back"), back)))
	return Screen{Text: sb.String(), Keyboard: kb}
}

// ConfirmDeleteProject asks to confirm project deletion.
func ConfirmDeleteProject(l i18n.Localizer, p domain.Project) Screen {
	return Screen{
		Text: l.T("project.confirm_delete", "name", Escape(p.Name)),
		Keyboard: [][]models.InlineKeyboardButton{row(
			btn(l.T("project.confirm_yes"), cb.New(cb.KindProjectDelete, p.ID, cb.Confirm)),
			btn(l.T("btn.cancel"), cb.New(cb.KindProject, p.ID)),
		)},
	}
}

// Prompt asks the user to send text, with a cancel button leading to cancel.
func Prompt(l i18n.Localizer, text string, cancel cb.Data) Screen {
	return Screen{
		Text:     text,
		Keyboard: [][]models.InlineKeyboardButton{row(btn(l.T("btn.cancel"), cancel))},
	}
}

// Stale replaces an outdated live screen.
func Stale(l i18n.Localizer) Screen {
	return Screen{
		Text:     l.T("stale.text"),
		Keyboard: [][]models.InlineKeyboardButton{row(btn(l.T("btn.open"), cb.New(cb.KindReopen)))},
	}
}
