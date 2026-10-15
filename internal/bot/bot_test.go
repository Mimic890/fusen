package bot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/config"
	"github.com/Mimic890/fusen/internal/i18n"
	"github.com/Mimic890/fusen/internal/store"
	"github.com/Mimic890/fusen/locales"
)

type apiCall struct {
	method string
	params map[string]string
}

// fakeAPI is a minimal Telegram Bot API server that records calls.
type fakeAPI struct {
	mu     sync.Mutex
	calls  []apiCall
	nextID int
	// failDelete makes deleteMessage fail, like for messages older than 48 hours.
	failDelete bool
	// failEdit makes editMessageText fail, like for a message deleted by the user.
	failEdit bool
	// rateLimit makes the next N calls of a method respond with 429.
	rateLimit map[string]int
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := path.Base(r.URL.Path)
	params := map[string]string{}
	if err := r.ParseMultipartForm(1 << 20); err == nil {
		for k, v := range r.MultipartForm.Value {
			params[k] = v[0]
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, apiCall{method: method, params: params})

	if f.rateLimit[method] > 0 {
		f.rateLimit[method]--
		io.WriteString(w, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`)
		return
	}

	if method == "editMessageText" && f.failEdit {
		io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: message to edit not found"}`)
		return
	}

	switch method {
	case "sendMessage", "sendRichMessage", "editMessageText":
		id := f.nextID + 1
		if method != "editMessageText" {
			f.nextID++
		} else {
			id, _ = strconv.Atoi(params["message_id"])
		}
		fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"date":0,"chat":{"id":%s,"type":"private"}}}`, id, params["chat_id"])
	case "deleteMessage":
		if f.failDelete {
			io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: message can't be deleted"}`)
			return
		}
		io.WriteString(w, `{"ok":true,"result":true}`)
	default:
		io.WriteString(w, `{"ok":true,"result":true}`)
	}
}

// take returns and resets recorded calls.
func (f *fakeAPI) take() []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls
	f.calls = nil
	return c
}

func methods(calls []apiCall) string {
	m := make([]string, len(calls))
	for i, c := range calls {
		m[i] = c.method
	}
	return strings.Join(m, ",")
}

// stubSleep makes 429 retries instant for the test.
func stubSleep(t *testing.T) {
	orig := sleep
	sleep = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { sleep = orig })
}

type harness struct {
	t     *testing.T
	api   *fakeAPI
	bot   *Bot
	store *store.Store
	upd   int64
}

const userID = 1001

func newHarness(t *testing.T, allowed ...int64) *harness {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	tr, err := i18n.Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}

	api := &fakeAPI{rateLimit: map[string]int{}}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	cfg := config.Config{BotToken: "1:test", DefaultTZ: "UTC", AllowedUserIDs: map[int64]struct{}{}}
	for _, id := range allowed {
		cfg.AllowedUserIDs[id] = struct{}{}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	b, err := newBot(cfg, st, tr, log, tgbot.WithServerURL(srv.URL), tgbot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}

	stubSleep(t)
	return &harness{t: t, api: api, bot: b, store: st}
}

func (h *harness) message(text string, msgID int) {
	h.messageAs(userID, text, msgID)
}

func (h *harness) messageAs(uid int64, text string, msgID int) {
	h.upd++
	h.bot.handleUpdate(context.Background(), nil, &models.Update{ID: h.upd, Message: &models.Message{
		ID:   msgID,
		From: &models.User{ID: uid, LanguageCode: "ru"},
		Chat: models.Chat{ID: uid, Type: models.ChatTypePrivate},
		Text: text,
	}})
}

func (h *harness) press(msgID int, data cb.Data) {
	h.pressAs(userID, msgID, data)
}

func (h *harness) pressAs(uid int64, msgID int, data cb.Data) {
	h.upd++
	h.bot.handleUpdate(context.Background(), nil, &models.Update{ID: h.upd, CallbackQuery: &models.CallbackQuery{
		ID:   "q",
		From: models.User{ID: uid, LanguageCode: "ru"},
		Message: models.MaybeInaccessibleMessage{
			Type:    models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{ID: msgID, Chat: models.Chat{ID: uid, Type: models.ChatTypePrivate}},
		},
		Data: data.String(),
	}})
}

