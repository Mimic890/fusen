// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the bot runtime configuration.
type Config struct {
	BotToken string
	DBPath   string
	// DefaultTZ is assigned to new users until they pick their own timezone.
	DefaultTZ string
	LogLevel  slog.Level
	// AllowedUserIDs restricts access to these Telegram user IDs. Empty means open to everyone.
	AllowedUserIDs map[int64]struct{}
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	cfg := Config{
		BotToken:  strings.TrimSpace(getenv("BOT_TOKEN")),
		DBPath:    envOr(getenv, "DB_PATH", "data/fusen.db"),
		DefaultTZ: envOr(getenv, "DEFAULT_TZ", "UTC"),
	}

	var errs []error

	if cfg.BotToken == "" {
		errs = append(errs, errors.New("BOT_TOKEN is required"))
	}

	if _, err := time.LoadLocation(cfg.DefaultTZ); err != nil {
		errs = append(errs, fmt.Errorf("DEFAULT_TZ: %w", err))
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(envOr(getenv, "LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}

	ids, err := parseIDs(getenv("ALLOWED_USER_IDS"))
	if err != nil {
		errs = append(errs, fmt.Errorf("ALLOWED_USER_IDS: %w", err))
	}
	cfg.AllowedUserIDs = ids

	return cfg, errors.Join(errs...)
}

// IsAllowed reports whether the user may use the bot.
func (c Config) IsAllowed(userID int64) bool {
	if len(c.AllowedUserIDs) == 0 {
		return true
	}
	_, ok := c.AllowedUserIDs[userID]
	return ok
}

func envOr(getenv func(string) string, key, def string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return def
}

func parseIDs(s string) (map[int64]struct{}, error) {
	ids := make(map[int64]struct{})
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user id %q", part)
		}
		ids[id] = struct{}{}
	}
	return ids, nil
}
