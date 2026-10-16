package views

import (
	"math"
	"strings"
	"testing"
	"time"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
)

func TestNotificationScreensRender(t *testing.T) {
	const big = math.MaxInt64
	due := time.Now().Add(time.Hour)
	task := domain.Task{ID: big, ProjectID: 1, Title: "<call>", Priority: domain.PriorityHigh, DueAt: &due, Tags: []string{"a"}}
	allDay := task
	allDay.DueAllDay = true
	item := domain.DigestItem{Task: task, ProjectName: "P & Q"}
	many := make([]domain.DigestItem, 30)
	for i := range many {
		many[i] = item
	}
	card := Card{Task: task, Project: domain.Project{Name: "P"}, Column: domain.Column{Name: "C"}, Loc: time.UTC}
	projects := []domain.ProjectSummary{{Project: domain.Project{ID: 1, Name: "P"}}, {Project: domain.Project{ID: big, Name: "Q"}}}
	reminders := []domain.Reminder{
		{ID: big, RemindAt: due, Kind: domain.ReminderCustom},
		{ID: 2, RemindAt: due, Kind: domain.ReminderDeadline},
	}

	for _, lang := range []string{"en", "ru"} {
		l := localizer(t, lang)
		for name, s := range map[string]Screen{
			"deadline":     ReminderNotice(l, domain.ReminderJob{Reminder: domain.Reminder{ID: big, Kind: domain.ReminderDeadline}, Task: task, ProjectName: "P"}, time.UTC),
			"custom":       ReminderNotice(l, domain.ReminderJob{Reminder: domain.Reminder{ID: 1, Kind: domain.ReminderCustom}, Task: allDay}, time.UTC),
			"result":       NoticeResult(l, "done", big),
			"digest":       DigestNotice(l, domain.Digest{Overdue: many, Today: []domain.DigestItem{item}}, time.UTC),
			"quick one":    QuickAdded(l, []domain.Task{task}, domain.Project{Name: "P"}, domain.Column{ID: big, Name: "C"}, time.UTC),
			"quick many":   QuickAdded(l, []domain.Task{task, allDay}, domain.Project{Name: "P"}, domain.Column{ID: 1, Name: "C"}, time.UTC),
			"picker":       ProjectPicker(l, card, projects),
			"reminders":    Reminders(l, card, reminders),
			"no reminders": Reminders(l, card, nil),
		} {
			t.Run(lang+"/"+name, func(t *testing.T) { checkScreen(t, s) })
		}
	}
}

func TestDigestLimitsButtonsAndList(t *testing.T) {
	l := localizer(t, "en")
	item := domain.DigestItem{Task: domain.Task{ID: 1, Title: "x", DueAt: ptrTime(time.Now())}}
	many := make([]domain.DigestItem, 30)
	for i := range many {
		many[i] = item
	}
	s := DigestNotice(l, domain.Digest{Overdue: many}, time.UTC)
	if n := strings.Count(s.Text, "\n• "); n != digestListLimit {
		t.Errorf("listed %d tasks", n)
	}
	if !strings.Contains(s.Text, "…and 15 more") {
		t.Errorf("missing overflow line: %s", s.Text)
	}
	if len(s.Keyboard) != digestButtons+1 {
		t.Errorf("buttons = %d", len(s.Keyboard))
	}
}

func TestReminderNoticeActions(t *testing.T) {
	l := localizer(t, "en")
	j := domain.ReminderJob{Reminder: domain.Reminder{ID: 7, Kind: domain.ReminderDeadline}, Task: domain.Task{ID: 9, Title: "x"}}
	var kinds []string
	for _, r := range ReminderNotice(l, j, time.UTC).Keyboard {
		for _, b := range r {
			d, _ := cb.Decode(b.CallbackData)
			kinds = append(kinds, d.Kind+":"+strings.Join(d.Args, ":"))
		}
	}
	want := []string{"ra:7:d", "ra:7:h", "ra:7:t", "no:9"}
	if strings.Join(kinds, " ") != strings.Join(want, " ") {
		t.Errorf("buttons = %v", kinds)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
