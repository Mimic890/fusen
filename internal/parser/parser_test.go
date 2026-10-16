package parser

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

var msk, _ = time.LoadLocation("Europe/Moscow")

// now is Thursday, 15 October 2026, 12:00 in Moscow.
var now = time.Date(2026, 10, 15, 12, 0, 0, 0, msk)

func at(y int, m time.Month, d, h, min int) *time.Time {
	t := time.Date(y, m, d, h, min, 0, 0, msk)
	return &t
}

func day(y int, m time.Month, d int) *time.Time {
	t := domain.EndOfDay(time.Date(y, m, d, 0, 0, 0, 0, msk), msk)
	return &t
}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Result
	}{
		{"купить кабель #homelab !завтра 18:00 !!", Result{Title: "купить кабель", Tags: []string{"homelab"},
			Priority: domain.PriorityMed, Due: at(2026, 10, 16, 18, 0)}},
		{"buy milk", Result{Title: "buy milk"}},
		{"  call   mom  ", Result{Title: "call mom"}},
		{"report !!! #Work #work #urgent", Result{Title: "report", Tags: []string{"work", "urgent"}, Priority: domain.PriorityHigh}},
		{"! water plants", Result{Title: "water plants", Priority: domain.PriorityLow}},
		{"!!!! panic", Result{Title: "panic", Priority: domain.PriorityHigh}},
		{"pay rent !today", Result{Title: "pay rent", Due: day(2026, 10, 15), AllDay: true}},
		{"оплатить !сегодня", Result{Title: "оплатить", Due: day(2026, 10, 15), AllDay: true}},
		{"x !послезавтра 9:05", Result{Title: "x", Due: at(2026, 10, 17, 9, 5)}},
		{"x !Tomorrow", Result{Title: "x", Due: day(2026, 10, 16), AllDay: true}},
		// Weekdays mean the next occurrence after today; today is Thursday.
		{"x !пт", Result{Title: "x", Due: day(2026, 10, 16), AllDay: true}},
		{"x !thu", Result{Title: "x", Due: day(2026, 10, 22), AllDay: true}},
		{"x !понедельник 10:00", Result{Title: "x", Due: at(2026, 10, 19, 10, 0)}},
		{"x !mon", Result{Title: "x", Due: day(2026, 10, 19), AllDay: true}},
		// Dates.
		{"x !25.12", Result{Title: "x", Due: day(2026, 12, 25), AllDay: true}},
		{"x !25/12 23:30", Result{Title: "x", Due: at(2026, 12, 25, 23, 30)}},
		{"x !1.03", Result{Title: "x", Due: day(2027, 3, 1), AllDay: true}}, // passed this year
		{"x !15.10", Result{Title: "x", Due: day(2026, 10, 15), AllDay: true}},
		{"x !5.1.27", Result{Title: "x", Due: day(2027, 1, 5), AllDay: true}},
		{"x !2027-02-03 8:00", Result{Title: "x", Due: at(2027, 2, 3, 8, 0)}},
		// Times alone: today, or tomorrow when already past.
		{"x !18:00", Result{Title: "x", Due: at(2026, 10, 15, 18, 0)}},
		{"x !9:00", Result{Title: "x", Due: at(2026, 10, 16, 9, 0)}},
		{"x !12:00", Result{Title: "x", Due: at(2026, 10, 16, 12, 0)}},
		// Meta tokens anywhere; the last deadline wins.
		{"!завтра #a fix !! bug !пт", Result{Title: "fix bug", Tags: []string{"a"}, Priority: domain.PriorityMed,
			Due: day(2026, 10, 16), AllDay: true}},
		// Things that look like meta but are not stay in the title.
		{"wow! great !idea 18:00", Result{Title: "wow! great !idea 18:00"}},
		{"x !31.02 # #! c#", Result{Title: "x !31.02 # #! c#"}},
		{"x !25:00", Result{Title: "x !25:00"}},
		{"meet at 18:00 tomorrow", Result{Title: "meet at 18:00 tomorrow"}},
		// A time after a date belongs to it only when written as HH:MM.
		{"x !завтра 18 часов", Result{Title: "x 18 часов", Due: day(2026, 10, 16), AllDay: true}},
	}
	for _, c := range cases {
		got, err := Parse(c.in, now, msk)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if got.Title != c.want.Title || !slices.Equal(got.Tags, c.want.Tags) || got.Priority != c.want.Priority ||
			got.AllDay != c.want.AllDay || !sameTime(got.Due, c.want.Due) {
			t.Errorf("Parse(%q)\n got %+v due=%v\nwant %+v due=%v", c.in, got, fmtTime(got.Due), c.want, fmtTime(c.want.Due))
		}
	}
}

func TestParseUsesLocation(t *testing.T) {
	// 23:30 UTC on the 15th is already the 16th in Moscow.
	late := time.Date(2026, 10, 15, 23, 30, 0, 0, time.UTC)
	got, err := Parse("x !today", late, msk)
	if err != nil || !sameTime(got.Due, day(2026, 10, 16)) {
		t.Errorf("got %v, %v", fmtTime(got.Due), err)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]error{
		"":                domain.ErrEmptyName,
		"#tag !завтра !!": domain.ErrEmptyName,
		"x #" + strings.Repeat("t", domain.MaxTagLen+1): domain.ErrTagTooLong,
		"x #1 #2 #3 #4 #5 #6 #7 #8 #9 #10 #11":          domain.ErrTooManyTags,
		strings.Repeat("a", domain.MaxTitleLen+1):       domain.ErrTitleTooLong,
	}
	for in, want := range cases {
		if _, err := Parse(in, now, msk); !errors.Is(err, want) {
			t.Errorf("Parse(%q) = %v, want %v", in, err, want)
		}
	}
}

func TestParseDue(t *testing.T) {
	cases := []struct {
		in     string
		want   *time.Time
		allDay bool
	}{
		{"завтра 18:00", at(2026, 10, 16, 18, 0), false},
		{"завтра в 18:00", at(2026, 10, 16, 18, 0), false},
		{"fri at 9:30", at(2026, 10, 16, 9, 30), false},
		{"25.12", day(2026, 12, 25), true},
		{" 18:00 ", at(2026, 10, 15, 18, 0), false},
		{"Today", day(2026, 10, 15), true},
	}
	for _, c := range cases {
		got, allDay, err := ParseDue(c.in, now, msk)
		if err != nil || allDay != c.allDay || !got.Equal(*c.want) {
			t.Errorf("ParseDue(%q) = %v %v %v; want %v %v", c.in, got, allDay, err, *c.want, c.allDay)
		}
	}
	for _, in := range []string{"", "soon", "завтра вечером", "18:00 завтра", "завтра 18:00 x", "в", "32.01"} {
		if _, _, err := ParseDue(in, now, msk); !errors.Is(err, ErrBadDate) {
			t.Errorf("ParseDue(%q): want ErrBadDate, got %v", in, err)
		}
	}
}

func TestResultTask(t *testing.T) {
	r := Result{Title: "x", Tags: []string{"a"}, Priority: domain.PriorityLow, Due: at(2026, 1, 1, 1, 1), AllDay: false}
	nt := r.Task()
	if nt.Title != "x" || nt.Priority != domain.PriorityLow || nt.DueAt != r.Due || len(nt.Tags) != 1 {
		t.Errorf("task = %+v", nt)
	}
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return "nil"
	}
	return t.In(msk).Format("2006-01-02 15:04:05")
}
