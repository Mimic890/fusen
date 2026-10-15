package views

import (
	"strconv"

	"github.com/go-telegram/bot/models"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/i18n"
)

func columnLabel(c domain.ColumnSummary) string {
	s := c.Name + " · " + strconv.Itoa(c.Tasks)
	if c.IsDone {
		s = "✅ " + s
	}
	return s
}

// Columns renders the column settings of a project.
func Columns(l i18n.Localizer, p domain.Project, cols []domain.ColumnSummary) Screen {
	text := "<b>" + Escape(p.Name) + "</b> › " + l.T("columns.title") + "\n" + l.T("columns.hint")
	var kb [][]models.InlineKeyboardButton
	for _, c := range cols {
		kb = append(kb, row(btn(columnLabel(c), cb.New(cb.KindColumn, c.ID))))
	}
	kb = append(kb,
		row(btn(l.T("columns.new"), cb.New(cb.KindColumnNew, p.ID))),
		row(btn(l.T("btn.back"), cb.New(cb.KindProjectManage, p.ID))),
	)
	return Screen{Text: text, Keyboard: kb}
}

// Column renders the settings of one column. first and last mark its board position.
func Column(l i18n.Localizer, p domain.Project, c domain.ColumnSummary, first, last bool) Screen {
	text := "<b>" + Escape(p.Name) + "</b> › " + Escape(c.Name) + "\n" + l.T("board.count", "count", c.Tasks)
	if c.IsDone {
		text += "\n" + l.T("column.is_done")
	}

	var move []models.InlineKeyboardButton
	if !first {
		move = append(move, btn(l.T("column.move_left"), cb.New(cb.KindColumnMove, c.ID, cb.DirLeft)))
	}
	if !last {
		move = append(move, btn(l.T("column.move_right"), cb.New(cb.KindColumnMove, c.ID, cb.DirRight)))
	}

	done := btn(l.T("column.mark_done"), cb.New(cb.KindColumnDone, c.ID, "1"))
	if c.IsDone {
		done = btn(l.T("column.unmark_done"), cb.New(cb.KindColumnDone, c.ID, "0"))
	}

	var kb [][]models.InlineKeyboardButton
	if len(move) > 0 {
		kb = append(kb, move)
	}
	kb = append(kb,
		row(btn(l.T("column.rename"), cb.New(cb.KindColumnRename, c.ID)), done),
		row(btn(l.T("column.delete"), cb.New(cb.KindColumnDelete, c.ID))),
		row(btn(l.T("btn.back"), cb.New(cb.KindColumns, p.ID))),
	)
	return Screen{Text: text, Keyboard: kb}
}

// ConfirmDeleteColumn asks to confirm column deletion.
func ConfirmDeleteColumn(l i18n.Localizer, c domain.Column) Screen {
	return confirm(l, l.T("column.confirm_delete", "name", Escape(c.Name)),
		cb.New(cb.KindColumnDelete, c.ID, cb.Confirm), cb.New(cb.KindColumn, c.ID))
}
