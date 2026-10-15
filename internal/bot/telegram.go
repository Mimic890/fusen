package bot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/Mimic890/fusen/internal/views"
)

// maxRetries is how many times a call is repeated after a 429 response.
const maxRetries = 3

// sleep waits for d or until ctx is done. Replaced in tests.
var sleep = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// withRetry calls fn, waiting retry_after and retrying when Telegram responds with 429.
func withRetry[T any](ctx context.Context, log *slog.Logger, method string, fn func(context.Context) (T, error)) (T, error) {
	for attempt := 0; ; attempt++ {
		v, err := fn(ctx)
		var tooMany *tgbot.TooManyRequestsError
		if err == nil || !errors.As(err, &tooMany) || attempt >= maxRetries {
			return v, err
		}
		wait := time.Duration(max(tooMany.RetryAfter, 1)) * time.Second
		log.Warn("rate limited", "method", method, "retry_after", tooMany.RetryAfter, "attempt", attempt+1)
		if err := sleep(ctx, wait); err != nil {
			return v, err
		}
	}
}

func isNotModified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "message is not modified")
}

var noPreview = &models.LinkPreviewOptions{IsDisabled: tgbot.True()}

func (b *Bot) sendScreen(ctx context.Context, chatID int64, s views.Screen) (int, error) {
	msg, err := withRetry(ctx, b.log, "sendMessage", func(ctx context.Context) (*models.Message, error) {
		return b.api.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:             chatID,
			Text:               s.Text,
			ParseMode:          models.ParseModeHTML,
			LinkPreviewOptions: noPreview,
			ReplyMarkup:        s.Markup(),
		})
	})
	if err != nil {
		return 0, err
	}
	return msg.ID, nil
}

func (b *Bot) editScreen(ctx context.Context, chatID int64, msgID int, s views.Screen) error {
	_, err := withRetry(ctx, b.log, "editMessageText", func(ctx context.Context) (*models.Message, error) {
		return b.api.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:             chatID,
			MessageID:          msgID,
			Text:               s.Text,
			ParseMode:          models.ParseModeHTML,
			LinkPreviewOptions: noPreview,
			ReplyMarkup:        s.Markup(),
		})
	})
	if isNotModified(err) {
		return nil
	}
	return err
}

func (b *Bot) deleteMessage(ctx context.Context, chatID int64, msgID int) error {
	_, err := withRetry(ctx, b.log, "deleteMessage", func(ctx context.Context) (bool, error) {
		return b.api.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: chatID, MessageID: msgID})
	})
	return err
}

func (b *Bot) answerCallback(ctx context.Context, id, text string) {
	_, err := withRetry(ctx, b.log, "answerCallbackQuery", func(ctx context.Context) (bool, error) {
		return b.api.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: id, Text: text})
	})
	if err != nil {
		b.log.Debug("answer callback", "err", err)
	}
}

func (b *Bot) sendText(ctx context.Context, chatID int64, text string) {
	if _, err := b.sendScreen(ctx, chatID, views.Screen{Text: text}); err != nil {
		b.log.Warn("send text", "chat_id", chatID, "err", err)
	}
}
