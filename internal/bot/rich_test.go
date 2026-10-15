package bot

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
)

// richHTML returns the rich HTML of an API call, or "" for plain text calls.
func richHTML(t *testing.T, c apiCall) string {
	t.Helper()
	raw, ok := c.params["rich_message"]
	if !ok {
		return ""
	}
	var m struct {
		HTML string `json:"html"`
		Skip bool   `json:"skip_entity_detection"`
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("rich_message: %v", err)
	}
	if !m.Skip {
		t.Error("entity detection must be skipped for user text")
	}
	return m.HTML
}

func lastCall(t *testing.T, calls []apiCall, method string) apiCall {
	t.Helper()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].method == method {
			return calls[i]
		}
	}
	t.Fatalf("no %s in %s", method, methods(calls))
	return apiCall{}
}

func TestSwitchViewsOnLiveScreen(t *testing.T) {
	h, inbox, cols := boardHarness(t)
	ctx := context.Background()
	live := h.live()
	tasks, _ := h.store.CreateTasks(ctx, userID, cols[0].ID, []string{"buy cable"})
	h.store.CreateTasks(ctx, userID, cols[2].ID, []string{"old"})

	// Table view: the same live message is edited into a rich table.
	h.press(live, cb.New(cb.KindView, cols[0].ID, domain.ViewTable.Code()))
	edit := lastCall(t, h.api.take(), "editMessageText")
	table := richHTML(t, edit)
	if edit.params["message_id"] != itoa(live) || !strings.Contains(table, "<table") ||
		!strings.Contains(table, `data="`+cb.New(cb.KindTask, tasks[0].ID).String()+`"`) {
		t.Fatalf("table edit: %+v\n%s", edit.params, table)
	}
	if !strings.Contains(edit.params["reply_markup"], "• ▦") {
		t.Errorf("view switch must mark the table: %s", edit.params["reply_markup"])
	}
	if u, _ := h.store.GetUser(ctx, userID); u.ViewMode != domain.ViewTable {
		t.Errorf("view not remembered: %q", u.ViewMode)
	}

	// Opening a project from home keeps the chosen view.
	h.press(live, cb.New(cb.KindHome))
	h.press(live, cb.New(cb.KindProject, inbox.ID))
	if richHTML(t, lastCall(t, h.api.take(), "editMessageText")) == "" {
		t.Error("project opened in plain view despite table preference")
	}

	// A task card from a rich view is plain text; "Board" goes back to the rich view.
	h.press(live, cb.New(cb.KindTask, tasks[0].ID))
	card := lastCall(t, h.api.take(), "editMessageText")
	if richHTML(t, card) != "" || !strings.Contains(card.params["text"], "buy cable") {
		t.Fatalf("card: %+v", card.params)
	}
	h.press(live, cb.New(cb.KindBoard, cols[0].ID))
	if richHTML(t, lastCall(t, h.api.take(), "editMessageText")) == "" {
		t.Error("back to board did not restore the table")
	}

	// Article view collapses done columns.
	h.press(live, cb.New(cb.KindView, cols[0].ID, domain.ViewArticle.Code()))
	article := richHTML(t, lastCall(t, h.api.take(), "editMessageText"))
	if !strings.Contains(article, "<details><summary>Done · 1</summary>") {
		t.Errorf("article: %s", article)
	}

	// "More" in rich views leads to the buttons view of that column.
	h.press(live, cb.New(cb.KindView, cols[2].ID, domain.ViewButtons.Code()))
	edit = lastCall(t, h.api.take(), "editMessageText")
	if richHTML(t, edit) != "" || !strings.Contains(edit.params["text"], "› Done (1 / 2)") {
		t.Errorf("buttons view of the done column: %+v", edit.params)
	}
}

func TestNewTaskFromTableView(t *testing.T) {
	h, _, cols := boardHarness(t)
	ctx := context.Background()
	live := h.live()
	_ = h.store.SetViewMode(ctx, userID, domain.ViewTable)

	h.press(live, cb.New(cb.KindTaskNew, cols[0].ID))
	h.message("one\ntwo", 601)
	html := richHTML(t, lastCall(t, h.api.take(), "editMessageText"))
	if !strings.Contains(html, ">one</tg-button>") || !strings.Contains(html, ">two</tg-button>") {
		t.Errorf("table after create: %s", html)
	}
}

func TestRichScreenSentWhenEditFails(t *testing.T) {
	h, inbox, _ := boardHarness(t)
	_ = h.store.SetViewMode(context.Background(), userID, domain.ViewArticle)
	old := h.live()

	// If the live message cannot be edited, the rich board is sent as a new message.
	h.api.failEdit = true
	h.press(old, cb.New(cb.KindProject, inbox.ID))
	calls := h.api.take()
	if richHTML(t, lastCall(t, calls, "sendRichMessage")) == "" {
		t.Fatalf("calls: %s", methods(calls))
	}
	if h.live() == old {
		t.Error("live screen not replaced")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
