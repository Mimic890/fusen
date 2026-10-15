package bot

import (
	"context"
	"strings"
	"testing"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
)

func TestNewUserDefaultsFollowLanguage(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.messageAs(3003, "/start", 500) // language_code "ru"
	u, _ := h.store.GetUser(ctx, 3003)
	ps, _ := h.store.ListProjects(ctx, 3003, false)
	cols, _ := h.store.ListColumns(ctx, ps[0].ID)
	if u.Lang != "ru" || u.DigestTime != domain.DefaultDigestTime || ps[0].Name != "Входящие" ||
		cols[0].Name != "К выполнению" || !cols[2].IsDone {
		t.Fatalf("user %+v, inbox %q, columns %+v", u, ps[0].Name, cols)
	}
}

func TestSwitchLanguage(t *testing.T) {
	h, inbox, _ := boardHarness(t)
	ctx := context.Background()
	live := h.live()

	h.press(live, cb.New(cb.KindSetLang, domain.LangEN))
	calls := h.api.take()
	text, markup := lastEdit(t, calls)
	if !strings.Contains(text, "<b>Settings</b>") || !strings.Contains(markup, "• English") {
		t.Fatalf("settings in en: %q %q", text, markup)
	}
	if lastText(calls, "answerCallbackQuery") != "Saved." {
		t.Errorf("toast must use the new language: %q", lastText(calls, "answerCallbackQuery"))
	}
	if p, _ := h.store.GetProject(ctx, userID, inbox.ID); p.Name != "Inbox" {
		t.Errorf("default inbox name not translated: %q", p.Name)
	}

	// Once the user changed the language, Telegram's language_code no longer applies.
	h.press(live, cb.New(cb.KindHome))
	text, markup = lastEdit(t, h.api.take())
	if !strings.Contains(text, "<b>Projects</b>") || !strings.Contains(markup, "Settings") {
		t.Errorf("home in en: %q %q", text, markup)
	}

	h.press(live, cb.New(cb.KindSetLang, "de"))
	if u, _ := h.store.GetUser(ctx, userID); u.Lang != domain.LangEN {
		t.Errorf("unsupported language accepted: %q", u.Lang)
	}
}

func TestTimezoneSettings(t *testing.T) {
	h, _, _ := boardHarness(t)
	ctx := context.Background()
	live := h.live()

	h.press(live, cb.New(cb.KindTimezone, "Europe/Moscow"))
	if u, _ := h.store.GetUser(ctx, userID); u.Timezone != "Europe/Moscow" {
		t.Fatalf("preset: %q", u.Timezone)
	}

	h.press(live, cb.New(cb.KindTimezone))
	h.message("Mars/Base", 601)
	text, _ := lastEdit(t, h.api.take())
	if !strings.Contains(text, "Неизвестный часовой пояс") {
		t.Errorf("bad zone must keep the picker with an error: %q", text)
	}
	h.message("asia/tokyo", 602)
	u, _ := h.store.GetUser(ctx, userID)
	if u.Timezone != "Asia/Tokyo" || u.State != "" {
		t.Fatalf("typed zone: %+v", u)
	}
	h.press(live, cb.New(cb.KindTimezone))
	h.message("+3", 603)
	if u, _ = h.store.GetUser(ctx, userID); u.Timezone != "Etc/GMT-3" {
		t.Errorf("offset: %q", u.Timezone)
	}

	// Crafted callback data with an unknown zone is rejected.
	h.press(live, cb.New(cb.KindTimezone, "Local"))
	if u, _ = h.store.GetUser(ctx, userID); u.Timezone != "Etc/GMT-3" {
		t.Errorf("crafted zone accepted: %q", u.Timezone)
	}
}

func TestDigestAndRemindSettings(t *testing.T) {
	h, _, _ := boardHarness(t)
	ctx := context.Background()
	live := h.live()

	h.press(live, cb.New(cb.KindDigest, cb.Off))
	if u, _ := h.store.GetUser(ctx, userID); u.DigestTime != "" {
		t.Fatalf("digest off: %q", u.DigestTime)
	}
	h.press(live, cb.New(cb.KindDigest, "0730"))
	if u, _ := h.store.GetUser(ctx, userID); u.DigestTime != "07:30" {
		t.Fatalf("digest preset: %q", u.DigestTime)
	}
	h.press(live, cb.New(cb.KindDigest))
	h.message("25:00", 601)
	if u, _ := h.store.GetUser(ctx, userID); u.State == "" || u.DigestTime != "07:30" {
		t.Fatalf("invalid time must keep the prompt: %+v", u)
	}
	h.message("8.15", 602)
	if u, _ := h.store.GetUser(ctx, userID); u.DigestTime != "08:15" || u.State != "" {
		t.Fatalf("typed time: %+v", u)
	}

	h.press(live, cb.New(cb.KindRemind, 15))
	if u, _ := h.store.GetUser(ctx, userID); u.RemindBeforeMin != 15 {
		t.Fatalf("remind: %d", u.RemindBeforeMin)
	}
	h.press(live, cb.New(cb.KindRemind, 7))
	if u, _ := h.store.GetUser(ctx, userID); u.RemindBeforeMin != 15 {
		t.Errorf("non-preset accepted: %d", u.RemindBeforeMin)
	}
	h.press(live, cb.New(cb.KindSetView, domain.ViewArticle.Code()))
	if u, _ := h.store.GetUser(ctx, userID); u.ViewMode != domain.ViewArticle {
		t.Errorf("view: %q", u.ViewMode)
	}
}

func TestSettingsCommand(t *testing.T) {
	h, _, _ := boardHarness(t)
	h.message("/settings", 601)
	calls := h.api.take()
	if got := methods(calls); got != "deleteMessage,editMessageText" {
		t.Fatalf("calls: %s", got)
	}
	if text, _ := lastEdit(t, calls); !strings.Contains(text, "<b>Настройки</b>") {
		t.Errorf("settings: %q", text)
	}
}
