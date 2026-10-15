package bot

import (
	"context"
	"strconv"
	"time"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/views"
)

// loadCard gathers everything needed to render a task card.
func (b *Bot) loadCard(ctx context.Context, r *request, taskID int64) (views.Card, error) {
	t, err := b.store.GetTask(ctx, r.user.TgID, taskID)
	if err != nil {
		return views.Card{}, err
	}
	p, err := b.store.GetProject(ctx, r.user.TgID, t.ProjectID)
	if err != nil {
		return views.Card{}, err
	}
	cols, err := b.store.ListColumns(ctx, p.ID)
	if err != nil {
		return views.Card{}, err
	}
	items, err := b.store.ListChecklist(ctx, t.ID)
	if err != nil {
		return views.Card{}, err
	}

	c := views.Card{Task: t, Project: p, Items: items, Loc: r.user.Location(), Now: time.Now()}
	idx := columnIndex(cols, t.ColumnID)
	if idx < 0 {
		return c, domain.ErrNotFound
	}
	c.Column = cols[idx].Column
	if idx > 0 {
		c.Prev = &cols[idx-1].Column
	}
	if idx < len(cols)-1 {
		c.Next = &cols[idx+1].Column
	}
	return c, nil
}

// showTask renders a task screen built by render from the loaded card.
func (b *Bot) showTask(ctx context.Context, r *request, taskID int64, notice string, render func(views.Card) views.Screen) error {
	c, err := b.loadCard(ctx, r, taskID)
	if err != nil {
		return err
	}
	return b.show(ctx, r, render(c).WithNotice(notice))
}

func (b *Bot) cardView(r *request) func(views.Card) views.Screen {
	return func(c views.Card) views.Screen { return views.TaskCard(r.l, c) }
}

func (b *Bot) editView(r *request) func(views.Card) views.Screen {
	return func(c views.Card) views.Screen { return views.TaskEdit(r.l, c) }
}

func (b *Bot) checklistView(r *request, deleteMode bool) func(views.Card) views.Screen {
	return func(c views.Card) views.Screen {
		return views.Checklist(r.l, c, deleteMode && len(c.Items) > 0)
	}
}

// taskArg returns the task ID from the first callback argument.
func taskArg(r *request) (int64, error) {
	return r.data.ID(0)
}

func (b *Bot) onTask(ctx context.Context, r *request) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	return b.showTask(ctx, r, id, "", b.cardView(r))
}

func (b *Bot) taskNewPrompt(r *request, c domain.Column) views.Screen {
	return views.Prompt(r.l, r.l.T("task.prompt_new", "column", views.Escape(c.Name)), cb.New(cb.KindBoard, c.ID))
}

func (b *Bot) onTaskNew(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	c, err := b.store.GetColumn(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	if err := b.setState(ctx, r, cb.New(cb.KindTaskNew, c.ID)); err != nil {
		return err
	}
	return b.show(ctx, r, b.taskNewPrompt(r, c))
}

func (b *Bot) onTaskMove(ctx context.Context, r *request) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	target, err := b.store.MoveTask(ctx, r.user.TgID, id, dirArg(r.data.Arg(1)))
	if err != nil {
		return err
	}
	r.toast = r.l.T("task.moved", "column", target.Name)
	return b.showTask(ctx, r, id, "", b.cardView(r))
}

func (b *Bot) onTaskEdit(ctx context.Context, r *request) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	return b.showTask(ctx, r, id, "", b.editView(r))
}

// taskPrompt asks for a task field. A Clear button is shown when clearable.
func (b *Bot) taskPrompt(r *request, t domain.Task, key string, clear *cb.Data) views.Screen {
	text := r.l.T(key, "title", views.Escape(t.Title))
	cancel := cb.New(cb.KindTaskEdit, t.ID)
	if clear != nil {
		return views.Prompt(r.l, text, cancel, views.Button(r.l.T("btn.clear"), *clear))
	}
	return views.Prompt(r.l, text, cancel)
}

func (b *Bot) titlePrompt(r *request, t domain.Task) views.Screen {
	return b.taskPrompt(r, t, "task.prompt_title", nil)
}

