package bot

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/i18n"
	"github.com/Mimic890/fusen/locales"
)

// fixture holds IDs of one user's objects.
type fixture struct {
	project, column, task, item, reminder int64
	emptyColumn                           int64
}

// makeFixture opens the live screen for uid and creates a project with a task,
// a checklist item and a custom reminder.
func makeFixture(t *testing.T, h *harness, uid int64) fixture {
	t.Helper()
	ctx := context.Background()
	h.messageAs(uid, "/start", 500)
	p, err := h.store.CreateProject(ctx, uid, "Fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	cols, _ := h.store.ListColumns(ctx, p.ID)
	tasks, _ := h.store.CreateTasks(ctx, uid, cols[0].ID, []string{"task"})
	_ = h.store.AddChecklistItems(ctx, uid, tasks[0].ID, []string{"item"})
	items, _ := h.store.ListChecklist(ctx, tasks[0].ID)
	_ = h.store.AddReminder(ctx, uid, tasks[0].ID, time.Now().Add(time.Hour))
	rs, _ := h.store.ListReminders(ctx, tasks[0].ID)
	h.api.take()
	return fixture{project: p.ID, column: cols[0].ID, task: tasks[0].ID, item: items[0].ID, reminder: rs[0].ID,
		emptyColumn: cols[len(cols)-1].ID}
}

// routeArgs builds valid callback arguments for every route from a fixture.
// A nil entry means the route takes no IDs.
var routeArgs = map[string]func(f fixture) []any{
	cb.KindNoop:           nil,
	cb.KindHome:           nil,
	cb.KindArchive:        nil,
	cb.KindProject:        func(f fixture) []any { return []any{f.project} },
	cb.KindProjectManage:  func(f fixture) []any { return []any{f.project} },
	cb.KindProjectNew:     nil,
	cb.KindProjectRename:  func(f fixture) []any { return []any{f.project} },
	cb.KindProjectArchive: func(f fixture) []any { return []any{f.project, "1"} },
	cb.KindProjectDelete:  func(f fixture) []any { return []any{f.project} },
	cb.KindBoard:          func(f fixture) []any { return []any{f.column} },
	cb.KindView:           func(f fixture) []any { return []any{f.column, domain.ViewTable.Code()} },
	cb.KindColumns:        func(f fixture) []any { return []any{f.project} },
	cb.KindColumn:         func(f fixture) []any { return []any{f.column} },
	cb.KindColumnNew:      func(f fixture) []any { return []any{f.project} },
	cb.KindColumnRename:   func(f fixture) []any { return []any{f.column} },
	cb.KindColumnMove:     func(f fixture) []any { return []any{f.column, cb.DirRight} },
	cb.KindColumnDone:     func(f fixture) []any { return []any{f.column, "1"} },
	cb.KindColumnDelete:   func(f fixture) []any { return []any{f.emptyColumn} },
	cb.KindTaskNew:        func(f fixture) []any { return []any{f.column} },
	cb.KindTask:           func(f fixture) []any { return []any{f.task} },
	cb.KindTaskMove:       func(f fixture) []any { return []any{f.task, cb.DirRight} },
	cb.KindTaskEdit:       func(f fixture) []any { return []any{f.task} },
	cb.KindTaskTitle:      func(f fixture) []any { return []any{f.task} },
	cb.KindTaskDesc:       func(f fixture) []any { return []any{f.task} },
	cb.KindTaskTags:       func(f fixture) []any { return []any{f.task} },
	cb.KindTaskPriority:   func(f fixture) []any { return []any{f.task} },
	cb.KindTaskDelete:     func(f fixture) []any { return []any{f.task} },
	cb.KindTaskDue:        func(f fixture) []any { return []any{f.task} },
	cb.KindTaskProject:    func(f fixture) []any { return []any{f.task} },
	cb.KindReminders:      func(f fixture) []any { return []any{f.task} },
	cb.KindReminderDelete: func(f fixture) []any { return []any{f.reminder} },
	cb.KindReminderAction: func(f fixture) []any { return []any{f.reminder, cb.ActHour} },
	cb.KindNotifyOpen:     func(f fixture) []any { return []any{f.task} },
	cb.KindSettings:       nil,
	cb.KindSetLang:        nil,
	cb.KindSetView:        nil,
	cb.KindTimezone:       nil,
	cb.KindDigest:         nil,
	cb.KindRemind:         nil,
	cb.KindChecklist:      func(f fixture) []any { return []any{f.task} },
	cb.KindChecklistAdd:   func(f fixture) []any { return []any{f.task} },
	cb.KindItemToggle:     func(f fixture) []any { return []any{f.item} },
	cb.KindItemDelete:     func(f fixture) []any { return []any{f.item} },
}

// plainArgs are arguments of routes without IDs.
var plainArgs = map[string][]any{
	cb.KindSetLang: {domain.LangEN},
	cb.KindSetView: {domain.ViewArticle.Code()},
}

func TestEveryKindIsRoutedAndCovered(t *testing.T) {
	src, err := os.ReadFile("../callback/kinds.go")
	if err != nil {
		t.Fatal(err)
	}
	kinds := regexp.MustCompile(`(?m)^\s*Kind\w+\s*=\s*"(\w+)"`).FindAllStringSubmatch(string(src), -1)
	if len(kinds) < 40 {
		t.Fatalf("found only %d kinds", len(kinds))
	}
	h := newHarness(t)
	for _, m := range kinds {
		kind := m[1]
		if kind == cb.KindReopen {
			continue // handled before routing
		}
		if _, ok := h.bot.routes[kind]; !ok {
			t.Errorf("kind %q has no route", kind)
		}
		if _, ok := routeArgs[kind]; !ok {
			t.Errorf("kind %q is missing in routeArgs", kind)
		}
	}
}

func messages(t *testing.T, lang string) i18n.Localizer {
	t.Helper()
	tr, err := i18n.Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	return tr.For(lang)
}

// pressedMessage returns the message a button of kind would be on: the live
// screen, or a notification for notification kinds.
func pressedMessage(h *harness, kind string) int {
	if notificationKinds[kind] {
		return 9999
	}
	return h.live()
}

func TestEveryRouteRenders(t *testing.T) {
	generic := messages(t, "ru").T("error.generic")
	for kind, args := range routeArgs {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t)
			f := makeFixture(t, h, userID)
			var a []any
			if args != nil {
				a = args(f)
			} else {
				a = plainArgs[kind]
			}
			h.press(pressedMessage(h, kind), cb.New(kind, a...))
			calls := h.api.take()
			if toast := lastText(calls, "answerCallbackQuery"); toast == generic {
				t.Fatalf("generic error for %s", cb.New(kind, a...))
			}
			if kind != cb.KindNoop && !strings.Contains(methods(calls), "editMessageText") &&
				!strings.Contains(methods(calls), "sendMessage") && !strings.Contains(methods(calls), "sendRichMessage") {
				t.Errorf("nothing shown: %s", methods(calls))
			}
		})
	}
}

