package views

import (
	"strings"
	"time"

	"github.com/go-telegram/bot/models"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/i18n"
)

// langNames are shown in their own language so they can be found in any UI language.
var langNames = map[string]string{domain.LangEN: "English", domain.LangRU: "Русский"}

func marked(label string, on bool) string {
	if on {
		return "• " + label
	}
	return label
}

// Duration formats minutes as "15 min", "1 h", "1 d" or "1 h 30 min".
func Duration(l i18n.Localizer, minutes int) string {
	switch {
	case minutes <= 0:
		return l.T("settings.off")
	case minutes%1440 == 0:
		return l.T("dur.day", "n", minutes/1440)
	case minutes%60 == 0:
		return l.T("dur.hour", "n", minutes/60)
	case minutes > 60:
		return l.T("dur.hour", "n", minutes/60) + " " + l.T("dur.min", "n", minutes%60)
	}
	return l.T("dur.min", "n", minutes)
}

func digestLabel(l i18n.Localizer, clock string) string {
	if clock == "" {
		return l.T("settings.off")
	}
	return clock
}

func timezoneLabel(name string, now time.Time) string {
	if off := domain.UTCOffset(name, now); off != name {
		return name + " · " + off
	}
	return name
}

// Settings renders the settings overview.
func Settings(l i18n.Localizer, u domain.User, now time.Time) Screen {
	var sb strings.Builder
	sb.WriteString(l.T("settings.title"))
	sb.WriteString("\n\n" + l.T("settings.lang", "value", langNames[u.Lang]))
	sb.WriteString("\n" + l.T("settings.view", "value", l.T("view."+string(u.ViewMode))))
	sb.WriteString("\n" + l.T("settings.timezone", "value", Escape(timezoneLabel(u.Timezone, now))))
	sb.WriteString("\n" + l.T("settings.digest", "value", digestLabel(l, u.DigestTime)))
	sb.WriteString("\n" + l.T("settings.remind", "value", Duration(l, u.RemindBeforeMin)))

	var langs, viewsRow []models.InlineKeyboardButton
	for _, lang := range domain.Langs {
		langs = append(langs, btn(marked(langNames[lang], lang == u.Lang), cb.New(cb.KindSetLang, lang)))
	}
	for _, v := range domain.Views {
		viewsRow = append(viewsRow, btn(marked(l.T("view."+string(v)), v == u.ViewMode), cb.New(cb.KindSetView, v.Code())))
	}
	return Screen{Text: sb.String(), Keyboard: [][]models.InlineKeyboardButton{
		langs,
		viewsRow,
		row(
			btn(l.T("settings.timezone_btn"), cb.New(cb.KindTimezone)),
			btn(l.T("settings.digest_btn"), cb.New(cb.KindDigest)),
		),
		row(btn(l.T("settings.remind_btn"), cb.New(cb.KindRemind))),
		row(btn(l.T("btn.projects"), cb.New(cb.KindHome))),
	}}
}

func backToSettings(l i18n.Localizer) []models.InlineKeyboardButton {
	return row(btn(l.T("btn.back"), cb.New(cb.KindSettings)))
}

// TimezonePicker offers preset zones; the user may also type a zone name or offset.
func TimezonePicker(l i18n.Localizer, u domain.User, now time.Time) Screen {
	text := l.T("timezone.title", "value", Escape(timezoneLabel(u.Timezone, now))) + "\n\n" + l.T("timezone.prompt")
	var kb [][]models.InlineKeyboardButton
	var pair []models.InlineKeyboardButton
	for _, name := range domain.TimezonePresets {
		pair = append(pair, btn(marked(timezoneLabel(name, now), name == u.Timezone), cb.New(cb.KindTimezone, name)))
		if len(pair) == 2 {
			kb = append(kb, pair)
			pair = nil
		}
	}
	if len(pair) > 0 {
		kb = append(kb, pair)
	}
	kb = append(kb, backToSettings(l))
	return Screen{Text: text, Keyboard: kb}
}

// DigestPicker offers preset digest times; the user may also type a time.
func DigestPicker(l i18n.Localizer, u domain.User) Screen {
	text := l.T("digest.title", "value", digestLabel(l, u.DigestTime)) + "\n\n" + l.T("digest.prompt")
	var times []models.InlineKeyboardButton
	for _, clock := range domain.DigestPresets {
		times = append(times, btn(marked(clock, clock == u.DigestTime), cb.New(cb.KindDigest, domain.ClockCode(clock))))
	}
	return Screen{Text: text, Keyboard: [][]models.InlineKeyboardButton{
		times,
		row(btn(marked(l.T("settings.off"), u.DigestTime == ""), cb.New(cb.KindDigest, cb.Off))),
		backToSettings(l),
	}}
}

// RemindPicker offers how long before a deadline to send a reminder.
func RemindPicker(l i18n.Localizer, u domain.User) Screen {
	text := l.T("remind.title", "value", Duration(l, u.RemindBeforeMin))
	var kb [][]models.InlineKeyboardButton
	var r []models.InlineKeyboardButton
	for _, m := range domain.RemindPresets {
		r = append(r, btn(marked(Duration(l, m), m == u.RemindBeforeMin), cb.New(cb.KindRemind, m)))
		if len(r) == 3 {
			kb = append(kb, r)
			r = nil
		}
	}
	if len(r) > 0 {
		kb = append(kb, r)
	}
	kb = append(kb, backToSettings(l))
	return Screen{Text: text, Keyboard: kb}
}
