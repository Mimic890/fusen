package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Mimic890/fusen/internal/domain"
)

// setupBoard creates user 1 with a project and returns it with its columns.
func setupBoard(t *testing.T) (*Store, domain.Project, []domain.ColumnSummary) {
	t.Helper()
	s := openTest(t)
	newUser(t, s, 1)
	newUser(t, s, 2)
	p, err := s.CreateProject(context.Background(), 1, "A")
	if err != nil {
		t.Fatal(err)
	}
	cols, err := s.ListColumns(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, p, cols
}

func TestCreateAndListTasks(t *testing.T) {
	s, _, cols := setupBoard(t)
	ctx := context.Background()
	todo := cols[0].ID

	created, err := s.CreateTasks(ctx, 1, todo, []string{"a", "b", "c"})
	if err != nil || len(created) != 3 {
		t.Fatalf("create: %v %v", created, err)
	}
	if _, err := s.CreateTasks(ctx, 2, todo, []string{"x"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign create: %v", err)
	}

	page, total, err := s.ListColumnTasks(ctx, todo, 1, 1)
	if err != nil || total != 3 || len(page) != 1 || page[0].Title != "b" {
		t.Fatalf("page: %+v total=%d err=%v", page, total, err)
	}

	// New tasks in a done column are done right away.
	done, _ := s.CreateTasks(ctx, 1, cols[2].ID, []string{"d"})
	got, _ := s.GetTask(ctx, 1, done[0].ID)
	if got.DoneAt == nil {
		t.Error("task in done column has no done_at")
	}
}

func TestMoveTask(t *testing.T) {
	s, _, cols := setupBoard(t)
	ctx := context.Background()
	tasks, _ := s.CreateTasks(ctx, 1, cols[0].ID, []string{"a"})
	id := tasks[0].ID

	if _, err := s.MoveTask(ctx, 1, id, domain.Left); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("move past left edge: %v", err)
	}
	for _, want := range []int64{cols[1].ID, cols[2].ID} {
		c, err := s.MoveTask(ctx, 1, id, domain.Right)
		if err != nil || c.ID != want {
			t.Fatalf("move right: %+v %v", c, err)
		}
	}
	got, _ := s.GetTask(ctx, 1, id)
	if got.ColumnID != cols[2].ID || got.DoneAt == nil {
		t.Fatalf("after moving to done: %+v", got)
	}
	if _, err := s.MoveTask(ctx, 1, id, domain.Right); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("move past right edge: %v", err)
	}
	if _, err := s.MoveTask(ctx, 1, id, domain.Left); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetTask(ctx, 1, id); got.DoneAt != nil {
		t.Error("done_at kept after leaving done column")
	}
	if _, err := s.MoveTask(ctx, 2, id, domain.Left); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign move: %v", err)
	}
}

