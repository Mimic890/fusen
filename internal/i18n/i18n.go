// Package i18n provides translated UI strings loaded from embedded JSON files.
package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// Fallback is used when a language or key is missing.
const Fallback = "en"

// Bundle holds all loaded translations.
type Bundle struct {
	msgs map[string]map[string]string
}

// Load reads every <lang>.json in fsys. Each file is a flat object of key -> string.
func Load(fsys fs.FS) (*Bundle, error) {
	names, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}
	b := &Bundle{msgs: make(map[string]map[string]string)}
	for _, name := range names {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		m := make(map[string]string)
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("locale %s: %w", name, err)
		}
		b.msgs[strings.TrimSuffix(path.Base(name), ".json")] = m
	}
	if _, ok := b.msgs[Fallback]; !ok {
		return nil, fmt.Errorf("fallback locale %q is missing", Fallback)
	}
	return b, nil
}

// Langs returns the loaded language codes.
func (b *Bundle) Langs() []string {
	out := make([]string, 0, len(b.msgs))
	for l := range b.msgs {
		out = append(out, l)
	}
	return out
}

// Keys returns all keys of a language.
func (b *Bundle) Keys(lang string) []string {
	out := make([]string, 0, len(b.msgs[lang]))
	for k := range b.msgs[lang] {
		out = append(out, k)
	}
	return out
}

// Raw returns the untranslated template for a key.
func (b *Bundle) Raw(lang, key string) (string, bool) {
	s, ok := b.msgs[lang][key]
	return s, ok
}

// For returns a localizer bound to lang.
func (b *Bundle) For(lang string) Localizer {
	return Localizer{b: b, lang: lang}
}

// Localizer translates keys for one language.
type Localizer struct {
	b    *Bundle
	lang string
}

// Lang returns the bound language.
func (l Localizer) Lang() string { return l.lang }

// T returns the translation of key with {name} placeholders replaced by
// the given name/value pairs. Missing keys fall back to English, then to the key itself.
func (l Localizer) T(key string, kv ...any) string {
	s, ok := l.b.msgs[l.lang][key]
	if !ok {
		if s, ok = l.b.msgs[Fallback][key]; !ok {
			return key
		}
	}
	if len(kv) == 0 {
		return s
	}
	pairs := make([]string, 0, len(kv))
	for i := 0; i+1 < len(kv); i += 2 {
		pairs = append(pairs, "{"+fmt.Sprint(kv[i])+"}", fmt.Sprint(kv[i+1]))
	}
	return strings.NewReplacer(pairs...).Replace(s)
}
