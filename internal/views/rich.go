package views

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-telegram/bot/models"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/i18n"
)

// Rich message limits allow up to 500 blocks, counting table rows and list
// items. These caps keep a board well below that.
//
// The visible text is limited to 32768 characters, so task labels are
// shortened and the article stops listing tasks once it reaches textBudget.
const (
	TableRowLimit    = 15  // task rows per table
	ArticleTaskLimit = 400 // list items per article, shared by all columns
	RichLabelLen     = 60  // max runes of a task title in rich views
	textBudget       = 28000
)

// RichBoard is the data for the table and article views: the whole project.
type RichBoard struct {
	Project domain.Project
	Columns []domain.ColumnSummary
	Tasks   []domain.TaskSummary // all tasks in board order
	Now     time.Time
	Loc     *time.Location
}

func (b RichBoard) tasksByColumn() map[int64][]domain.TaskSummary {
	m := make(map[int64][]domain.TaskSummary, len(b.Columns))
	for _, t := range b.Tasks {
		m[t.ColumnID] = append(m[t.ColumnID], t)
	}
	return m
}

// richButton renders an inline callback button inside rich HTML.
// Button text may contain only plain text.
func richButton(label string, data cb.Data) string {
	return `<tg-button type="callback_data" style="link" data="` + Escape(data.String()) + `">` +
		Escape(label) + `</tg-button>`
}

// shorten cuts s to n runes, marking the cut with an ellipsis.
func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func richTaskLabel(t domain.TaskSummary, now time.Time) string {
	s := priorityIcon(t.Priority) + shorten(t.Title, RichLabelLen)
	if t.IsOverdue(now) {
		s += " ⏰"
	}
	return s
}

// viewSwitch is the keyboard row to switch between board views; current is marked.
func viewSwitch(l i18n.Localizer, columnID int64, current domain.ViewMode) []models.InlineKeyboardButton {
	var r []models.InlineKeyboardButton
	for _, v := range domain.Views {
		label := l.T("view." + string(v))
		if v == current {
			label = "• " + label
		}
		r = append(r, btn(label, cb.New(cb.KindView, columnID, v.Code())))
	}
	return r
}

// richKeyboard is shared by the table and article views.
func richKeyboard(l i18n.Localizer, b RichBoard, current domain.ViewMode) [][]models.InlineKeyboardButton {
	first := b.Columns[0].ID
	return [][]models.InlineKeyboardButton{
		row(btn(l.T("board.new_task"), cb.New(cb.KindTaskNew, first))),
		viewSwitch(l, first, current),
		row(
			btn(l.T("btn.projects"), cb.New(cb.KindHome)),
			btn(l.T("board.manage"), cb.New(cb.KindProjectManage, b.Project.ID)),
		),
	}
}

func columnTitle(c domain.ColumnSummary) string {
	return Escape(c.Name) + " · " + strconv.Itoa(c.Tasks)
}

// BoardTable renders the project as a table: kanban columns are table columns,
// cells are task buttons. Long columns end with a "more" button that opens the
// column in the buttons view.
func BoardTable(l i18n.Localizer, b RichBoard) Screen {
	byCol := b.tasksByColumn()

	var sb strings.Builder
	sb.WriteString("<h3>" + Escape(b.Project.Name) + "</h3>")
	sb.WriteString("<table bordered striped compact><tr>")
	rows := 0
	for _, c := range b.Columns {
		sb.WriteString(`<th>` + columnTitle(c) + `</th>`)
		rows = max(rows, len(byCol[c.ID]))
	}
	sb.WriteString("</tr>")

	overflow := rows > TableRowLimit
	if overflow {
		rows = TableRowLimit - 1 // keep the last row for "more" buttons
	}
	for i := range rows {
		sb.WriteString("<tr>")
		for _, c := range b.Columns {
			sb.WriteString("<td>")
			if tasks := byCol[c.ID]; i < len(tasks) {
				sb.WriteString(richButton(richTaskLabel(tasks[i], b.Now), cb.New(cb.KindTask, tasks[i].ID)))
			}
			sb.WriteString("</td>")
		}
		sb.WriteString("</tr>")
	}
	if overflow {
		sb.WriteString("<tr>")
		for _, c := range b.Columns {
			sb.WriteString("<td>")
			if hidden := len(byCol[c.ID]) - rows; hidden > 0 {
				sb.WriteString(richButton(l.T("board.more", "count", hidden), cb.New(cb.KindView, c.ID, domain.ViewButtons.Code())))
			}
			sb.WriteString("</td>")
		}
		sb.WriteString("</tr>")
	}
	sb.WriteString("</table>")
	if len(b.Tasks) == 0 {
		sb.WriteString("<p><i>" + Escape(l.T("board.empty")) + "</i></p>")
	}

	return Screen{Rich: sb.String(), Keyboard: richKeyboard(l, b, domain.ViewTable)}
}

// BoardArticle renders the project as a document: a section per column with a
// task list; done columns are collapsed. Tasks beyond the per-column share of
// ArticleTaskLimit or the text budget are replaced by a "more" button.
func BoardArticle(l i18n.Localizer, b RichBoard) Screen {
	byCol := b.tasksByColumn()
	perColumn := ArticleTaskLimit / len(b.Columns)
	budget := textBudget

	var sb strings.Builder
	sb.WriteString("<h2>" + Escape(b.Project.Name) + "</h2>")
	for _, c := range b.Columns {
		var section strings.Builder
		tasks := byCol[c.ID]
		if len(tasks) == 0 {
			section.WriteString("<p><i>" + Escape(l.T("board.empty")) + "</i></p>")
		} else {
			section.WriteString("<ul>")
			for i, t := range tasks {
				item, size := articleItem(t, b)
				if i == perColumn || size > budget {
					section.WriteString("<li>" + richButton(l.T("board.more", "count", len(tasks)-i),
						cb.New(cb.KindView, c.ID, domain.ViewButtons.Code())) + "</li>")
					break
				}
				budget -= size
				section.WriteString("<li>" + item + "</li>")
			}
			section.WriteString("</ul>")
		}

		if c.IsDone {
			sb.WriteString("<details><summary>" + columnTitle(c) + "</summary>" + section.String() + "</details>")
		} else {
			sb.WriteString("<h4>" + columnTitle(c) + "</h4>" + section.String())
		}
	}
	return Screen{Rich: sb.String(), Keyboard: richKeyboard(l, b, domain.ViewArticle)}
}

// articleItem is a task button followed by its deadline, tags and checklist
// progress. It also returns the approximate visible length in runes.
func articleItem(t domain.TaskSummary, b RichBoard) (string, int) {
	label := richTaskLabel(t, b.Now)
	size := utf8.RuneCountInString(label)
	parts := []string{richButton(label, cb.New(cb.KindTask, t.ID))}
	if t.DueAt != nil {
		size += 32 // rendered date and time in the user's locale
		parts = append(parts, TgTime(*t.DueAt, b.Loc))
	}
	if len(t.Tags) > 0 {
		tags := strings.Join(t.Tags, " #")
		size += utf8.RuneCountInString(tags) + 1
		parts = append(parts, "#"+Escape(tags))
	}
	if t.ChecklistTotal > 0 {
		progress := "☑ " + strconv.Itoa(t.ChecklistDone) + "/" + strconv.Itoa(t.ChecklistTotal)
		size += utf8.RuneCountInString(progress)
		parts = append(parts, progress)
	}
	return strings.Join(parts, " · "), size + 3*(len(parts)-1)
}
