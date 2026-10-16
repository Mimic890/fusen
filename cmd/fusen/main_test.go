package main

import (
	"strings"
	"testing"
)

func TestRunFailsOnInvalidConfig(t *testing.T) {
	t.Setenv("BOT_TOKEN", "")
	err := run()
	if err == nil || !strings.Contains(err.Error(), "BOT_TOKEN is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunFailsOnUnwritableDatabase(t *testing.T) {
	t.Setenv("BOT_TOKEN", "1:test")
	t.Setenv("DB_PATH", "/proc/fusen/fusen.db")
	err := run()
	if err == nil || !strings.HasPrefix(err.Error(), "store:") {
		t.Fatalf("err = %v", err)
	}
}
