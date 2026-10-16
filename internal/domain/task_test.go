package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseTags(t *testing.T) {
	cases := []struct {
		in   string
		want []string
		err  error
	}{
		{"#home, urgent #Work", []string{"home", "urgent", "work"}, nil},
		{"#дом #Дом ##дом", []string{"дом"}, nil},
		{"#home-lab #a_b #c!d", []string{"home-lab", "a_b", "cd"}, nil},
		{"  , # ", nil, nil},
		{"#" + strings.Repeat("x", MaxTagLen+1), nil, ErrTagTooLong},
		{strings.Repeat("#a ", 1) + "#b #c #d #e #f #g #h #i #j #k", nil, ErrTooManyTags},
	}
	for _, c := range cases {
		got, err := ParseTags(c.in)
		if !errors.Is(err, c.err) || !slices.Equal(got, c.want) {
			t.Errorf("ParseTags(%q) = %v, %v; want %v, %v", c.in, got, err, c.want, c.err)
		}
	}
}

func TestNormalizeTitleAndItem(t *testing.T) {
	if got, err := NormalizeTitle("  buy \n cable "); err != nil || got != "buy cable" {
		t.Errorf("title: %q %v", got, err)
	}
	if _, err := NormalizeTitle(" "); !errors.Is(err, ErrEmptyName) {
		t.Errorf("empty title: %v", err)
	}
	if _, err := NormalizeTitle(strings.Repeat("a", MaxTitleLen+1)); !errors.Is(err, ErrTitleTooLong) {
		t.Errorf("long title: %v", err)
	}
	if _, err := NormalizeItem(strings.Repeat("a", MaxItemLen+1)); !errors.Is(err, ErrItemTooLong) {
		t.Errorf("long item: %v", err)
	}
}

func TestNormalizeDescription(t *testing.T) {
	got, err := NormalizeDescription("\n  line one  \nline two\t\n\n")
	if err != nil || got != "line one\nline two" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := NormalizeDescription(strings.Repeat("a", MaxDescriptionLen+1)); !errors.Is(err, ErrDescriptionTooLong) {
		t.Errorf("long: %v", err)
	}
}

func TestSplitLines(t *testing.T) {
	got := SplitLines(" one \n\n two  three \n ")
	if !slices.Equal(got, []string{"one", "two three"}) {
		t.Errorf("got %q", got)
	}
}

func TestIsOverdue(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	cases := []struct {
		task Task
		want bool
	}{
		{Task{DueAt: &past}, true},
		{Task{DueAt: &future}, false},
		{Task{}, false},
		{Task{DueAt: &past, DoneAt: &now}, false},
	}
	for i, c := range cases {
		if got := c.task.IsOverdue(now); got != c.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}

func TestDeadlineReminderAt(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		due    time.Time
		allDay bool
		before int
		want   time.Time
		ok     bool
	}{
		{"regular", now.Add(3 * time.Hour), false, 60, now.Add(2 * time.Hour), true},
		{"too close fires at the deadline", now.Add(30 * time.Minute), false, 60, now.Add(30 * time.Minute), true},
		{"past deadline", now.Add(-time.Minute), false, 60, time.Time{}, false},
		{"all day", now.Add(3 * time.Hour), true, 60, time.Time{}, false},
		{"disabled", now.Add(3 * time.Hour), false, 0, time.Time{}, false},
	}
	for _, c := range cases {
		got, ok := DeadlineReminderAt(c.due, c.allDay, c.before, now)
		if ok != c.ok || !got.Equal(c.want) {
			t.Errorf("%s: got %v %v", c.name, got, ok)
		}
	}
}

func TestEndOfDay(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	got := EndOfDay(time.Date(2026, 10, 15, 22, 0, 0, 0, time.UTC), loc) // 01:00 on the 16th in Moscow
	if got.Format("2006-01-02 15:04:05") != "2026-10-16 23:59:59" || got.Location() != loc {
		t.Errorf("got %v", got)
	}
}
