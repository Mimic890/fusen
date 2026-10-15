package views

import (
	"math"
	"strings"
	"testing"

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
	cols := []domain.Column{{Name: "Todo"}, {Name: "Done", IsDone: true}}

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