func (h *harness) live() int {
	u, err := h.store.GetUser(context.Background(), userID)
	if err != nil {
		h.t.Fatal(err)
	}
	return u.LiveMessageID
}

func lastText(calls []apiCall, method string) string {
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].method == method {
			return calls[i].params["text"]
		}
	}
	return ""
}

func TestStartOpensLiveScreenAndRetiresOld(t *testing.T) {
	h := newHarness(t)

	h.message("/start", 500)
	calls := h.api.take()
	if got := methods(calls); got != "deleteMessage,sendMessage" {
		t.Fatalf("first start: %s", got)
	}
	if calls[0].params["message_id"] != "500" {
		t.Errorf("user /start not deleted: %+v", calls[0])
	}
	if !strings.Contains(calls[1].params["reply_markup"], "Входящие") {
		t.Errorf("home screen keyboard: %q", calls[1].params["reply_markup"])
	}
	first := h.live()

	h.message("/start", 501)
	calls = h.api.take()
	if got := methods(calls); got != "deleteMessage,sendMessage,deleteMessage" {
		t.Fatalf("second start: %s", got)
	}
	if calls[2].params["message_id"] != strconv.Itoa(first) {
		t.Errorf("old live screen not deleted: %+v", calls[2])
	}

	// If the old screen cannot be deleted, it is marked stale instead.
	h.api.failDelete = true
	second := h.live()
	h.message("/start", 502)
	calls = h.api.take()
	if got := methods(calls); got != "deleteMessage,sendMessage,deleteMessage,editMessageText" {
		t.Fatalf("third start: %s", got)
	}
	stale := calls[3]
	if stale.params["message_id"] != strconv.Itoa(second) || !strings.Contains(stale.params["reply_markup"], `"o"`) {
		t.Errorf("stale edit: %+v", stale)
	}
}

func TestStaleScreenCallback(t *testing.T) {
	h := newHarness(t)
	h.message("/start", 500)
	h.api.take()

	h.press(h.live()+100, cb.New(cb.KindProjectNew))
	calls := h.api.take()
	if got := methods(calls); got != "editMessageText,answerCallbackQuery" {
		t.Fatalf("stale press: %s", got)
	}
	if calls[1].params["text"] == "" {
		t.Error("stale press must show a toast")
	}

	h.press(h.live()+100, cb.New(cb.KindReopen))
	if got := methods(h.api.take()); got != "sendMessage,deleteMessage,answerCallbackQuery" {
		t.Fatalf("reopen: %s", got)
	}
}

func TestCreateRenameArchiveDeleteProject(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.message("/start", 500)
	live := h.live()
	h.api.take()

	// Plain text without pending input is quick input (covered in quick_test.go).
	h.message("hello", 501)
	if got := methods(h.api.take()); got != "editMessageText,deleteMessage" {
		t.Fatalf("idle text: %s", got)
	}

	h.press(live, cb.New(cb.KindProjectNew))
	h.api.take()

	// Invalid name keeps the prompt open.
	h.message("   ", 502)
	calls := h.api.take()
	if got := methods(calls); got != "editMessageText,deleteMessage" {
		t.Fatalf("empty name: %s", got)
	}
	if u, _ := h.store.GetUser(ctx, userID); u.State != cb.KindProjectNew {
		t.Fatalf("state after invalid name = %q", u.State)
	}

	h.message("  Home   lab ", 503)
	calls = h.api.take()
	if got := methods(calls); got != "editMessageText,deleteMessage" {
		t.Fatalf("create: %s", got)
	}
	if !strings.Contains(lastText(calls, "editMessageText"), "Home lab") {
		t.Errorf("project screen: %q", lastText(calls, "editMessageText"))
	}
	ps, _ := h.store.ListProjects(ctx, userID, false)
	if len(ps) != 2 || ps[1].Name != "Home lab" {
		t.Fatalf("projects = %+v", ps)
	}
	p := ps[1]
	u, _ := h.store.GetUser(ctx, userID)
	if u.State != "" || u.CurrentProjectID != p.ID {
		t.Fatalf("user after create = %+v", u)
	}

	h.press(live, cb.New(cb.KindProjectRename, p.ID))
	h.message("Garage", 504)
	h.api.take()
	if got, _ := h.store.GetProject(ctx, userID, p.ID); got.Name != "Garage" {
		t.Fatalf("rename: %+v", got)
	}

	h.press(live, cb.New(cb.KindProjectArchive, p.ID, "1"))
	if got, _ := h.store.GetProject(ctx, userID, p.ID); !got.Archived {
		t.Fatal("not archived")
	}
	h.press(live, cb.New(cb.KindProjectArchive, p.ID, "0"))
	if got, _ := h.store.GetProject(ctx, userID, p.ID); got.Archived {
		t.Fatal("not restored")
	}
	h.api.take()

	h.press(live, cb.New(cb.KindProjectDelete, p.ID))
	if _, err := h.store.GetProject(ctx, userID, p.ID); err != nil {
		t.Fatal("deleted without confirmation")
	}
	h.press(live, cb.New(cb.KindProjectDelete, p.ID, cb.Confirm))
	if _, err := h.store.GetProject(ctx, userID, p.ID); err == nil {
		t.Fatal("not deleted")
	}

	// Every screen change edited the same live message.
	for _, c := range h.api.take() {
		if c.method == "sendMessage" {
			t.Errorf("unexpected new message: %+v", c)
		}
	}
}

