package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // embed the timezone database: the runtime image has none
)

var (
	ErrBadTimezone = errors.New("unknown timezone")
	ErrBadClock    = errors.New("invalid time of day")
)

// Langs lists supported languages in display order.
var Langs = []string{LangEN, LangRU}

// IsLang reports whether lang is supported.
func IsLang(lang string) bool {
	return lang == LangEN || lang == LangRU
}

// TimezonePresets are offered as one-tap choices in settings.
var TimezonePresets = []string{
	"UTC",
	"Europe/London",
	"Europe/Berlin",
	"Europe/Kyiv",
	"Europe/Moscow",
	"Asia/Tbilisi",
	"Asia/Yekaterinburg",
	"Asia/Almaty",
	"Asia/Novosibirsk",
	"Asia/Vladivostok",
	"America/New_York",
	"America/Los_Angeles",
}

// DigestPresets are offered digest times ("HH:MM").
var DigestPresets = []string{"07:00", "08:00", "09:00", "10:00"}

// DefaultDigestTime is set for new users.
const DefaultDigestTime = "09:00"

// RemindPresets are offered "remind before deadline" values in minutes; 0 disables.
var RemindPresets = []int{0, 15, 30, 60, 180, 1440}

var offsetRe = regexp.MustCompile(`^(?i:utc|gmt)?\s*([+-])\s*(\d{1,2})$`)

// ParseTimezone accepts an IANA name in any letter case ("europe/moscow") or a
// whole-hour UTC offset ("+3", "UTC-5") and returns a loadable location name.
func ParseTimezone(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrBadTimezone
	}
	if m := offsetRe.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[2])
		if h == 0 {
			return "UTC", nil
		}
		// Etc/GMT zones have inverted signs: UTC+3 is Etc/GMT-3.
		sign := "-"
		if m[1] == "-" {
			sign = "+"
		}
		name := "Etc/GMT" + sign + strconv.Itoa(h)
		if _, err := time.LoadLocation(name); err != nil {
			return "", ErrBadTimezone
		}
		return name, nil
	}
	if strings.EqualFold(s, "utc") || strings.EqualFold(s, "gmt") {
		return "UTC", nil
	}
	for _, name := range []string{s, canonicalZone(s)} {
		// LoadLocation also accepts "Local" and paths; only real zone names are wanted.
		if name == "Local" || strings.Contains(name, "..") {
			continue
		}
		if _, err := time.LoadLocation(name); err == nil && strings.Contains(name, "/") {
			return name, nil
		}
	}
	return "", ErrBadTimezone
}

// canonicalZone capitalizes zone name parts: "america/new_york" -> "America/New_York".
func canonicalZone(s string) string {
	parts := strings.Split(s, "/")
	for i, p := range parts {
		words := strings.Split(p, "_")
		for j, w := range words {
			if w == "" {
				continue
			}
			words[j] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
		parts[i] = strings.Join(words, "_")
	}
	return strings.Join(parts, "/")
}

// UTCOffset formats the zone's current offset, e.g. "UTC+3" or "UTC+5:30".
func UTCOffset(name string, now time.Time) string {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return "UTC"
	}
	_, off := now.In(loc).Zone()
	if off == 0 {
		return "UTC"
	}
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	s := "UTC" + sign + strconv.Itoa(off/3600)
	if m := off % 3600 / 60; m != 0 {
		s += fmt.Sprintf(":%02d", m)
	}
	return s
}

var clockRe = regexp.MustCompile(`^(\d{1,2})(?:[:.](\d{2}))?$`)

// ParseClock parses a time of day ("9", "9:30", "09.30") into "HH:MM".
func ParseClock(s string) (string, error) {
	m := clockRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", ErrBadClock
	}
	h, _ := strconv.Atoi(m[1])
	mins := 0
	if m[2] != "" {
		mins, _ = strconv.Atoi(m[2])
	}
	if h > 23 || mins > 59 {
		return "", ErrBadClock
	}
	return fmt.Sprintf("%02d:%02d", h, mins), nil
}

// ClockCode packs "HH:MM" into "HHMM" for callback data (':' is the separator).
func ClockCode(clock string) string {
	return strings.ReplaceAll(clock, ":", "")
}

// ParseClockCode is the inverse of ClockCode.
func ParseClockCode(code string) (string, error) {
	if len(code) != 4 {
		return "", ErrBadClock
	}
	return ParseClock(code[:2] + ":" + code[2:])
}
