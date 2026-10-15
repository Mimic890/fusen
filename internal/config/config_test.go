package config

import (
	"log/slog"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(map[string]string{"BOT_TOKEN": "123:abc"}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DBPath != "data/fusen.db" || cfg.DefaultTZ != "UTC" || cfg.LogLevel != slog.LevelInfo {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if !cfg.IsAllowed(42) {
		t.Error("empty allowlist must allow everyone")
	}
}

func TestLoadAllowlist(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"BOT_TOKEN":        "123:abc",
		"ALLOWED_USER_IDS": " 1, 2 ,,3",
		"LOG_LEVEL":        "debug",
		"DEFAULT_TZ":       "Europe/Moscow",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, id := range []int64{1, 2, 3} {
		if !cfg.IsAllowed(id) {
			t.Errorf("user %d must be allowed", id)
		}
	}
	if cfg.IsAllowed(4) {
		t.Error("user 4 must be denied")
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("log level = %v", cfg.LogLevel)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := map[string]map[string]string{
		"missing token": {},
		"bad tz":        {"BOT_TOKEN": "x", "DEFAULT_TZ": "Mars/Base"},
		"bad level":     {"BOT_TOKEN": "x", "LOG_LEVEL": "loud"},
		"bad ids":       {"BOT_TOKEN": "x", "ALLOWED_USER_IDS": "1,abc"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := load(env(m)); err == nil {
				t.Error("expected error")
			}
		})
	}
}
