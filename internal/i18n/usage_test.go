package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/locales"
)

var (
	// lookupRe matches translation lookups with a literal key: l.T("home.title", ...).
	// Prefixes of dynamic keys ("view." + mode) end with a dot and are skipped.
	lookupRe = regexp.MustCompile(`\.T\("([a-z0-9_.]*[a-z0-9_])"`)
	// literalRe matches any lowercase string literal that could be a key; keys are also
	// passed around as arguments, e.g. taskPrompt(r, t, "task.prompt_title", nil).
	literalRe = regexp.MustCompile(`"([a-z][a-z0-9_.]*)"`)
)

// dynamicKeys are built at runtime, e.g. "view." + string(mode).
func dynamicKeys() []string {
	var keys []string
	for _, v := range domain.Views {
		keys = append(keys, "view."+string(v))
	}
	return keys
}

// scanKeys returns keys matched by re in non-test Go sources of the module.
func scanKeys(t *testing.T, re *regexp.Regexp) []string {
	t.Helper()
	root := filepath.Join("..", "..")
	seen := map[string]bool{}
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				seen[m[1]] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range dynamicKeys() {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func TestEveryUsedKeyIsTranslated(t *testing.T) {
	b, err := Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	used := scanKeys(t, lookupRe)
	if len(used) < 50 {
		t.Fatalf("found only %d keys; is the source scan broken?", len(used))
	}
	for _, k := range used {
		for _, lang := range b.Langs() {
			if _, ok := b.Raw(lang, k); !ok {
				t.Errorf("key %q is used in code but missing in %s.json", k, lang)
			}
		}
	}
}

func TestNoUnusedKeys(t *testing.T) {
	b, err := Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	used := scanKeys(t, literalRe)
	for _, k := range b.Keys(Fallback) {
		if !slices.Contains(used, k) {
			t.Errorf("key %q is not used in code", k)
		}
	}
}
