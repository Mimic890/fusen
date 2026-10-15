package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Mimic890/fusen/internal/domain"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "sub", "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newUser(t *testing.T, s *Store, id int64) domain.User {
	t.Helper()
	u, created, err := s.EnsureUser(context.Background(), NewUser{TgID: id, Lang: "ru", Timezone: "UTC", InboxName: "Входящие"})
	if err != nil || !created {
		t.Fatalf("ensure user: created=%v err=%v", created, err)
	}
	return u
}

func addTask(t *testing.T, s *Store, projectID, columnID int64, due *time.Time) {
	t.Helper()
	var dueAt any
	if due != nil {
		dueAt = due.Unix()
	}
	now := time.Now().Unix()
	if _, err := s.db.Exec(`INSERT INTO tasks (project_id, column_id, title, position, due_at, created_at, updated_at)
		VALUES (?, ?, 'x', 0, ?, ?, ?)`, projectID, columnID, dueAt, now, now); err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	fsys := fstest.MapFS{"0001_init.sql": {Data: []byte("SELECT 1")}}
	// Version 1 is already applied, so this must be a no-op.
	if err := migrate(ctx, s.db, fsys); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("schema_migrations rows = %d, err %v", n, err)
	}
}

func TestMigrateRejectsBadNames(t *testing.T) {
	for _, fsys := range []fstest.MapFS{
		{"init.sql": {}},
		{"abc_init.sql": {}},
		{"0001_a.sql": {}, "1_b.sql": {}},
	} {
		if _, err := loadMigrations(fsys); err == nil {
			t.Errorf("%v: expected error", fsys)
		}
	}
}

