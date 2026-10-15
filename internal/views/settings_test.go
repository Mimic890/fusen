package views

import (
	"strings"
	"testing"
	"time"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
)

func TestSettingsScreensRender(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	users := []domain.User{
		{Lang: "ru", ViewMode: domain.ViewTable, Timezone: "Europe/Moscow", DigestTime: "09:00", RemindBeforeMin: 60},
		{Lang: "en", ViewMode: domain.ViewButtons, Timezone: "Asia/Kolkata", RemindBeforeMin: 0},
	}
	for _, u := range users {
		l := localizer(t, u.Lang)
		for name, s := range map[string]Screen{
			"settings": Settings(l, u, now),
			"timezone": TimezonePicker(l, u, now),
			"digest":   DigestPicker(l, u),
			"remind":   RemindPicker(l, u),
		} {
			t.Run(u.Lang+"/"+name, func(t *testing.T) { checkScreen(t, s) })
		}
	}

	l := localizer(t, "ru")
	s := Settings(l, users[0], now)
	for _, want := range []string{"Русский", "▦ Таблица", "Europe/Moscow · UTC+3", "09:00", "за 1 ч"} {
		if !strings.Contains(s.Text, want) {
			t.Errorf("settings text lacks %q: %s", want, s.Text)
		}
	}
	if got := Settings(localizer(t, "en"), users[1], now).Text; !strings.Contains(got, "Asia/Kolkata · UTC+5:30") ||
		!strings.Contains(got, "Morning digest: off") {
		t.Errorf("en settings: %s", got)
	}
}

func TestPickersMarkCurrent(t *testing.T) {
	l := localizer(t, "en")
	u := domain.User{Lang: "en", Timezone: "Europe/Berlin", DigestTime: "08:00", RemindBeforeMin: 30}
	now := time.Now()
	cases := map[string]struct {
		s    Screen
		kind string
		want string
	}{
		"timezone": {TimezonePicker(l, u, now), cb.KindTimezone, "Europe/Berlin"},
		"digest":   {DigestPicker(l, u), cb.KindDigest, "0800"},
		"remind":   {RemindPicker(l, u), cb.KindRemind, cb.ID(30)},
	}
	for name, c := range cases {
		var marked []string
		for _, r := range c.s.Keyboard {
			for _, b := range r {
				if d, _ := cb.Decode(b.CallbackData); d.Kind == c.kind && strings.HasPrefix(b.Text, "• ") {
					marked = append(marked, d.Arg(0))
				}
			}
		}
		if len(marked) != 1 || marked[0] != c.want {
			t.Errorf("%s: marked %v, want %s", name, marked, c.want)
		}
	}
}

func TestDuration(t *testing.T) {
	en, ru := localizer(t, "en"), localizer(t, "ru")
	cases := []struct {
		min    int
		en, ru string
	}{
		{0, "off", "выкл."},
		{15, "15 min", "15 мин"},
		{60, "1 h", "1 ч"},
		{90, "1 h 30 min", "1 ч 30 мин"},
		{1440, "1 d", "1 д"},
	}
	for _, c := range cases {
		if got := Duration(en, c.min); got != c.en {
			t.Errorf("en %d: %q", c.min, got)
		}
		if got := Duration(ru, c.min); got != c.ru {
			t.Errorf("ru %d: %q", c.min, got)
		}
	}
}

func TestCardDeadlineUsesDateTimeEntity(t *testing.T) {
	l := localizer(t, "en")
	due := time.Date(2026, 10, 18, 15, 0, 0, 0, time.UTC)
	loc, _ := time.LoadLocation("Europe/Moscow")
	c := Card{Task: domain.Task{ID: 1, Title: "x", DueAt: &due}, Loc: loc, Now: due.Add(-time.Hour)}
	s := TaskCard(l, c)
	want := `<tg-time unix="` + "1792335600" + `" format="wdt">18.10.2026 18:00</tg-time>`
	if !strings.Contains(s.Text, want) {
		t.Errorf("card: %s", s.Text)
	}
}

func TestHomeHasSettings(t *testing.T) {
	s := Home(localizer(t, "en"), nil, domain.DueSummary{}, 0)
	if !strings.Contains(s.Markup().InlineKeyboard[len(s.Keyboard)-1][0].CallbackData, cb.KindSettings) {
		t.Error("home must link to settings")
	}
}
