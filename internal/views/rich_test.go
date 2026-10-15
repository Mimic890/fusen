package views

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
)

var (
	dataAttrRe = regexp.MustCompile(`data="([^"]*)"`)
	tagRe      = regexp.MustCompile(`</?([a-z0-9-]+)[^>]*>`)
	// Tags that open a block counted toward the 500-block limit.
	blockRe = regexp.MustCompile(`<(p|h[1-6]|ul|li|tr|details|table)[ >]`)
)

// checkRich validates rich HTML: balanced tags, decodable button data and the block limit.
func checkRich(t *testing.T, s Screen) {
	t.Helper()
	if !s.IsRich() {
		t.Fatal("screen is not rich")
	}
	for _, m := range dataAttrRe.FindAllStringSubmatch(s.Rich, -1) {
		if _, err := cb.Decode(m[1]); err != nil {
			t.Errorf("rich button data %q: %v", m[1], err)
		}
	}
	var stack []string
	for _, m := range tagRe.FindAllStringSubmatch(s.Rich, -1) {
		name := m[1]
		if strings.HasPrefix(m[0], "</") {
			if len(stack) == 0 || stack[len(stack)-1] != name {
				t.Fatalf("unbalanced </%s> in %s", name, s.Rich)
			}
			stack = stack[:len(stack)-1]
			continue
		}
		stack = append(stack, name)
	}
	if len(stack) > 0 {
		t.Fatalf("unclosed tags %v", stack)
	}
	if n := len(blockRe.FindAllString(s.Rich, -1)); n > 500 {
		t.Errorf("%d blocks exceed the rich message limit", n)
	}
	checkScreen(t, Screen{Text: "x", Keyboard: s.Keyboard})
}

func richBoard(tasksPerColumn ...int) RichBoard {
	b := RichBoard{Project: domain.Project{ID: 1, Name: "Home <lab>"}, Now: time.Now(), Loc: time.UTC}
	due := time.Now().Add(time.Hour)
	id := int64(1)
	for i, n := range tasksPerColumn {
		col := domain.ColumnSummary{Column: domain.Column{ID: int64(100 + i), Name: fmt.Sprintf("C%d", i), IsDone: i == len(tasksPerColumn)-1}, Tasks: n}
		b.Columns = append(b.Columns, col)
		for j := range n {
			b.Tasks = append(b.Tasks, domain.TaskSummary{
				Task:           domain.Task{ID: id, ColumnID: col.ID, Title: fmt.Sprintf("task %d & <x>", j), DueAt: &due, Tags: []string{"home"}},
				ChecklistDone:  1,
				ChecklistTotal: 2,
			})
			id++
		}
	}
	return b
}

func TestRichBoardsRender(t *testing.T) {
	for _, lang := range []string{"en", "ru"} {
		l := localizer(t, lang)
		for _, sizes := range [][]int{{0, 0, 0}, {3, 1, 2}, {40, 0, 5}, {1}} {
			b := richBoard(sizes...)
			t.Run(fmt.Sprintf("%s/table/%v", lang, sizes), func(t *testing.T) { checkRich(t, BoardTable(l, b)) })
			t.Run(fmt.Sprintf("%s/article/%v", lang, sizes), func(t *testing.T) { checkRich(t, BoardArticle(l, b)) })
		}
	}
}

// visibleLen approximates the rendered text length: tags stripped, entities decoded.
func visibleLen(rich string) int {
	text := html.UnescapeString(tagRe.ReplaceAllString(rich, ""))
	return utf8.RuneCountInString(text)
}

func TestRichBoardsFitLimitsAtMaxSize(t *testing.T) {
	l := localizer(t, "en")
	sizes := make([]int, domain.MaxColumns)
	for i := range sizes {
		sizes[i] = 100
	}
	b := richBoard(sizes...)
	// Worst case: maximum title length and tag count everywhere.
	tags := make([]string, domain.MaxTags)
	for i := range tags {
		tags[i] = strings.Repeat("t", domain.MaxTagLen)
	}
	for i := range b.Tasks {
		b.Tasks[i].Title = strings.Repeat("я", domain.MaxTitleLen)
		b.Tasks[i].Tags = tags
	}
	for name, s := range map[string]Screen{"table": BoardTable(l, b), "article": BoardArticle(l, b)} {
		checkRich(t, s)
		if n := visibleLen(s.Rich); n > 32768 {
			t.Errorf("%s: visible text %d exceeds 32768", name, n)
		}
	}
}