func (b *Bot) descPrompt(r *request, t domain.Task) views.Screen {
	var clear *cb.Data
	if t.Description != "" {
		d := cb.New(cb.KindTaskDesc, t.ID, cb.Clear)
		clear = &d
	}
	return b.taskPrompt(r, t, "task.prompt_desc", clear)
}

func (b *Bot) tagsPrompt(r *request, t domain.Task) views.Screen {
	var clear *cb.Data
	if len(t.Tags) > 0 {
		d := cb.New(cb.KindTaskTags, t.ID, cb.Clear)
		clear = &d
	}
	return b.taskPrompt(r, t, "task.prompt_tags", clear)
}

// promptTask sets the pending input for a task field and shows its prompt.
func (b *Bot) promptTask(ctx context.Context, r *request, kind string, prompt func(*request, domain.Task) views.Screen) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	t, err := b.store.GetTask(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	if err := b.setState(ctx, r, cb.New(kind, t.ID)); err != nil {
		return err
	}
	return b.show(ctx, r, prompt(r, t))
}

func (b *Bot) onTaskTitle(ctx context.Context, r *request) error {
	return b.promptTask(ctx, r, cb.KindTaskTitle, b.titlePrompt)
}

func (b *Bot) onTaskDesc(ctx context.Context, r *request) error {
	if r.data.Arg(1) != cb.Clear {
		return b.promptTask(ctx, r, cb.KindTaskDesc, b.descPrompt)
	}
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	if err := b.store.SetTaskDescription(ctx, r.user.TgID, id, ""); err != nil {
		return err
	}
	r.toast = r.l.T("task.updated")
	return b.showTask(ctx, r, id, "", b.editView(r))
}

func (b *Bot) onTaskTags(ctx context.Context, r *request) error {
	if r.data.Arg(1) != cb.Clear {
		return b.promptTask(ctx, r, cb.KindTaskTags, b.tagsPrompt)
	}
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	if err := b.store.SetTaskTags(ctx, r.user.TgID, id, nil); err != nil {
		return err
	}
	r.toast = r.l.T("task.updated")
	return b.showTask(ctx, r, id, "", b.editView(r))
}

func (b *Bot) onTaskPriority(ctx context.Context, r *request) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	if r.data.Arg(1) == "" {
		return b.showTask(ctx, r, id, "", func(c views.Card) views.Screen { return views.TaskPriority(r.l, c) })
	}
	p := domain.Priority(r.data.Int(1))
	if p < domain.PriorityNone || p > domain.PriorityHigh {
		return domain.ErrNotFound
	}
	if err := b.store.SetTaskPriority(ctx, r.user.TgID, id, p); err != nil {
		return err
	}
	r.toast = r.l.T("task.updated")
	return b.showTask(ctx, r, id, "", b.cardView(r))
}

func (b *Bot) onTaskDelete(ctx context.Context, r *request) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	t, err := b.store.GetTask(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	if r.data.Arg(1) != cb.Confirm {
		return b.show(ctx, r, views.ConfirmDeleteTask(r.l, t))
	}
	if err := b.store.DeleteTask(ctx, r.user.TgID, id); err != nil {
		return err
	}
	r.toast = r.l.T("task.deleted")
	return b.showBoard(ctx, r, t.ColumnID, 0, "")
}

// Checklist.

func (b *Bot) onChecklist(ctx context.Context, r *request) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	return b.showTask(ctx, r, id, "", b.checklistView(r, r.data.Arg(1) == cb.DeleteMode))
}

func (b *Bot) checklistAddView(r *request) func(views.Card) views.Screen {
	return func(c views.Card) views.Screen { return views.ChecklistAdd(r.l, c) }
}

func (b *Bot) onChecklistAdd(ctx context.Context, r *request) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	if _, err := b.store.GetTask(ctx, r.user.TgID, id); err != nil {
		return err
	}
	if err := b.setState(ctx, r, cb.New(cb.KindChecklistAdd, id)); err != nil {
		return err
	}
	return b.showTask(ctx, r, id, "", b.checklistAddView(r))
}

