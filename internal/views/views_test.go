package views

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/i18n"
	"github.com/Mimic890/fusen/locales"
)

func localizer(t *testing.T, lang string) i18n.Localizer {
	t.Helper()
	b, err := i18n.Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	return b.For(lang)
}

// checkScreen verifies that every button carries valid callback data.
func checkScreen(t *testing.T, s Screen) {
	t.Helper()
	if s.Text == "" {
		t.Error("empty text")
	}
	for _, r := range s.Keyboard {
		for _, b := range r {
			if b.Text == "" {
				t.Error("button without text")
			}
			if _, err := cb.Decode(b.CallbackData); err != nil {
				t.Errorf("button %q: %v", b.Text, err)
			}
		}
	}
}

func TestScreensRender(t *testing.T) {
	const bigID = math.MaxInt64
	inbox := domain.ProjectSummary{Project: domain.Project{ID: 1, Name: "Inbox", IsInbox: true}, OpenTasks: 3}
	proj := domain.Project{ID: bigID, Name: "<Home & lab>"}
	cols := []domain.ColumnSummary{
		{Column: domain.Column{ID: 10, Name: "Todo"}, Tasks: 20},
		{Column: domain.Column{ID: bigID, Name: "Done", IsDone: true}},
	}
	due := time.Now().Add(-time.Hour)
	task := domain.Task{ID: bigID, ColumnID: 10, Title: "<t>", Priority: domain.PriorityHigh,
		DueAt: &due, Description: "a\nb", Tags: []string{"home"}}
	items := []domain.ChecklistItem{{ID: bigID, Text: "one", Done: true}, {ID: 2, Text: "two"}}
	card := Card{Task: task, Project: proj, Column: cols[0].Column, Next: &cols[1].Column,
		Items: items, Loc: time.UTC, Now: time.Now()}
	tasks := []domain.TaskSummary{{Task: task, ChecklistDone: 1, ChecklistTotal: 2}}

	for _, lang := range []string{"en", "ru"} {
		l := localizer(t, lang)
		archived := proj
		archived.Archived = true

		screens := map[string]Screen{
			"home":        Home(l, []domain.ProjectSummary{inbox, {Project: proj}}, domain.DueSummary{Overdue: 1}, 2),
			"home empty":  Home(l, nil, domain.DueSummary{}, 0),
			"archive":     Archive(l, []domain.ProjectSummary{{Project: archived}}),
			"project":     Project(l, proj, cols),
			"archived":    Project(l, archived, cols),
			"inbox":       Project(l, inbox.Project, cols),
			"confirm":     ConfirmDeleteProject(l, proj),
			"prompt":      Prompt(l, "x", cb.New(cb.KindHome)),
			"stale":       Stale(l),
			"with notice": Stale(l).WithNotice("<ok>"),
			"board":       BoardButtons(l, Board{Project: proj, Columns: cols, Index: 0, Tasks: tasks, Page: 1, Now: time.Now()}),
			"board last":  BoardButtons(l, Board{Project: proj, Columns: cols, Index: 1}),
			"card":        TaskCard(l, card),
			"card bare":   TaskCard(l, Card{Task: domain.Task{ID: 1, Title: "x"}, Loc: time.UTC}),
			"edit":        TaskEdit(l, card),
			"priority":    TaskPriority(l, card),
			"checklist":   Checklist(l, card, false),
			"checklist d": Checklist(l, card, true),
			"checklist 0": Checklist(l, Card{Task: task, Loc: time.UTC}, false),
			"items add":   ChecklistAdd(l, card),
			"task delete": ConfirmDeleteTask(l, task),
			"columns":     Columns(l, proj, cols),
			"column":      Column(l, proj, cols[0], true, false),
			"column done": Column(l, proj, cols[1], false, true),
			"col delete":  ConfirmDeleteColumn(l, cols[0].Column),
		}
		for name, s := range screens {
			t.Run(lang+"/"+name, func(t *testing.T) { checkScreen(t, s) })
		}
	}
}

