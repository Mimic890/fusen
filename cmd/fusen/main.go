// Command fusen runs the fusen Telegram bot.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mimic890/fusen/internal/bot"
	"github.com/Mimic890/fusen/internal/config"
	"github.com/Mimic890/fusen/internal/i18n"
	"github.com/Mimic890/fusen/internal/scheduler"
	"github.com/Mimic890/fusen/internal/store"
	"github.com/Mimic890/fusen/locales"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer st.Close()

	tr, err := i18n.Load(locales.FS)
	if err != nil {
		return fmt.Errorf("locales: %w", err)
	}

	b, err := bot.New(cfg, st, tr, log)
	if err != nil {
		return err
	}

	log.Info("fusen started", "db", cfg.DBPath, "default_tz", cfg.DefaultTZ, "allowlist", len(cfg.AllowedUserIDs))
	go scheduler.New(st, b, log).Run(ctx)
	b.Run(ctx)
	log.Info("fusen stopped")
	return nil
}