func TestEnsureUserCreatesInbox(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := newUser(t, s, 100)
	if u.Lang != "ru" || u.ViewMode != domain.ViewButtons || u.RemindBeforeMin != 60 {
		t.Errorf("unexpected user defaults: %+v", u)
	}

	if _, created, err := s.EnsureUser(ctx, NewUser{TgID: 100, Lang: "en", Timezone: "UTC", InboxName: "Inbox"}); err != nil || created {
		t.Fatalf("second ensure: created=%v err=%v", created, err)
	}

	ps, err := s.ListProjects(ctx, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || !ps[0].IsInbox || ps[0].Name != "Входящие" {
		t.Fatalf("projects = %+v", ps)
	}
	cols, err := s.ListColumns(ctx, ps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 || cols[0].Name != "Todo" || !cols[2].IsDone {
		t.Fatalf("columns = %+v", cols)
	}
}

func TestUserFields(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	newUser(t, s, 1)
	p, _ := s.CreateProject(ctx, 1, "A", nil)

	if err := s.SetLiveMessage(ctx, 1, 55); err != nil {
		t.Fatal(err)
	}
	if err := s.SetState(ctx, 1, "pn"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrentProject(ctx, 1, p.ID); err != nil {
		t.Fatal(err)
	}
	u, _ := s.GetUser(ctx, 1)
	if u.LiveMessageID != 55 || u.State != "pn" || u.CurrentProjectID != p.ID {
		t.Fatalf("user = %+v", u)
	}

	// Archiving the current project resets it.
	if err := s.SetProjectArchived(ctx, 1, p.ID, true); err != nil {
		t.Fatal(err)
	}
	_ = s.SetLiveMessage(ctx, 1, 0)
	u, _ = s.GetUser(ctx, 1)
	if u.LiveMessageID != 0 || u.CurrentProjectID != 0 {
		t.Fatalf("user = %+v", u)
	}
}

func TestProjectLifecycle(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	newUser(t, s, 1)
	newUser(t, s, 2)

	a, err := s.CreateProject(ctx, 1, "A", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.CreateProject(ctx, 1, "B", nil)
	if b.Position <= a.Position {
		t.Errorf("positions: a=%d b=%d", a.Position, b.Position)
	}

	if err := s.RenameProject(ctx, 1, a.ID, "A2"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameProject(ctx, 2, a.ID, "hijack"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign rename: %v", err)
	}
	if _, err := s.GetProject(ctx, 2, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign get: %v", err)
	}

	if err := s.SetProjectArchived(ctx, 1, b.ID, true); err != nil {
		t.Fatal(err)
	}
	active, _ := s.ListProjects(ctx, 1, false)
	archived, _ := s.ListProjects(ctx, 1, true)
	if len(active) != 2 || active[1].Name != "A2" || len(archived) != 1 || archived[0].ID != b.ID {
		t.Fatalf("active=%+v archived=%+v", active, archived)
	}
	if n, _ := s.CountArchived(ctx, 1); n != 1 {
		t.Errorf("archived count = %d", n)
	}
}

func TestInboxProtected(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	newUser(t, s, 1)
	ps, _ := s.ListProjects(ctx, 1, false)
	inbox := ps[0].ID

	if err := s.SetProjectArchived(ctx, 1, inbox, true); !errors.Is(err, domain.ErrInboxProtected) {
		t.Errorf("archive inbox: %v", err)
	}
	if err := s.DeleteProject(ctx, 1, inbox); !errors.Is(err, domain.ErrInboxProtected) {
		t.Errorf("delete inbox: %v", err)
	}
	if err := s.RenameProject(ctx, 1, inbox, "Mail"); err != nil {
		t.Errorf("rename inbox: %v", err)
	}
}

func TestDeleteProjectCascades(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	newUser(t, s, 1)
	p, _ := s.CreateProject(ctx, 1, "A", nil)
	cols, _ := s.ListColumns(ctx, p.ID)
	addTask(t, s, p.ID, cols[0].ID, nil)

	if err := s.DeleteProject(ctx, 1, p.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM tasks) + (SELECT COUNT(*) FROM columns WHERE project_id = ?)`, p.ID).Scan(&n)
	if n != 0 {
		t.Errorf("leftover rows: %d", n)
	}
}

func TestOpenTasksAndDueSummary(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	newUser(t, s, 1)
	p, _ := s.CreateProject(ctx, 1, "A", nil)
	cols, _ := s.ListColumns(ctx, p.ID)
	todo, done := cols[0].ID, cols[2].ID

	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, loc)
	past := now.Add(-time.Hour)
	later := now.Add(5 * time.Hour)
	tomorrow := now.Add(13 * time.Hour)

	addTask(t, s, p.ID, todo, &past)
	addTask(t, s, p.ID, todo, &later)
	addTask(t, s, p.ID, todo, &tomorrow)
	addTask(t, s, p.ID, todo, nil)
	addTask(t, s, p.ID, done, &past) // done tasks are not counted

	ps, _ := s.ListProjects(ctx, 1, false)
	if ps[1].OpenTasks != 4 {
		t.Errorf("open tasks = %d", ps[1].OpenTasks)
	}
	ds, err := s.DueSummary(ctx, 1, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Overdue != 1 || ds.Today != 1 {
		t.Errorf("summary = %+v", ds)
	}

	_ = s.SetProjectArchived(ctx, 1, p.ID, true)
	if ds, _ = s.DueSummary(ctx, 1, now, loc); ds != (domain.DueSummary{}) {
		t.Errorf("archived project counted: %+v", ds)
	}
}

func TestSettings(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := newUser(t, s, 1)
	if u.DigestTime != domain.DefaultDigestTime {
		t.Errorf("default digest = %q", u.DigestTime)
	}

	if err := s.SetLang(ctx, 1, "en", "Входящие", "Inbox"); err != nil {
		t.Fatal(err)
	}
	_ = s.SetTimezone(ctx, 1, "Europe/Moscow")
	_ = s.SetDigestTime(ctx, 1, "")
	_ = s.SetRemindBefore(ctx, 1, 15)
	u, _ = s.GetUser(ctx, 1)
	if u.Lang != "en" || u.Timezone != "Europe/Moscow" || u.DigestTime != "" || u.RemindBeforeMin != 15 {
		t.Fatalf("user = %+v", u)
	}
	ps, _ := s.ListProjects(ctx, 1, false)
	if ps[0].Name != "Inbox" {
		t.Errorf("inbox not renamed: %q", ps[0].Name)
	}

	// A custom inbox name survives a language change.
	_ = s.RenameProject(ctx, 1, ps[0].ID, "Mail")
	_ = s.SetLang(ctx, 1, "ru", "Inbox", "Входящие")
	if ps, _ = s.ListProjects(ctx, 1, false); ps[0].Name != "Mail" {
		t.Errorf("custom inbox name lost: %q", ps[0].Name)
	}
	if err := s.SetLang(ctx, 1, "de", "", ""); err == nil {
		t.Error("CHECK constraint must reject unknown languages")
	}
}
