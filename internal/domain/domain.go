// Package domain defines core entities and business rules.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Supported languages.
const (
	LangEN = "en"
	LangRU = "ru"
)

// ViewMode is how a project board is rendered.
type ViewMode string

const (
	ViewButtons ViewMode = "buttons"
	ViewTable   ViewMode = "table"
	ViewArticle ViewMode = "article"
)

// Priority of a task.
type Priority int

const (
	PriorityNone Priority = iota
	PriorityLow
	PriorityMed
	PriorityHigh
)

// MaxNameLen limits project and column names, in runes.
const MaxNameLen = 64

var (
	ErrNotFound       = errors.New("not found")
	ErrEmptyName      = errors.New("name is empty")
	ErrNameTooLong    = errors.New("name is too long")
	ErrInboxProtected = errors.New("inbox can only be renamed")
)

// User is a bot user with personal settings.
type User struct {
	TgID             int64
	Lang             string
	ViewMode         ViewMode
	Timezone         string
	DigestTime       string // "HH:MM", empty = disabled
	RemindBeforeMin  int
	LiveMessageID    int // 0 = none
	State            string
	CurrentProjectID int64 // 0 = none
	CreatedAt        time.Time
}

// Location returns the user's timezone, falling back to UTC.
func (u User) Location() *time.Location {
	if loc, err := time.LoadLocation(u.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

// Project groups tasks on a kanban board.
type Project struct {
	ID        int64
	UserID    int64
	Name      string
	Position  int
	Archived  bool
	IsInbox   bool
	CreatedAt time.Time
}

// ProjectSummary is a project with its open task count, for lists.
type ProjectSummary struct {
	Project
	OpenTasks int
}

// Column is a kanban column within a project.
type Column struct {
	ID        int64
	ProjectID int64
	Name      string
	Position  int
	IsDone    bool
}

// DefaultColumn describes a column created for every new project.
type DefaultColumn struct {
	Name   string
	IsDone bool
}

// DefaultColumns are the initial columns of a project when no localized
// names are given. Names are stored as is and can be renamed by the user.
var DefaultColumns = []DefaultColumn{
	{Name: "Todo"},
	{Name: "In progress"},
	{Name: "Done", IsDone: true},
}

// DueSummary counts open tasks by deadline.
type DueSummary struct {
	Overdue int
	Today   int
}

// NormalizeName trims and validates a project or column name.
func NormalizeName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", ErrEmptyName
	}
	if utf8.RuneCountInString(s) > MaxNameLen {
		return "", ErrNameTooLong
	}
	return s, nil
}

// LangFromCode maps a Telegram language_code to a supported language.
func LangFromCode(code string) string {
	code = strings.ToLower(code)
	if code == LangRU || strings.HasPrefix(code, LangRU+"-") {
		return LangRU
	}
	return LangEN
}

// viewCodes are compact view mode codes for callback data.
var viewCodes = map[ViewMode]string{ViewButtons: "b", ViewTable: "t", ViewArticle: "a"}

// Views lists view modes in display order.
var Views = []ViewMode{ViewButtons, ViewTable, ViewArticle}

// Code returns the compact code of the view mode.
func (v ViewMode) Code() string { return viewCodes[v] }

// ParseViewCode maps a compact code back to a view mode.
func ParseViewCode(code string) (ViewMode, bool) {
	for v, c := range viewCodes {
		if c == code {
			return v, true
		}
	}
	return "", false
}
