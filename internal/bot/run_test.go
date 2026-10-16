package bot

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"

	"github.com/Mimic890/fusen/internal/config"
	"github.com/Mimic890/fusen/internal/i18n"
	"github.com/Mimic890/fusen/internal/store"
	"github.com/Mimic890/fusen/locales"
)

func TestNewAndRunRegisterCommands(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tr, _ := i18n.Load(locales.FS)
	api := &fakeAPI{rateLimit: map[string]int{}}
	srv := httptest.NewServer(api)
	defer srv.Close()

	// Without WithSkipGetMe: New verifies the token with getMe.
	b, err := newBot(config.Config{BotToken: "1:test"}, st, tr, slog.New(slog.NewTextHandler(io.Discard, nil)),
		tgbot.WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { b.Run(ctx); close(done) }()

	deadline := time.After(5 * time.Second)
	for {
		api.mu.Lock()
		var cmds []apiCall
		polled := false
		for _, c := range api.calls {
			switch c.method {
			case "setMyCommands":
				cmds = append(cmds, c)
			case "getUpdates":
				polled = true
			}
		}
		api.mu.Unlock()
		if len(cmds) == 2 && polled {
			if cmds[0].params["language_code"] != "" || cmds[1].params["language_code"] != "ru" ||
				!strings.Contains(cmds[1].params["commands"], "Настройки") {
				t.Errorf("commands: %+v", cmds)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("commands not registered or polling not started: %s", methods(api.calls))
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

func TestNewFailsOnBadToken(t *testing.T) {
	srv := httptest.NewServer(nil) // 404 for getMe
	defer srv.Close()
	tr, _ := i18n.Load(locales.FS)
	_, err := newBot(config.Config{BotToken: "1:bad"}, nil, tr, slog.New(slog.NewTextHandler(io.Discard, nil)),
		tgbot.WithServerURL(srv.URL), tgbot.WithCheckInitTimeout(2*time.Second))
	if err == nil {
		t.Fatal("expected an error for an invalid token")
	}
}
