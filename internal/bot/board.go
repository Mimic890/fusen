package bot

import (
	"context"
	"math"
	"time"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/views"
)

// lastPage requests the last page of a column; it is clamped to the real page count.
const lastPage = math.MaxInt32

func columnIndex(cols []domain.ColumnSummary, id int64) int {
	for i, c := range cols {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func dirArg(arg string) domain.Direction {
	if arg == cb.DirLeft {
		return domain.Left
	}
	return domain.Right
}

// showBoard shows the board column with the given ID.
func (b *Bot) showBoard(ctx context.Context, r *request, columnID int64, page int, notice string) error {
	c, err := b.store.GetColumn(ctx, r.user.TgID, columnID)
	if err != nil {
		return err
	}
	p, err := b.store.GetProject(ctx, r.user.TgID, c.ProjectID)
	if err != nil {
		return err
	}
	return b.showBoardOf(ctx, r, p, c.ID, page, notice)
}

// showBoardOf shows project p in the user's view. columnID (0 = first column)
// and page apply to the buttons view.
func (b *Bot) showBoardOf(ctx context.Context, r *request, p domain.Project, columnID int64, page int, notice string) error {
	cols, err := b.store.ListColumns(ctx, p.ID)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return domain.ErrNotFound
	}
	if !p.Archived {
		if err := b.store.SetCurrentProject(ctx, r.user.TgID, p.ID); err != nil {
			return err
		}
	}

	var s views.Screen
	switch r.user.ViewMode {
	case domain.ViewTable, domain.ViewArticle:
		tasks, err := b.store.ListProjectTasks(ctx, p.ID)
		if err != nil {
			return err
		}
		rb := views.RichBoard{Project: p, Columns: cols, Tasks: tasks, Now: time.Now(), Loc: r.user.Location()}
		if r.user.ViewMode == domain.ViewTable {
			s = views.BoardTable(r.l, rb)
		} else {
			s = views.BoardArticle(r.l, rb)
		}
	default:
		idx := max(columnIndex(cols, columnID), 0)
		page = views.ClampPage(page, cols[idx].Tasks)
		tasks, _, err := b.store.ListColumnTasks(ctx, cols[idx].ID, page*views.PageSize, views.PageSize)
		if err != nil {
			return err
		}
		s = views.BoardButtons(r.l, views.Board{Project: p, Columns: cols, Index: idx, Tasks: tasks, Page: page, Now: time.Now()})
	}
	return b.show(ctx, r, s.WithNotice(notice))
}

func (b *Bot) onBoard(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	return b.showBoard(ctx, r, id, r.data.Int(1), "")
}

// onView switches the board view and remembers it as the user's preference.
func (b *Bot) onView(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	v, ok := domain.ParseViewCode(r.data.Arg(1))
	if !ok {
		return domain.ErrNotFound
	}
	if v != r.user.ViewMode {
		if err := b.store.SetViewMode(ctx, r.user.TgID, v); err != nil {
			return err
		}
		r.user.ViewMode = v
	}
	return b.showBoard(ctx, r, id, 0, "")
}

// Column settings.

func (b *Bot) showColumns(ctx context.Context, r *request, projectID int64, notice string) error {
	p, err := b.store.GetProject(ctx, r.user.TgID, projectID)
	if err != nil {
		return err
	}
	cols, err := b.store.ListColumns(ctx, p.ID)
	if err != nil {
		return err
	}
	return b.show(ctx, r, views.Columns(r.l, p, cols).WithNotice(notice))
}

// loadColumn returns a column with its project, siblings and index among them.
func (b *Bot) loadColumn(ctx context.Context, r *request, id int64) (domain.Project, []domain.ColumnSummary, int, error) {
	c, err := b.store.GetColumn(ctx, r.user.TgID, id)
	if err != nil {
		return domain.Project{}, nil, 0, err
	}
	p, err := b.store.GetProject(ctx, r.user.TgID, c.ProjectID)
	if err != nil {
		return p, nil, 0, err
	}
	cols, err := b.store.ListColumns(ctx, p.ID)
	if err != nil {
		return p, nil, 0, err
	}
	idx := columnIndex(cols, id)
	if idx < 0 {
		return p, nil, 0, domain.ErrNotFound
	}
	return p, cols, idx, nil
}

func (b *Bot) showColumn(ctx context.Context, r *request, id int64, notice string) error {
	p, cols, idx, err := b.loadColumn(ctx, r, id)
	if err != nil {
		return err
	}
	s := views.Column(r.l, p, cols[idx], idx == 0, idx == len(cols)-1)
	return b.show(ctx, r, s.WithNotice(notice))
}

func (b *Bot) onColumns(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	return b.showColumns(ctx, r, id, "")
}

func (b *Bot) onColumn(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	return b.showColumn(ctx, r, id, "")
}

func (b *Bot) columnNewPrompt(r *request, projectID int64) views.Screen {
	return views.Prompt(r.l, r.l.T("column.prompt_new"), cb.New(cb.KindColumns, projectID))
}

func (b *Bot) onColumnNew(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	p, err := b.store.GetProject(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	if err := b.setState(ctx, r, cb.New(cb.KindColumnNew, p.ID)); err != nil {
		return err
	}
	return b.show(ctx, r, b.columnNewPrompt(r, p.ID))
}

func (b *Bot) columnRenamePrompt(r *request, c domain.Column) views.Screen {
	return views.Prompt(r.l, r.l.T("column.prompt_rename", "name", views.Escape(c.Name)), cb.New(cb.KindColumn, c.ID))
}

func (b *Bot) onColumnRename(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	c, err := b.store.GetColumn(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	if err := b.setState(ctx, r, cb.New(cb.KindColumnRename, c.ID)); err != nil {
		return err
	}
	return b.show(ctx, r, b.columnRenamePrompt(r, c))
}

func (b *Bot) onColumnMove(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	if err := b.store.MoveColumn(ctx, r.user.TgID, id, dirArg(r.data.Arg(1))); err != nil {
		return err
	}
	return b.showColumn(ctx, r, id, "")
}

func (b *Bot) onColumnDone(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	if err := b.store.SetColumnDone(ctx, r.user.TgID, id, r.data.Arg(1) == "1"); err != nil {
		return err
	}
	return b.showColumn(ctx, r, id, "")
}

func (b *Bot) onColumnDelete(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	p, cols, idx, err := b.loadColumn(ctx, r, id)
	if err != nil {
		return err
	}
	if r.data.Arg(1) == cb.Confirm {
		if err := b.store.DeleteColumn(ctx, r.user.TgID, id); err != nil {
			return err
		}
		r.toast = r.l.T("column.deleted")
		return b.showColumns(ctx, r, p.ID, "")
	}

	// Check up front so the user is not asked to confirm an impossible action.
	switch {
	case cols[idx].Tasks > 0:
		return domain.ErrColumnNotEmpty
	case len(cols) == 1:
		return domain.ErrLastColumn
	}
	return b.show(ctx, r, views.ConfirmDeleteColumn(r.l, cols[idx].Column))
}

func (b *Bot) inputColumnNew(ctx context.Context, r *request, text string) error {
	projectID, err := r.data.ID(0)
	if err != nil {
		return err
	}
	name, err := domain.NormalizeName(text)
	if isInputError(err) {
		return b.reprompt(ctx, r, b.columnNewPrompt(r, projectID), err)
	}
	if _, err := b.store.CreateColumn(ctx, r.user.TgID, projectID, name); err != nil {
		return err
	}
	if err := b.clearState(ctx, r); err != nil {
		return err
	}
	return b.showColumns(ctx, r, projectID, r.l.T("column.created"))
}

func (b *Bot) inputColumnRename(ctx context.Context, r *request, text string) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	c, err := b.store.GetColumn(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	name, err := domain.NormalizeName(text)
	if isInputError(err) {
		return b.reprompt(ctx, r, b.columnRenamePrompt(r, c), err)
	}
	if err := b.store.RenameColumn(ctx, r.user.TgID, c.ID, name); err != nil {
		return err
	}
	if err := b.clearState(ctx, r); err != nil {
		return err
	}
	return b.showColumn(ctx, r, c.ID, r.l.T("project.renamed"))
}
