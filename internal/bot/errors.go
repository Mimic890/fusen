package bot

import (
	"context"
	"errors"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
)

// statusRe extracts the status of errors the client does not type, such as 5xx.
var statusRe = regexp.MustCompile(`error response from telegram for method \S+, (\d{3}) `)

// IsTemporary reports whether a failed Bot API call may succeed if retried:
// network errors, rate limiting (429) and server errors (5xx, including
// non-JSON error pages from a proxy). Client errors such as a blocked bot
// (403) or a bad request (400) are permanent.
func IsTemporary(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var tooMany *tgbot.TooManyRequestsError
	var netErr net.Error
	switch {
	case errors.As(err, &tooMany), errors.As(err, &netErr),
		errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, context.DeadlineExceeded):
		return true
	}
	msg := err.Error()
	if m := statusRe.FindStringSubmatch(msg); m != nil {
		code, _ := strconv.Atoi(m[1])
		return code >= 500
	}
	return strings.Contains(msg, "error decode response body") || strings.Contains(msg, "error read response body")
}

// Temporary implements scheduler.Notifier.
func (b *Bot) Temporary(err error) bool { return IsTemporary(err) }