func TestEveryRouteRejectsForeignIDs(t *testing.T) {
	notFound := messages(t, "ru").T("error.not_found")
	for kind, args := range routeArgs {
		if args == nil {
			continue
		}
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t)
			makeFixture(t, h, userID)
			foreign := makeFixture(t, h, 2002)
			data := cb.New(kind, args(foreign)...)

			h.press(pressedMessage(h, kind), data)
			calls := h.api.take()
			if got := lastText(calls, "answerCallbackQuery"); got != notFound {
				t.Errorf("%s: toast %q, want not found", data, got)
			}
			for _, c := range calls {
				if c.method != "answerCallbackQuery" {
					t.Errorf("%s: unexpected %s on another user's object", data, c.method)
				}
			}
		})
	}
}

func TestMalformedCallbackData(t *testing.T) {
	h := newHarness(t)
	makeFixture(t, h, userID)
	for _, data := range []cb.Data{
		cb.New(cb.KindTask),               // missing ID
		cb.New(cb.KindTask, "!!"),         // not base36
		cb.New(cb.KindTaskPriority, 1, 9), // out of range
		cb.New(cb.KindView, 1, "z"),       // unknown view
		cb.New(cb.KindDigest, "9999"),     // invalid time
		cb.New("zz"),                      // unknown kind
	} {
		h.press(h.live(), data)
		for _, c := range h.api.take() {
			if c.method != "answerCallbackQuery" {
				t.Errorf("%s: unexpected %s", data, c.method)
			}
		}
	}
}
