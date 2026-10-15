package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		in   string
		want string
		err  error
	}{
		{"  Home  lab ", "Home lab", nil},
		{"Дом\nи\tдача", "Дом и дача", nil},
		{"   ", "", ErrEmptyName},
		{strings.Repeat("я", MaxNameLen), strings.Repeat("я", MaxNameLen), nil},
		{strings.Repeat("я", MaxNameLen+1), "", ErrNameTooLong},
	}
	for _, c := range cases {
		got, err := NormalizeName(c.in)
		if !errors.Is(err, c.err) || got != c.want {
			t.Errorf("NormalizeName(%q) = %q, %v; want %q, %v", c.in, got, err, c.want, c.err)
		}
	}
}

func TestLangFromCode(t *testing.T) {
	cases := map[string]string{
		"ru": LangRU, "ru-RU": LangRU, "RU": LangRU,
		"en": LangEN, "de": LangEN, "": LangEN, "rus": LangEN,
	}
	for in, want := range cases {
		if got := LangFromCode(in); got != want {
			t.Errorf("LangFromCode(%q) = %q, want %q", in, got, want)
		}
	}
}
