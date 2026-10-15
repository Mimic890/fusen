package i18n

import (
	"regexp"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/Mimic890/fusen/locales"
)

var placeholderRe = regexp.MustCompile(`\{[a-z_]+\}`)

func TestLocalesConsistent(t *testing.T) {
	b, err := Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "ru"} {
		if len(b.Keys(lang)) == 0 {
			t.Fatalf("locale %s is empty", lang)
		}
	}

	en := b.Keys("en")
	slices.Sort(en)
	for _, lang := range b.Langs() {
		keys := b.Keys(lang)
		slices.Sort(keys)
		if !slices.Equal(keys, en) {
			t.Errorf("locale %s keys differ from en:\n missing: %v\n extra: %v",
				lang, diff(en, keys), diff(keys, en))
			continue
		}
		for _, k := range en {
			want, _ := b.Raw("en", k)
			got, _ := b.Raw(lang, k)
			if !sameSet(placeholderRe.FindAllString(want, -1), placeholderRe.FindAllString(got, -1)) {
				t.Errorf("locale %s key %s: placeholders differ from en", lang, k)
			}
		}
	}
}

func TestT(t *testing.T) {
	b, err := Load(fstest.MapFS{
		"en.json": {Data: []byte(`{"hi": "Hi, {name}!", "only_en": "en"}`)},
		"ru.json": {Data: []byte(`{"hi": "Привет, {name}!"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	ru := b.For("ru")
	if got := ru.T("hi", "name", "Bob"); got != "Привет, Bob!" {
		t.Errorf("got %q", got)
	}
	if got := ru.T("only_en"); got != "en" {
		t.Errorf("fallback: got %q", got)
	}
	if got := b.For("de").T("hi", "name", 1); got != "Hi, 1!" {
		t.Errorf("unknown lang: got %q", got)
	}
	if got := ru.T("missing"); got != "missing" {
		t.Errorf("missing key: got %q", got)
	}
}

func TestLoadRequiresFallback(t *testing.T) {
	if _, err := Load(fstest.MapFS{"ru.json": {Data: []byte(`{}`)}}); err == nil {
		t.Error("expected error without en.json")
	}
}

func diff(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}

func sameSet(a, b []string) bool {
	return len(diff(a, b)) == 0 && len(diff(b, a)) == 0
}
