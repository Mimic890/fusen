package bot

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbot "github.com/go-telegram/bot"

	"github.com/Mimic890/fusen/internal/views"
)

// sendWith sends a message through a server that always answers with status and body.
func sendWith(t *testing.T, status int, body string) error {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	defer srv.Close()
	return sendTo(t, srv.URL)
}

func sendTo(t *testing.T, url string) error {
	t.Helper()
	api, err := tgbot.New("1:test", tgbot.WithServerURL(url), tgbot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	b := &Bot{api: api, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	stubSleep(t)
	_, err = b.sendScreen(context.Background(), 1, views.Screen{Text: "x"})
	return err
}

func TestIsTemporary(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"server error", 500, `{"ok":false,"error_code":500,"description":"Internal Server Error"}`, true},
		{"bad gateway page", 502, `<html>Bad Gateway</html>`, true},
		{"rate limited", 429, `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":1}}`, true},
		{"blocked by user", 403, `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`, false},
		{"bad request", 400, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := sendWith(t, c.status, c.body)
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := IsTemporary(err); got != c.want {
				t.Errorf("IsTemporary(%v) = %v, want %v", err, got, c.want)
			}
		})
	}

	t.Run("network", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close() // connection refused
		err := sendTo(t, url)
		if err == nil || !IsTemporary(err) {
			t.Errorf("network error %v: temporary = %v", err, IsTemporary(err))
		}
	})

	if IsTemporary(nil) || IsTemporary(context.Canceled) {
		t.Error("nil and cancellation are not temporary")
	}
}