func TestTaskFieldsAndTags(t *testing.T) {
	s, _, cols := setupBoard(t)
	ctx := context.Background()
	tasks, _ := s.CreateTasks(ctx, 1, cols[0].ID, []string{"a"})
	id := tasks[0].ID

	if err := s.SetTaskTitle(ctx, 1, id, "b"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTaskDescription(ctx, 1, id, "line1\nline2"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTaskPriority(ctx, 1, id, domain.PriorityHigh); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTaskTags(ctx, 1, id, []string{"work", "home"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTask(ctx, 1, id)
	if got.Title != "b" || got.Description != "line1\nline2" || got.Priority != domain.PriorityHigh ||
		!slices.Equal(got.Tags, []string{"home", "work"}) {
		t.Fatalf("task = %+v", got)
	}

	if err := s.SetTaskTags(ctx, 1, id, []string{"home"}); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetTask(ctx, 1, id); !slices.Equal(got.Tags, []string{"home"}) {
		t.Fatalf("tags = %v", got.Tags)
	}
	if err := s.SetTaskTitle(ctx, 2, id, "hijack"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign update: %v", err)
	}
	if _, err := s.GetTask(ctx, 2, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign get: %v", err)
	}

	if err := s.DeleteTask(ctx, 1, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTask(ctx, 1, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("deleted task still found: %v", err)
	}
}

func TestChecklist(t *testing.T) {
	s, _, cols := setupBoard(t)
	ctx := context.Background()
	tasks, _ := s.CreateTasks(ctx, 1, cols[0].ID, []string{"a"})
	id := tasks[0].ID

	if err := s.AddChecklistItems(ctx, 1, id, []string{"one", "two"}); err != nil {
		t.Fatal(err)
	}
	items, _ := s.ListChecklist(ctx, id)
	if len(items) != 2 || items[0].Text != "one" {
		t.Fatalf("items = %+v", items)
	}

	if tid, err := s.ToggleChecklistItem(ctx, 1, items[0].ID); err != nil || tid != id {
		t.Fatalf("toggle: %d %v", tid, err)
	}
	page, _, _ := s.ListColumnTasks(ctx, cols[0].ID, 0, 10)
	if page[0].ChecklistDone != 1 || page[0].ChecklistTotal != 2 {
		t.Errorf("progress = %d/%d", page[0].ChecklistDone, page[0].ChecklistTotal)
	}

	if _, err := s.ToggleChecklistItem(ctx, 2, items[0].ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign toggle: %v", err)
	}
	if _, err := s.DeleteChecklistItem(ctx, 1, items[1].ID); err != nil {
		t.Fatal(err)
	}
	if items, _ = s.ListChecklist(ctx, id); len(items) != 1 || !items[0].Done {
		t.Fatalf("items = %+v", items)
	}

	many := make([]string, domain.MaxChecklistItems)
	for i := range many {
		many[i] = "x"
	}
	if err := s.AddChecklistItems(ctx, 1, id, many); !errors.Is(err, domain.ErrTooManyItems) {
		t.Errorf("limit: %v", err)
	}
}

func TestColumns(t *testing.T) {
	s, p, cols := setupBoard(t)
	ctx := context.Background()

	c, err := s.CreateColumn(ctx, 1, p.ID, "Review")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateColumn(ctx, 2, p.ID, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign create: %v", err)
	}

	// Move Review left of Done.
	if err := s.MoveColumn(ctx, 1, c.ID, domain.Left); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveColumn(ctx, 1, cols[0].ID, domain.Left); err != nil {
		t.Fatal("moving past the edge must be a no-op")
	}
	got, _ := s.ListColumns(ctx, p.ID)
	names := []string{got[0].Name, got[1].Name, got[2].Name, got[3].Name}
	if !slices.Equal(names, []string{"Todo", "In progress", "Review", "Done"}) {
		t.Fatalf("order = %v", names)
	}

	if err := s.RenameColumn(ctx, 1, c.ID, "QA"); err != nil {
		t.Fatal(err)
	}

	tasks, _ := s.CreateTasks(ctx, 1, c.ID, []string{"a"})
	if err := s.SetColumnDone(ctx, 1, c.ID, true); err != nil {
		t.Fatal(err)
	}
	if task, _ := s.GetTask(ctx, 1, tasks[0].ID); task.DoneAt == nil {
		t.Error("done_at not set by column toggle")
	}
	if err := s.DeleteColumn(ctx, 1, c.ID); !errors.Is(err, domain.ErrColumnNotEmpty) {
		t.Errorf("delete non-empty: %v", err)
	}
	_ = s.DeleteTask(ctx, 1, tasks[0].ID)
	if err := s.DeleteColumn(ctx, 1, c.ID); err != nil {
		t.Fatal(err)
	}

	for _, col := range cols[1:] {
		if err := s.DeleteColumn(ctx, 1, col.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteColumn(ctx, 1, cols[0].ID); !errors.Is(err, domain.ErrLastColumn) {
		t.Errorf("delete last: %v", err)
	}
}
