//go:build live

// Live smoke test against the real Bot API. It sends every board view to a
// chat, edits it through plain and rich content, and deletes the messages.
//
//	BOT_TOKEN=... FUSEN_LIVE_CHAT_ID=... go test -tags live -run Live ./internal/bot/
package bot

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"

	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/i18n"
	"github.com/Mimic890/fusen/internal/views"
	"github.com/Mimic890/fusen/locales"
)

func liveBoard(perColumn []int, titleLen int) views.RichBoard {
	b := views.RichBoard{Project: domain.Project{ID: 1, Name: "Live <smoke> & test"}, Now: time.Now(), Loc: time.UTC}
	due := time.Now().Add(-time.Hour)
	id := int64(1)
	for i, n := range perColumn {
		c := domain.ColumnSummary{Column: domain.Column{ID: int64(100 + i), Name: "Col " + strconv.Itoa(i), IsDone: i == len(perColumn)-1}, Tasks: n}
		b.Columns = append(b.Columns, c)
		for range n {
			b.Tasks = append(b.Tasks, domain.TaskSummary{
				Task: domain.Task{ID: id, ColumnID: c.ID, Title: strings.Repeat("я", titleLen) + " #x <b>",
					Priority: domain.PriorityHigh, DueAt: &due, Tags: []string{"home", "work"}},
				ChecklistDone: 1, ChecklistTotal: 3,
			})
			id++
		}
	}
	return b
}

func TestLiveRichViews(t *testing.T) {
	token := os.Getenv("BOT_TOKEN")
	chat, _ := strconv.ParseInt(os.Getenv("FUSEN_LIVE_CHAT_ID"), 10, 64)
	if token == "" || chat == 0 {
		t.Skip("BOT_TOKEN and FUSEN_LIVE_CHAT_ID are required")
	}
	api, err := tgbot.New(token, tgbot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	b := &Bot{api: api, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	tr, err := i18n.Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	l := tr.For("ru")
	ctx := context.Background()

	maxCols := make([]int, domain.MaxColumns)
	for i := range maxCols {
		maxCols[i] = 100
	}
	screens := map[string]views.Screen{
		"table empty":     views.BoardTable(l, liveBoard([]int{0, 0, 0}, 5)),
		"table small":     views.BoardTable(l, liveBoard([]int{3, 1, 2}, 10)),
		"table overflow":  views.BoardTable(l, liveBoard([]int{40, 0, 2}, 10)),
		"table max":       views.BoardTable(l, liveBoard(maxCols, domain.MaxTitleLen)),
		"article small":   views.BoardArticle(l, liveBoard([]int{3, 0, 2}, 10)),
		"article max":     views.BoardArticle(l, liveBoard(maxCols, domain.MaxTitleLen)),
		"article notice":  views.BoardArticle(l, liveBoard([]int{1, 1}, 10)).WithNotice("Сохранено."),
		"buttons (plain)": views.BoardButtons(l, views.Board{Project: domain.Project{ID: 1, Name: "P"}, Columns: liveBoard([]int{1}, 3).Columns}),
		"card with tg-time": views.TaskCard(l, views.Card{Task: liveBoard([]int{1}, 20).Tasks[0].Task,
			Project: domain.Project{Name: "P"}, Column: domain.Column{Name: "C"}, Loc: time.UTC, Now: time.Now()}),
		"settings": views.Settings(l, domain.User{Lang: "ru", ViewMode: domain.ViewTable, Timezone: "Asia/Kolkata",
			DigestTime: "09:00", RemindBeforeMin: 90}, time.Now()),
		"timezone picker": views.TimezonePicker(l, domain.User{Timezone: "UTC"}, time.Now()),
	}
	task := liveBoard([]int{1}, 20).Tasks[0].Task
	soon := time.Now().Add(time.Hour)
	task.DueAt = &soon
	allDay := task
	allDay.DueAllDay = true
	item := domain.DigestItem{Task: task, ProjectName: "P"}
	card := views.Card{Task: task, Project: domain.Project{Name: "P"}, Column: domain.Column{Name: "C"}, Loc: time.UTC}
	for name, s := range map[string]views.Screen{
		"reminder notice": views.ReminderNotice(l, domain.ReminderJob{Reminder: domain.Reminder{ID: 1, Kind: domain.ReminderDeadline},
			Task: task, ProjectName: "P"}, time.UTC),
		"digest":      views.DigestNotice(l, domain.Digest{Overdue: []domain.DigestItem{item}, Today: []domain.DigestItem{{Task: allDay, ProjectName: "P"}}}, time.UTC),
		"quick added": views.QuickAdded(l, []domain.Task{task, allDay}, domain.Project{Name: "P"}, domain.Column{ID: 1, Name: "C"}, time.UTC),
		"reminders":   views.Reminders(l, card, []domain.Reminder{{ID: 1, RemindAt: soon, Kind: domain.ReminderCustom}, {ID: 2, RemindAt: soon, Kind: domain.ReminderDeadline}}),
	} {
		screens[name] = s
	}

	var sent []int
	defer func() {
		for _, id := range sent {
			_ = b.deleteMessage(ctx, chat, id)
		}
	}()
	plain := views.Screen{Text: "plain"}
	for name, s := range screens {
		id, err := b.sendScreen(ctx, chat, s)
		if err != nil {
			t.Errorf("%s: send: %v", name, err)
			continue
		}
		sent = append(sent, id)
		// The live screen switches between plain and rich content in place.
		if err := b.editScreen(ctx, chat, id, plain); err != nil {
			t.Errorf("%s: edit to plain: %v", name, err)
		}
		if err := b.editScreen(ctx, chat, id, s); err != nil {
			t.Errorf("%s: edit back: %v", name, err)
		}
		if err := b.editScreen(ctx, chat, id, s); err != nil {
			t.Errorf("%s: unchanged edit must be a no-op: %v", name, err)
		}
	}
}
