package views

import (
	"strconv"
	"time"

	"github.com/go-telegram/bot/models"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/i18n"
)

// PageSize is the number of tasks per board page.
const PageSize = 8

// Board is the data for one column of a project in the buttons view.
type Board struct {
	Project domain.Project
	Columns []domain.ColumnSummary
	Index   int // index of the shown column in Columns
	Tasks   []domain.TaskSummary
	Page    int
	Now     time.Time
}

// Pages returns the number of pages of the shown column (at least 1).
func (b Board) Pages() int {
	return Pages(b.Columns[b.Index].Tasks)
}

// Pages returns the page count for n tasks (at least 1).
func Pages(n int) int {
	return max(1, (n+PageSize-1)/PageSize)
}

// ClampPage limits page to the valid range for n tasks.
func ClampPage(page, n int) int {
	return min(max(page, 0), Pages(n)-1)
}

func priorityIcon(p domain.Priority) string {
	switch p {
	case domain.PriorityHigh:
		return "🔴 "
	case domain.PriorityMed:
		return "🟠 "
	case domain.PriorityLow:
		return "🔵 "
	}
	return ""
}

func taskLabel(t domain.TaskSummary, now time.Time) string {
	s := priorityIcon(t.Priority) + t.Title
	if t.ChecklistTotal > 0 {
		s += " · " + strconv.Itoa(t.ChecklistDone) + "/" + strconv.Itoa(t.ChecklistTotal)
	}
	if t.IsOverdue(now) {
		s += " ⏰"
	}
	return s
}

// BoardButtons renders a project column with task buttons and column navigation.
func BoardButtons(l i18n.Localizer, b Board) Screen {
	col := b.Columns[b.Index]
	text := "<b>" + Escape(b.Project.Name) + "</b> › " + Escape(col.Name) +
		" (" + strconv.Itoa(b.Index+1) + "/" + strconv.Itoa(len(b.Columns)) + ")\n"
	if col.Tasks == 0 {
		text += l.T("board.empty")
	} else {
		text += l.T("board.count", "count", col.Tasks)
	}

	var kb [][]models.InlineKeyboardButton
	for _, t := range b.Tasks {
		kb = append(kb, row(btn(taskLabel(t, b.Now), cb.New(cb.KindTask, t.ID))))
	}

	if pages := b.Pages(); pages > 1 {
		var nav []models.InlineKeyboardButton
		if b.Page > 0 {
			nav = append(nav, btn("‹", cb.New(cb.KindBoard, col.ID, b.Page-1)))
		}
		nav = append(nav, btn(strconv.Itoa(b.Page+1)+"/"+strconv.Itoa(pages), cb.New(cb.KindNoop)))
		if b.Page < pages-1 {
			nav = append(nav, btn("›", cb.New(cb.KindBoard, col.ID, b.Page+1)))
		}
		kb = append(kb, nav)
	}

	kb = append(kb, row(btn(l.T("board.new_task"), cb.New(cb.KindTaskNew, col.ID))))

	var cols []models.InlineKeyboardButton
	if b.Index > 0 {
		prev := b.Columns[b.Index-1]
		cols = append(cols, btn("‹ "+prev.Name, cb.New(cb.KindBoard, prev.ID)))
	}
	if b.Index < len(b.Columns)-1 {
		next := b.Columns[b.Index+1]
		cols = append(cols, btn(next.Name+" ›", cb.New(cb.KindBoard, next.ID)))
	}
	if len(cols) > 0 {
		kb = append(kb, cols)
	}

	kb = append(kb, row(
		btn(l.T("btn.projects"), cb.New(cb.KindHome)),
		btn(l.T("board.manage"), cb.New(cb.KindProjectManage, b.Project.ID)),
	))
	return Screen{Text: text, Keyboard: kb}
}