func TestShorten(t *testing.T) {
	if got := shorten("абвгд", 3); got != "аб…" {
		t.Errorf("got %q", got)
	}
	if got := shorten("abc", 3); got != "abc" {
		t.Errorf("got %q", got)
	}
}

func TestBoardTable(t *testing.T) {
	l := localizer(t, "en")
	s := BoardTable(l, richBoard(TableRowLimit+5, 2, 1))

	if !strings.Contains(s.Rich, "<h3>Home &lt;lab&gt;</h3>") {
		t.Errorf("project name not escaped: %s", s.Rich)
	}
	if got := strings.Count(s.Rich, "<tr>"); got != TableRowLimit+1 {
		t.Errorf("rows = %d, want header + %d", got, TableRowLimit)
	}
	if !strings.Contains(s.Rich, "<th>C0 · 20</th>") {
		t.Errorf("header: %s", s.Rich)
	}
	more := cb.New(cb.KindView, 100, domain.ViewButtons.Code()).String()
	if !strings.Contains(s.Rich, `data="`+more+`">+6 more<`) {
		t.Errorf("missing more button for the long column: %s", s.Rich)
	}
	if strings.Contains(s.Rich, cb.New(cb.KindView, 101, domain.ViewButtons.Code()).String()) {
		t.Error("short column must not get a more button")
	}
	if !strings.Contains(s.Rich, `data="t:1">task 0 &amp; &lt;x&gt;</tg-button>`) {
		t.Errorf("task button: %s", s.Rich)
	}
}

func TestBoardArticle(t *testing.T) {
	l := localizer(t, "ru")
	b := richBoard(2, 0, 1)
	s := BoardArticle(l, b)

	if !strings.Contains(s.Rich, "<h4>C0 · 2</h4><ul><li>") {
		t.Errorf("open column section: %s", s.Rich)
	}
	if !strings.Contains(s.Rich, "<h4>C1 · 0</h4><p><i>") {
		t.Errorf("empty column section: %s", s.Rich)
	}
	if !strings.Contains(s.Rich, "<details><summary>C2 · 1</summary><ul>") {
		t.Errorf("done column must be collapsed: %s", s.Rich)
	}
	unix := b.Tasks[0].DueAt.Unix()
	if !strings.Contains(s.Rich, fmt.Sprintf(`<tg-time unix="%d" format="wdt">`, unix)) {
		t.Errorf("deadline must use tg-time: %s", s.Rich)
	}
	if !strings.Contains(s.Rich, "#home · ☑ 1/2") {
		t.Errorf("tags and progress: %s", s.Rich)
	}
}

func TestViewSwitchMarksCurrent(t *testing.T) {
	l := localizer(t, "en")
	b := richBoard(1, 1)
	cases := map[domain.ViewMode]Screen{
		domain.ViewTable:   BoardTable(l, b),
		domain.ViewArticle: BoardArticle(l, b),
		domain.ViewButtons: BoardButtons(l, Board{Project: b.Project, Columns: b.Columns}),
	}
	for want, s := range cases {
		var marked []string
		for _, r := range s.Keyboard {
			for _, btn := range r {
				d, _ := cb.Decode(btn.CallbackData)
				if d.Kind == cb.KindView && strings.HasPrefix(btn.Text, "• ") {
					marked = append(marked, d.Arg(1))
				}
			}
		}
		if len(marked) != 1 || marked[0] != want.Code() {
			t.Errorf("%s: marked %v", want, marked)
		}
	}
}

func TestRichNotice(t *testing.T) {
	s := Screen{Rich: "<p>x</p>"}.WithNotice("<saved>")
	if s.Rich != "<p><i>&lt;saved&gt;</i></p><p>x</p>" || s.Text != "" {
		t.Errorf("notice: %+v", s)
	}
}