func TestProjectEscapesAndProtectsInbox(t *testing.T) {
	l := localizer(t, "en")
	s := Project(l, domain.Project{ID: 2, Name: "<b>x</b>"}, nil)
	if strings.Contains(s.Text, "<b>x</b>") {
		t.Error("project name is not escaped")
	}

	inbox := Project(l, domain.Project{ID: 1, Name: "Inbox", IsInbox: true}, nil)
	for _, r := range inbox.Keyboard {
		for _, b := range r {
			d, _ := cb.Decode(b.CallbackData)
			if d.Kind == cb.KindProjectArchive || d.Kind == cb.KindProjectDelete {
				t.Errorf("inbox offers %q", b.Text)
			}
		}
	}
}

func TestHomeHidesEmptyArchive(t *testing.T) {
	l := localizer(t, "en")
	s := Home(l, nil, domain.DueSummary{}, 0)
	for _, r := range s.Keyboard {
		for _, b := range r {
			if b.CallbackData == cb.KindArchive {
				t.Error("archive button shown with no archived projects")
			}
		}
	}
}

func TestBoardPagination(t *testing.T) {
	l := localizer(t, "en")
	cols := []domain.ColumnSummary{{Column: domain.Column{ID: 1, Name: "Todo"}, Tasks: PageSize*2 + 1}}
	s := BoardButtons(l, Board{Project: domain.Project{ID: 1, Name: "P"}, Columns: cols, Page: 1})

	var labels []string
	for _, r := range s.Keyboard {
		for _, b := range r {
			labels = append(labels, b.Text)
		}
	}
	if !slices.Contains(labels, "2/3") || !slices.Contains(labels, "‹") || !slices.Contains(labels, "›") {
		t.Errorf("pagination row missing: %v", labels)
	}

	if got := ClampPage(lastPageForTest, PageSize*2+1); got != 2 {
		t.Errorf("ClampPage(last) = %d", got)
	}
	if got := ClampPage(-1, 0); got != 0 {
		t.Errorf("ClampPage(-1, 0) = %d", got)
	}
}

const lastPageForTest = 1 << 30

func TestCardMoveButtonsFollowEdges(t *testing.T) {
	l := localizer(t, "en")
	todo := domain.Column{ID: 1, Name: "Todo"}
	done := domain.Column{ID: 2, Name: "Done"}
	c := Card{Task: domain.Task{ID: 5, Title: "x"}, Column: todo, Next: &done, Loc: time.UTC}

	moves := func(s Screen) []string {
		var out []string
		for _, r := range s.Keyboard {
			for _, b := range r {
				if d, _ := cb.Decode(b.CallbackData); d.Kind == cb.KindTaskMove {
					out = append(out, d.Arg(1))
				}
			}
		}
		return out
	}
	if got := moves(TaskCard(l, c)); !slices.Equal(got, []string{cb.DirRight}) {
		t.Errorf("first column moves = %v", got)
	}
	c.Column, c.Prev, c.Next = done, &todo, nil
	if got := moves(TaskCard(l, c)); !slices.Equal(got, []string{cb.DirLeft}) {
		t.Errorf("last column moves = %v", got)
	}
}

func TestBoardHeaderCounts(t *testing.T) {
	l := localizer(t, "en")
	cols := []domain.ColumnSummary{
		{Column: domain.Column{ID: 1, Name: "Todo"}, Tasks: 3},
		{Column: domain.Column{ID: 2, Name: "Done"}, Tasks: 7},
	}
	s := BoardButtons(l, Board{Project: domain.Project{ID: 1, Name: "P"}, Columns: cols, Index: 1})
	if !strings.HasPrefix(s.Text, "<b>P</b> › Done (7 / 10)") {
		t.Errorf("header: %q", s.Text)
	}
}