func TestNavigationCancelsInput(t *testing.T) {
	h := newHarness(t)
	h.message("/start", 500)
	live := h.live()
	h.press(live, cb.New(cb.KindProjectNew))
	h.press(live, cb.New(cb.KindHome))
	if u, _ := h.store.GetUser(context.Background(), userID); u.State != "" {
		t.Fatalf("state = %q", u.State)
	}
}

func TestInboxCannotBeDeleted(t *testing.T) {
	h := newHarness(t)
	h.message("/start", 500)
	ps, _ := h.store.ListProjects(context.Background(), userID, false)
	h.api.take()

	h.press(h.live(), cb.New(cb.KindProjectDelete, ps[0].ID, cb.Confirm))
	calls := h.api.take()
	if lastText(calls, "answerCallbackQuery") == "" {
		t.Error("expected an error toast")
	}
	if ps2, _ := h.store.ListProjects(context.Background(), userID, false); len(ps2) != 1 {
		t.Fatal("inbox deleted")
	}
}

func TestAccessDenied(t *testing.T) {
	h := newHarness(t, 42)
	h.message("/start", 500)
	calls := h.api.take()
	if got := methods(calls); got != "sendMessage" || lastText(calls, "sendMessage") != "Доступ запрещён." {
		t.Fatalf("denied: %s %q", got, lastText(calls, "sendMessage"))
	}
	if _, err := h.store.GetUser(context.Background(), userID); err == nil {
		t.Error("denied user must not be stored")
	}
}

func TestRetryOn429(t *testing.T) {
	h := newHarness(t)
	h.api.rateLimit["sendMessage"] = 2
	h.message("/start", 500)
	if got := methods(h.api.take()); got != "deleteMessage,sendMessage,sendMessage,sendMessage" {
		t.Fatalf("calls: %s", got)
	}
	if h.live() == 0 {
		t.Fatal("live screen not stored after retries")
	}
}

func TestWithRetryGivesUp(t *testing.T) {
	stubSleep(t)
	n := 0
	_, err := withRetry(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), "m",
		func(context.Context) (int, error) {
			n++
			return 0, &tgbot.TooManyRequestsError{RetryAfter: 1}
		})
	if !tgbot.IsTooManyRequestsError(err) || n != maxRetries+1 {
		t.Fatalf("n=%d err=%v", n, err)
	}

	n = 0
	_, err = withRetry(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), "m",
		func(context.Context) (int, error) { n++; return 0, errors.New("boom") })
	if err == nil || n != 1 {
		t.Fatalf("non-429 must not retry: n=%d", n)
	}
}

func TestCommand(t *testing.T) {
	cases := map[string]string{
		"/start":                  "start",
		"/start t_42":             "start",
		"/Start@fusenboard_bot x": "start",
		" /help ":                 "help",
		"start":                   "",
		"":                        "",
	}
	for in, want := range cases {
		if got := command(in); got != want {
			t.Errorf("command(%q) = %q, want %q", in, got, want)
		}
	}
}
