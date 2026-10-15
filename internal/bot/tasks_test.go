package bot

import (
	"context"
	"slices"
	"strings"
	"testing"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/views"
)

// boardHarness opens the live screen and returns the inbox with its columns.
func boardHarness(t *testing.T) (*harness, domain.ProjectSummary, []domain.ColumnSummary) {
	t.Helper()
	h := newHarness(t)
	h.message("/start", 500)
	ctx := context.Background()
	ps, _ := h.store.ListProjects(ctx, userID, false)
	cols, _ := h.store.ListColumns(ctx, ps[0].ID)
	h.api.take()
	return h, ps[0], cols
}

// lastEdit returns the text and keyboard of the last live screen edit.
func lastEdit(t *testing.T, calls []apiCall) (text, markup string) {
	t.Helper()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].method == "editMessageText" {
			return calls[i].params["text"], calls[i].params["reply_markup"]
		}
	}
	t.Fatalf("no edit in %s", methods(calls))
	return "", ""
}

func TestCreateTasksFromBoard(t *testing.T) {
	h, inbox, cols := boardHarness(t)
	ctx := context.Background()
	live := h.live()

	h.press(live, cb.New(cb.KindProject, inbox.ID))
	text, markup := lastEdit(t, h.api.take())
	if !strings.Contains(text, "Входящие</b> › К выполнению (0 / 0)") || !strings.Contains(markup, "В работе ›") {
		t.Fatalf("board: %q %q", text, markup)
	}
	if u, _ := h.store.GetUser(ctx, userID); u.CurrentProjectID != inbox.ID {
		t.Errorf("current project = %d", u.CurrentProjectID)
	}

	h.press(live, cb.New(cb.KindTaskNew, cols[0].ID))
	var lines []string
	for i := range views.PageSize + 1 {
		lines = append(lines, "task "+string(rune('a'+i)))
	}
	h.message(strings.Join(lines, "\n"), 600)
	calls := h.api.take()
	text, markup = lastEdit(t, calls)
	if !strings.Contains(text, "К выполнению (9 / 9)") || !strings.Contains(markup, "2/2") || !strings.Contains(markup, "task i") {
		t.Fatalf("board after create should show the last page: %q %q", text, markup)
	}
	if calls[len(calls)-1].method != "deleteMessage" {
		t.Error("user input not deleted")
	}

	// Too long titles keep the prompt open.
	h.press(live, cb.New(cb.KindTaskNew, cols[0].ID))
	h.message(strings.Repeat("x", domain.MaxTitleLen+1), 601)
	if u, _ := h.store.GetUser(ctx, userID); u.State == "" {
		t.Error("state cleared after invalid title")
	}
}

func TestTaskCardFlow(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	live := h.live()
	tasks, _ := h.store.CreateTasks(ctx, userID, cols[0].ID, []string{"buy cable"})
	id := tasks[0].ID

	h.press(live, cb.New(cb.KindTask, id))
	text, markup := lastEdit(t, h.api.take())
	if !strings.Contains(text, "buy cable") || !strings.Contains(markup, "В работе →") || strings.Contains(markup, "← ") {
		t.Fatalf("card: %q %q", text, markup)
	}

	h.press(live, cb.New(cb.KindTaskMove, id, cb.DirRight))
	h.press(live, cb.New(cb.KindTaskMove, id, cb.DirRight))
	got, _ := h.store.GetTask(ctx, userID, id)
	if got.ColumnID != cols[2].ID || got.DoneAt == nil {
		t.Fatalf("after moves: %+v", got)
	}
	calls := h.api.take()
	if lastText(calls, "answerCallbackQuery") == "" {
		t.Error("move must show a toast")
	}

	h.press(live, cb.New(cb.KindTaskTitle, id))
	h.message("buy HDMI cable", 601)
	h.press(live, cb.New(cb.KindTaskDesc, id))
	h.message("  2 m long\nblack  ", 602)
	h.press(live, cb.New(cb.KindTaskTags, id))
	h.message("#homelab, #Shop", 603)
	h.press(live, cb.New(cb.KindTaskPriority, id, int(domain.PriorityHigh)))

	got, _ = h.store.GetTask(ctx, userID, id)
	if got.Title != "buy HDMI cable" || got.Description != "2 m long\nblack" ||
		!slices.Equal(got.Tags, []string{"homelab", "shop"}) || got.Priority != domain.PriorityHigh {
		t.Fatalf("task = %+v", got)
	}
	text, _ = lastEdit(t, h.api.take())
	if !strings.Contains(text, "#homelab #shop") || !strings.Contains(text, "Высокий") {
		t.Errorf("card text: %q", text)
	}

	h.press(live, cb.New(cb.KindTaskDesc, id, cb.Clear))
	h.press(live, cb.New(cb.KindTaskTags, id, cb.Clear))
	if got, _ = h.store.GetTask(ctx, userID, id); got.Description != "" || len(got.Tags) != 0 {
		t.Fatalf("not cleared: %+v", got)
	}

	h.press(live, cb.New(cb.KindTaskDelete, id))
	if _, err := h.store.GetTask(ctx, userID, id); err != nil {
		t.Fatal("deleted without confirmation")
	}
	h.press(live, cb.New(cb.KindTaskDelete, id, cb.Confirm))
	if _, err := h.store.GetTask(ctx, userID, id); err == nil {
		t.Fatal("not deleted")
	}
	text, _ = lastEdit(t, h.api.take())
	if !strings.Contains(text, "› Готово") {
		t.Errorf("after delete should return to the task's column: %q", text)
	}
}

