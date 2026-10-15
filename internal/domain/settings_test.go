package domain

import (
	"errors"
	"testing"
	"time"
)

func TestParseTimezone(t *testing.T) {
	cases := map[string]string{
		"Europe/Moscow":      "Europe/Moscow",
		"europe/moscow":      "Europe/Moscow",
		" america/new_york ": "America/New_York",
		"UTC":                "UTC",
		"gmt":                "UTC",
		"+3":                 "Etc/GMT-3",
		"UTC+3":              "Etc/GMT-3",
		"utc -5":             "Etc/GMT+5",
		"GMT+0":              "UTC",
		"+14":                "Etc/GMT-14",
	}
	for in, want := range cases {
		got, err := ParseTimezone(in)
		if err != nil || got != want {
			t.Errorf("ParseTimezone(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "Mars/Base", "+15", "Local", "Moscow", "../etc/passwd", "+3:30"} {
		if got, err := ParseTimezone(in); !errors.Is(err, ErrBadTimezone) {
			t.Errorf("ParseTimezone(%q) = %q, %v; want error", in, got, err)
		}
	}
}

func TestTimezonePresetsLoad(t *testing.T) {
	for _, name := range TimezonePresets {
		if got, err := ParseTimezone(name); err != nil || got != name {
			t.Errorf("preset %q: %q %v", name, got, err)
		}
	}
}

func TestUTCOffset(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"UTC":            "UTC",
		"Europe/Moscow":  "UTC+3",
		"Asia/Kolkata":   "UTC+5:30",
		"America/Denver": "UTC-7",
		"Etc/GMT-3":      "UTC+3",
		"Nope/Nope":      "UTC",
	}
	for in, want := range cases {
		if got := UTCOffset(in, now); got != want {
			t.Errorf("UTCOffset(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseClock(t *testing.T) {
	cases := map[string]string{"9": "09:00", "9:30": "09:30", "09.05": "09:05", " 23:59 ": "23:59", "0": "00:00"}
	for in, want := range cases {
		if got, err := ParseClock(in); err != nil || got != want {
			t.Errorf("ParseClock(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "24", "9:60", "9:5", "nine", "123"} {
		if _, err := ParseClock(in); !errors.Is(err, ErrBadClock) {
			t.Errorf("ParseClock(%q): want error", in)
		}
	}
	if got, err := ParseClockCode(ClockCode("07:30")); err != nil || got != "07:30" {
		t.Errorf("clock code round trip: %q %v", got, err)
	}
	if _, err := ParseClockCode("730"); err == nil {
		t.Error("short code accepted")
	}
}
