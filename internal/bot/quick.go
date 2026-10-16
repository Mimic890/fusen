package bot

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/parser"
	"github.com/Mimic890/fusen/internal/views"
)

// parseLines parses every non-empty line of text as a quick-add task.
func parseLines(text string, now time.Time, loc *time.Location) ([]domain.NewTask, error) {
	lines := domain.SplitLines(text)
	if len(lines) == 0 {
		return nil, domain.ErrEmptyName
	}
	out := make([]domain.NewTask, len(lines))
	for i, line := range lines {
		res, err := parser.Parse(line, now, loc)
		if err != nil {
			return nil, err
		}
		out[i] = res.Task()
	}
	return out, nil
}

// quickTarget returns the project quick input goes to: the open project, or the inbox.
func (b *Bot) quickTarget(ctx context.Context, r *request) (domain.Project, error) {
	if id := r.user.CurrentProjectID; id != 0 {
		p, err := b.store.GetProject(ctx, r.user.TgID, id)
		if err == nil && !p.Archived {
			return p, nil
		}
	}
	return b.store.GetInbox(ctx, r.user.TgID)
}

// onQuickAdd turns a plain message into tasks in the first column of the target project.
func (b *Bot) onQuickAdd(ctx context.Context, r *request, m *models.Message) {
	now := time.Now()
	tasks, err := parseLines(m.Text, now, r.user.Location())
	if err == nil {
		err = b.quickAdd(ctx, r, tasks, now)
	}
	if err != nil {
		// Keep the message so the text is not lost; explain on the live screen.
		if err := b.showHome(ctx, r, b.userError(r, err)); err != nil {
			b.log.Error("show home", "user_id", r.user.TgID, "err", err)
		}
		return
	}
	b.dropMessage(ctx, r, m.ID)
}

func (b *Bot) quickAdd(ctx context.Context, r *request, tasks []domain.NewTask, now time.Time) error {
	p, err := b.quickTarget(ctx, r)
	if err != nil {
		return err
	}
	cols, err := b.store.ListColumns(ctx, p.ID)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return domain.ErrNotFound
	}
	created, err := b.store.AddTasks(ctx, r.user.TgID, cols[0].ID, tasks, r.user.RemindBeforeMin, now)
	if err != nil {
		return err
	}
	return b.show(ctx, r, views.QuickAdded(r.l, created, p, cols[0].Column, r.user.Location()))
}

// Deadline.

func (b *Bot) duePrompt(r *request, t domain.Task) views.Screen {
	var clear *cb.Data
	if t.DueAt != nil {
		d := cb.New(cb.KindTaskDue, t.ID, cb.Clear)
		clear = &d
	}
	return b.taskPrompt(r, t, "task.prompt_due", clear)
}

func (b *Bot) onTaskDue(ctx context.Context, r *request) error {
	if r.data.Arg(1) != cb.Clear {
		return b.promptTask(ctx, r, cb.KindTaskDue, b.duePrompt)
	}
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	if err := b.store.SetTaskDue(ctx, r.user.TgID, id, nil, false, r.user.RemindBeforeMin, time.Now()); err != nil {
		return err
	}
	r.toast = r.l.T("task.updated")
	return b.showTask(ctx, r, id, "", b.editView(r))
}

func (b *Bot) inputTaskDue(ctx context.Context, r *request, text string) error {
	return b.inputTaskField(ctx, r, text, b.duePrompt, func(ctx context.Context, t domain.Task, text string) error {
		now := time.Now()
		due, allDay, err := parser.ParseDue(text, now, r.user.Location())
		if err != nil {
			return err
		}
		return b.store.SetTaskDue(ctx, r.user.TgID, t.ID, &due, allDay, r.user.RemindBeforeMin, now)
	})
}

// Move to another project.

func (b *Bot) onTaskProject(ctx context.Context, r *request) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	if r.data.Arg(1) == "" {
		projects, err := b.store.ListProjects(ctx, r.user.TgID, false)
		if err != nil {
			return err
		}
		return b.showTask(ctx, r, id, "", func(c views.Card) views.Screen {
			return views.ProjectPicker(r.l, c, projects)
		})
	}
	projectID, err := r.data.ID(1)
	if err != nil {
		return err
	}
	if _, err := b.store.MoveTaskToProject(ctx, r.user.TgID, id, projectID, time.Now()); err != nil {
		return err
	}
	p, err := b.store.GetProject(ctx, r.user.TgID, projectID)
	if err != nil {
		return err
	}
	r.toast = r.l.T("task.moved_project", "project", p.Name)
	return b.showTask(ctx, r, id, "", b.cardView(r))
}