func (b *Bot) onItemToggle(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	taskID, err := b.store.ToggleChecklistItem(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	return b.showTask(ctx, r, taskID, "", b.checklistView(r, false))
}

func (b *Bot) onItemDelete(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	taskID, err := b.store.DeleteChecklistItem(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	return b.showTask(ctx, r, taskID, "", b.checklistView(r, true))
}

// Text input.

// normalizeAll applies norm to every line, failing on the first invalid one.
func normalizeAll(lines []string, norm func(string) (string, error)) ([]string, error) {
	if len(lines) == 0 {
		return nil, domain.ErrEmptyName
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		v, err := norm(l)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (b *Bot) inputTaskNew(ctx context.Context, r *request, text string) error {
	columnID, err := r.data.ID(0)
	if err != nil {
		return err
	}
	c, err := b.store.GetColumn(ctx, r.user.TgID, columnID)
	if err != nil {
		return err
	}
	// Board input understands the quick-add syntax too: #tags, !priority, !deadline.
	now := time.Now()
	tasks, err := parseLines(text, now, r.user.Location())
	if isInputError(err) {
		return b.reprompt(ctx, r, b.taskNewPrompt(r, c), err)
	} else if err != nil {
		return err
	}
	if _, err := b.store.AddTasks(ctx, r.user.TgID, c.ID, tasks, r.user.RemindBeforeMin, now); err != nil {
		return err
	}
	if err := b.clearState(ctx, r); err != nil {
		return err
	}
	return b.showBoard(ctx, r, c.ID, lastPage, r.l.T("task.created", "count", strconv.Itoa(len(tasks))))
}

// inputTaskField handles text input for a single task field.
func (b *Bot) inputTaskField(ctx context.Context, r *request, text string,
	prompt func(*request, domain.Task) views.Screen,
	save func(ctx context.Context, t domain.Task, text string) error,
) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	t, err := b.store.GetTask(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	if err := save(ctx, t, text); isInputError(err) {
		return b.reprompt(ctx, r, prompt(r, t), err)
	} else if err != nil {
		return err
	}
	if err := b.clearState(ctx, r); err != nil {
		return err
	}
	return b.showTask(ctx, r, t.ID, r.l.T("task.updated"), b.cardView(r))
}

func (b *Bot) inputTaskTitle(ctx context.Context, r *request, text string) error {
	return b.inputTaskField(ctx, r, text, b.titlePrompt, func(ctx context.Context, t domain.Task, text string) error {
		title, err := domain.NormalizeTitle(text)
		if err != nil {
			return err
		}
		return b.store.SetTaskTitle(ctx, r.user.TgID, t.ID, title)
	})
}

func (b *Bot) inputTaskDesc(ctx context.Context, r *request, text string) error {
	return b.inputTaskField(ctx, r, text, b.descPrompt, func(ctx context.Context, t domain.Task, text string) error {
		desc, err := domain.NormalizeDescription(text)
		if err != nil {
			return err
		}
		return b.store.SetTaskDescription(ctx, r.user.TgID, t.ID, desc)
	})
}

func (b *Bot) inputTaskTags(ctx context.Context, r *request, text string) error {
	return b.inputTaskField(ctx, r, text, b.tagsPrompt, func(ctx context.Context, t domain.Task, text string) error {
		tags, err := domain.ParseTags(text)
		if err != nil {
			return err
		}
		return b.store.SetTaskTags(ctx, r.user.TgID, t.ID, tags)
	})
}

// inputChecklistAdd adds items and keeps the prompt open for more.
func (b *Bot) inputChecklistAdd(ctx context.Context, r *request, text string) error {
	id, err := taskArg(r)
	if err != nil {
		return err
	}
	items, err := normalizeAll(domain.SplitLines(text), domain.NormalizeItem)
	if err == nil {
		err = b.store.AddChecklistItems(ctx, r.user.TgID, id, items)
	}
	notice := ""
	if isInputError(err) {
		notice = b.userError(r, err)
	} else if err != nil {
		return err
	}
	return b.showTask(ctx, r, id, notice, b.checklistAddView(r))
}
