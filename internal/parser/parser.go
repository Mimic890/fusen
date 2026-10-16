// Package parser turns quick-add text into a task.
//
// Syntax (Russian and English words are accepted regardless of the UI language):
//
//		купить кабель #homelab !завтра 18:00 !!
//
//	  - #tag          adds a tag
//	  - !, !!, !!!    sets low, medium or high priority
//	  - !<date>       sets an all-day deadline: today, tomorrow, сегодня, завтра,
//	    послезавтра, a weekday (mon, пт, пятница), 25.12, 25.12.2026, 2026-12-25
//	  - !<date> HH:MM sets a deadline at a time of day
//	  - !HH:MM        sets a deadline today, or tomorrow if that time has passed
//
// Everything else is the title.
package parser

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

// ErrBadDate is returned by ParseDue for input that is not a date or time.
var ErrBadDate = errors.New("invalid date")

// Result is a parsed quick-add line.
type Result struct {
	Title    string
	Tags     []string
	Priority domain.Priority
	Due      *time.Time
	AllDay   bool
}

// Task converts the result to a task to create.
func (r Result) Task() domain.NewTask {
	return domain.NewTask{Title: r.Title, Priority: r.Priority, Tags: r.Tags, DueAt: r.Due, DueAllDay: r.AllDay}
}

// Parse parses one quick-add line. now and loc define "today" and how dates are read.
func Parse(s string, now time.Time, loc *time.Location) (Result, error) {
	var (
		res   Result
		words []string
		tags  []string
	)
	now = now.In(loc)
	tokens := strings.Fields(s)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch {
		case strings.HasPrefix(tok, "#") && len(tok) > 1:
			tag, err := domain.NormalizeTag(tok)
			if err != nil {
				return res, err
			}
			if tag == "" {
				words = append(words, tok)
				continue
			}
			tags = append(tags, tag)

		case strings.Trim(tok, "!") == "":
			res.Priority = priority(len(tok))

		case strings.HasPrefix(tok, "!"):
			next := ""
			if i+1 < len(tokens) {
				next = tokens[i+1]
			}
			due, allDay, used, ok := parseDue(tok[1:], next, now, loc)
			if !ok {
				words = append(words, tok)
				continue
			}
			res.Due, res.AllDay = &due, allDay
			i += used - 1

		default:
			words = append(words, tok)
		}
	}

	title, err := domain.NormalizeTitle(strings.Join(words, " "))
	if err != nil {
		return res, err
	}
	res.Title = title
	// Deduplicate and validate the tag list as a whole.
	if res.Tags, err = domain.ParseTags(strings.Join(tags, " ")); err != nil {
		return res, err
	}
	return res, nil
}

func priority(bangs int) domain.Priority {
	switch {
	case bangs >= 3:
		return domain.PriorityHigh
	case bangs == 2:
		return domain.PriorityMed
	}
	return domain.PriorityLow
}

// connectives may separate a date and a time: "завтра в 18:00", "fri at 9:30".
var connectives = map[string]bool{"в": true, "во": true, "at": true}

// ParseDue parses a standalone deadline such as "завтра 18:00", "fri", "25.12"
// or "18:00". It returns the deadline and whether it has no time of day.
func ParseDue(s string, now time.Time, loc *time.Location) (time.Time, bool, error) {
	var tokens []string
	for _, f := range strings.Fields(s) {
		if !connectives[strings.ToLower(f)] {
			tokens = append(tokens, f)
		}
	}
	if len(tokens) == 0 || len(tokens) > 2 {
		return time.Time{}, false, ErrBadDate
	}
	next := ""
	if len(tokens) == 2 {
		next = tokens[1]
	}
	due, allDay, used, ok := parseDue(tokens[0], next, now.In(loc), loc)
	if !ok || used != len(tokens) {
		return time.Time{}, false, ErrBadDate
	}
	return due, allDay, nil
}

// parseDue reads a date (optionally followed by a time in next) or a time.
// used is the number of consumed tokens.
func parseDue(tok, next string, now time.Time, loc *time.Location) (due time.Time, allDay bool, used int, ok bool) {
	if day, ok := parseDate(tok, now); ok {
		if h, m, ok := parseTime(next); ok {
			return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc), false, 2, true
		}
		return domain.EndOfDay(day, loc), true, 1, true
	}
	if h, m, ok := parseTime(tok); ok {
		due := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc)
		if !due.After(now) {
			due = due.AddDate(0, 0, 1)
		}
		return due, false, 1, true
	}
	return time.Time{}, false, 0, false
}

var relativeDays = map[string]int{
	"today": 0, "сегодня": 0,
	"tomorrow": 1, "tmrw": 1, "завтра": 1,
	"послезавтра": 2,
}

var weekdays = map[string]time.Weekday{
	"mon": time.Monday, "monday": time.Monday, "пн": time.Monday, "понедельник": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday, "вт": time.Tuesday, "вторник": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday, "ср": time.Wednesday, "среда": time.Wednesday, "среду": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday, "чт": time.Thursday, "четверг": time.Thursday,
	"fri": time.Friday, "friday": time.Friday, "пт": time.Friday, "пятница": time.Friday, "пятницу": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday, "сб": time.Saturday, "суббота": time.Saturday, "субботу": time.Saturday,
	"sun": time.Sunday, "sunday": time.Sunday, "вс": time.Sunday, "воскресенье": time.Sunday,
}

var (
	dmyRe  = regexp.MustCompile(`^(\d{1,2})[./](\d{1,2})(?:[./](\d{2}|\d{4}))?$`)
	isoRe  = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	timeRe = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
)

// parseDate returns midnight of the named day in now's location.
// A weekday means its next occurrence after today; a day and month without a
// year that has already passed this year means next year.
func parseDate(tok string, now time.Time) (time.Time, bool) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	word := strings.ToLower(tok)
	if n, ok := relativeDays[word]; ok {
		return today.AddDate(0, 0, n), true
	}
	if wd, ok := weekdays[word]; ok {
		days := (int(wd) - int(today.Weekday()) + 7) % 7
		if days == 0 {
			days = 7
		}
		return today.AddDate(0, 0, days), true
	}

	var y, m, d int
	if g := isoRe.FindStringSubmatch(tok); g != nil {
		y, m, d = atoi(g[1]), atoi(g[2]), atoi(g[3])
	} else if g := dmyRe.FindStringSubmatch(tok); g != nil {
		d, m = atoi(g[1]), atoi(g[2])
		switch len(g[3]) {
		case 0:
			y = today.Year()
		case 2:
			y = 2000 + atoi(g[3])
		default:
			y = atoi(g[3])
		}
	} else {
		return time.Time{}, false
	}

	date := time.Date(y, time.Month(m), d, 0, 0, 0, 0, now.Location())
	if date.Day() != d || int(date.Month()) != m { // e.g. 31.02
		return time.Time{}, false
	}
	if g := dmyRe.FindStringSubmatch(tok); g != nil && g[3] == "" && date.Before(today) {
		date = date.AddDate(1, 0, 0)
	}
	return date, true
}

func parseTime(tok string) (h, m int, ok bool) {
	g := timeRe.FindStringSubmatch(tok)
	if g == nil {
		return 0, 0, false
	}
	h, m = atoi(g[1]), atoi(g[2])
	if h > 23 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
