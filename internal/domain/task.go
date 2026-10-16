package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Limits keep a task card within Telegram's 4096-character message limit.
const (
	MaxTitleLen       = 200
	MaxDescriptionLen = 1000
	MaxItemLen        = 100
	MaxChecklistItems = 20
	MaxTags           = 10
	MaxTagLen         = 32
	// MaxColumns matches the rich message table limit of 20 columns.
	MaxColumns = 20
	// MaxReminders limits custom reminders per task.
	MaxReminders = 10
)

var (
	ErrTitleTooLong       = errors.New("title is too long")
	ErrDescriptionTooLong = errors.New("description is too long")
	ErrItemTooLong        = errors.New("checklist item is too long")
	ErrTooManyItems       = errors.New("too many checklist items")
	ErrTooManyTags        = errors.New("too many tags")
	ErrTagTooLong         = errors.New("tag is too long")
	ErrColumnNotEmpty     = errors.New("column has tasks")
	ErrLastColumn         = errors.New("project must keep at least one column")
	ErrTooManyColumns     = errors.New("too many columns")
	ErrNoDoneColumn       = errors.New("project has no done column")
	ErrTooManyReminders   = errors.New("too many reminders")
	ErrPastTime           = errors.New("time has passed")
)

// Task is a card on a kanban board.
type Task struct {
	ID          int64
	ProjectID   int64
	ColumnID    int64
	Title       string
	Description string
	Priority    Priority
	DueAt       *time.Time
	DueAllDay   bool // DueAt is the end of a day without a specific time
	Position    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DoneAt      *time.Time
	Tags        []string
}

// IsOverdue reports whether an open task is past its deadline.
func (t Task) IsOverdue(now time.Time) bool {
	return t.DoneAt == nil && t.DueAt != nil && t.DueAt.Before(now)
}

// NewTask describes a task to create.
type NewTask struct {
	Title     string
	Priority  Priority
	Tags      []string
	DueAt     *time.Time
	DueAllDay bool
}

// Reminder kinds.
const (
	ReminderDeadline = "deadline"
	ReminderCustom   = "custom"
)

// Reminder is a scheduled notification about a task.
type Reminder struct {
	ID       int64
	TaskID   int64
	RemindAt time.Time
	SentAt   *time.Time
	Kind     string
}

// DeadlineReminderAt returns when to remind about a deadline, or false if no
// reminder is needed: all-day deadlines are covered by the morning digest,
// past deadlines need none, and before <= 0 disables reminders. If the
// deadline is closer than before, the reminder fires at the deadline itself
// rather than right away, since the user has just set it.
func DeadlineReminderAt(due time.Time, allDay bool, before int, now time.Time) (time.Time, bool) {
	if allDay || before <= 0 || !due.After(now) {
		return time.Time{}, false
	}
	at := due.Add(-time.Duration(before) * time.Minute)
	if !at.After(now) {
		at = due
	}
	return at, true
}

// ReminderJob is a due reminder with what is needed to deliver it.
type ReminderJob struct {
	Reminder
	Task        Task
	ProjectName string
	// Skip is set when the reminder is obsolete: the task is done or its project archived.
	Skip bool
	User User
}

// DigestItem is a task listed in the morning digest.
type DigestItem struct {
	Task        Task
	ProjectName string
}

// Digest is the morning summary of a user's open tasks.
type Digest struct {
	Overdue []DigestItem
	Today   []DigestItem
}

// Empty reports whether there is nothing to send.
func (d Digest) Empty() bool { return len(d.Overdue) == 0 && len(d.Today) == 0 }

// EndOfDay returns the last second of t's day in loc.
func EndOfDay(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 23, 59, 59, 0, loc)
}

// TaskSummary is a task with checklist progress, for board lists.
type TaskSummary struct {
	Task
	ChecklistDone  int
	ChecklistTotal int
}

// ChecklistItem is a sub-step of a task.
type ChecklistItem struct {
	ID       int64
	TaskID   int64
	Text     string
	Done     bool
	Position int
}

// ColumnSummary is a column with its task count.
type ColumnSummary struct {
	Column
	Tasks int
}

// Direction of a move between neighbouring columns or positions.
type Direction int

const (
	Left  Direction = -1
	Right Direction = 1
)

// collapse trims text and collapses all whitespace runs to single spaces.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func checkLen(s string, max int, tooLong error) (string, error) {
	if s == "" {
		return "", ErrEmptyName
	}
	if utf8.RuneCountInString(s) > max {
		return "", tooLong
	}
	return s, nil
}

// NormalizeTitle validates a task title.
func NormalizeTitle(s string) (string, error) {
	return checkLen(collapse(s), MaxTitleLen, ErrTitleTooLong)
}

// NormalizeItem validates a checklist item.
func NormalizeItem(s string) (string, error) {
	return checkLen(collapse(s), MaxItemLen, ErrItemTooLong)
}

// NormalizeDescription trims a description, keeping line breaks.
func NormalizeDescription(s string) (string, error) {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRightFunc(l, unicode.IsSpace)
	}
	s = strings.Join(lines, "\n")
	return checkLen(s, MaxDescriptionLen, ErrDescriptionTooLong)
}

// SplitLines returns non-empty trimmed lines, for adding several items at once.
func SplitLines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(s, "\n") {
		if l = collapse(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// ParseTags parses "#home, urgent #Work" into normalized unique tag names.
func ParseTags(s string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	fields := strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == ',' })
	for _, f := range fields {
		tag, err := NormalizeTag(f)
		if err != nil {
			return nil, err
		}
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	if len(out) > MaxTags {
		return nil, ErrTooManyTags
	}
	return out, nil
}

// NormalizeTag lowercases a tag and strips the leading '#' and any characters
// other than letters, digits, '_' and '-'. It returns "" for an empty tag.
func NormalizeTag(s string) (string, error) {
	s = strings.ToLower(strings.TrimLeft(s, "#"))
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			return r
		}
		return -1
	}, s)
	if utf8.RuneCountInString(s) > MaxTagLen {
		return "", ErrTagTooLong
	}
	return s, nil
}