func TestChecklistFlow(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	live := h.live()
	tasks, _ := h.store.CreateTasks(ctx, userID, cols[0].ID, []string{"t"})
	id := tasks[0].ID

	h.press(live, cb.New(cb.KindChecklistAdd, id))
	h.message("one\ntwo", 601)
	h.message("three", 602)
	items, _ := h.store.ListChecklist(ctx, id)
	if len(items) != 3 {
		t.Fatalf("items = %+v", items)
	}
	if u, _ := h.store.GetUser(ctx, userID); u.State == "" {
		t.Error("checklist prompt must stay open for more items")
	}

	h.press(live, cb.New(cb.KindItemToggle, items[0].ID))
	if u, _ := h.store.GetUser(ctx, userID); u.State != "" {
		t.Error("pressing a button must end the input")
	}
	h.press(live, cb.New(cb.KindItemDelete, items[1].ID))
	items, _ = h.store.ListChecklist(ctx, id)
	if len(items) != 2 || !items[0].Done {
		t.Fatalf("items = %+v", items)
	}

	page, _, _ := h.store.ListColumnTasks(ctx, cols[0].ID, 0, 10)
	h.press(live, cb.New(cb.KindBoard, cols[0].ID))
	_, markup := lastEdit(t, h.api.take())
	if page[0].ChecklistTotal != 2 || !strings.Contains(markup, "t · 1/2") {
		t.Errorf("board label: %q", markup)
	}
}

func TestColumnSettingsFlow(t *testing.T) {
	h, inbox, cols := boardHarness(t)
	ctx := context.Background()
	live := h.live()

	h.press(live, cb.New(cb.KindColumnNew, inbox.ID))
	h.message("Review", 601)
	got, _ := h.store.ListColumns(ctx, inbox.ID)
	if len(got) != 4 || got[3].Name != "Review" {
		t.Fatalf("columns = %+v", got)
	}
	review := got[3].ID

	h.press(live, cb.New(cb.KindColumnMove, review, cb.DirLeft))
	h.press(live, cb.New(cb.KindColumnRename, review))
	h.message("QA", 602)
	got, _ = h.store.ListColumns(ctx, inbox.ID)
	if got[2].ID != review || got[2].Name != "QA" {
		t.Fatalf("columns = %+v", got)
	}

	h.store.CreateTasks(ctx, userID, cols[0].ID, []string{"x"})
	h.api.take()
	h.press(live, cb.New(cb.KindColumnDelete, cols[0].ID))
	if lastText(h.api.take(), "answerCallbackQuery") == "" {
		t.Error("deleting a non-empty column must explain why")
	}

	h.press(live, cb.New(cb.KindColumnDelete, review))
	h.press(live, cb.New(cb.KindColumnDelete, review, cb.Confirm))
	if got, _ = h.store.ListColumns(ctx, inbox.ID); len(got) != 3 {
		t.Fatalf("columns after delete = %+v", got)
	}

	h.press(live, cb.New(cb.KindColumnDone, cols[1].ID, "1"))
	if c, _ := h.store.GetColumn(ctx, userID, cols[1].ID); !c.IsDone {
		t.Error("column not marked done")
	}
}

func TestForeignTaskIsHidden(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	tasks, _ := h.store.CreateTasks(ctx, userID, cols[0].ID, []string{"mine"})

	// Another user cannot open the task through crafted callback data.
	const intruder = 2002
	h.messageAs(intruder, "/start", 700)
	u, _ := h.store.GetUser(ctx, intruder)
	h.api.take()

	h.pressAs(intruder, u.LiveMessageID, cb.New(cb.KindTask, tasks[0].ID))
	calls := h.api.take()
	if strings.Contains(methods(calls), "editMessageText") {
		t.Errorf("foreign task rendered: %s", methods(calls))
	}
	if lastText(calls, "answerCallbackQuery") == "" {
		t.Error("expected a not-found toast")
	}
}
