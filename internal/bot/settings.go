package bot

import (
	"context"
	"slices"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/views"
)

func (b *Bot) showSettings(ctx context.Context, r *request, notice string) error {
	return b.show(ctx, r, views.Settings(r.l, r.user, time.Now()).WithNotice(notice))
}

func (b *Bot) onSettings(ctx context.Context, r *request) error {
	return b.showSettings(ctx, r, "")
}

func (b *Bot) cmdSettings(ctx context.Context, r *request, m *models.Message) {
	b.dropMessage(ctx, r, m.ID)
	if err := b.clearState(ctx, r); err != nil {
		b.log.Error("clear state", "user_id", r.user.TgID, "err", err)
	}
	if err := b.showSettings(ctx, r, ""); err != nil {
		b.log.Error("settings", "user_id", r.user.TgID, "err", err)
	}
}

func (b *Bot) onSetLang(ctx context.Context, r *request) error {
	lang := r.data.Arg(0)
	if !domain.IsLang(lang) {
		return domain.ErrNotFound
	}
	if lang != r.user.Lang {
		oldInbox := b.i18n.For(r.user.Lang).T("project.inbox_name")
		newInbox := b.i18n.For(lang).T("project.inbox_name")
		if err := b.store.SetLang(ctx, r.user.TgID, lang, oldInbox, newInbox); err != nil {
			return err
		}
		r.user.Lang = lang
		r.l = b.i18n.For(lang)
	}
	r.toast = r.l.T("settings.saved")
	return b.showSettings(ctx, r, "")
}

func (b *Bot) onSetView(ctx context.Context, r *request) error {
	v, ok := domain.ParseViewCode(r.data.Arg(0))
	if !ok {
		return domain.ErrNotFound
	}
	if err := b.store.SetViewMode(ctx, r.user.TgID, v); err != nil {
		return err
	}
	r.user.ViewMode = v
	r.toast = r.l.T("settings.saved")
	return b.showSettings(ctx, r, "")
}

// The timezone and digest pickers also accept typed input while shown.

func (b *Bot) onTimezone(ctx context.Context, r *request) error {
	if arg := r.data.Arg(0); arg != "" {
		r.toast = r.l.T("settings.saved")
		return b.saveTimezone(ctx, r, arg)
	}
	if err := b.setState(ctx, r, cb.New(cb.KindTimezone)); err != nil {
		return err
	}
	return b.show(ctx, r, views.TimezonePicker(r.l, r.user, time.Now()))
}

func (b *Bot) saveTimezone(ctx context.Context, r *request, input string) error {
	tz, err := domain.ParseTimezone(input)
	if err != nil {
		return err
	}
	if err := b.store.SetTimezone(ctx, r.user.TgID, tz); err != nil {
		return err
	}
	r.user.Timezone = tz
	if err := b.clearState(ctx, r); err != nil {
		return err
	}
	return b.showSettings(ctx, r, "")
}

func (b *Bot) inputTimezone(ctx context.Context, r *request, text string) error {
	err := b.saveTimezone(ctx, r, text)
	if isInputError(err) {
		return b.reprompt(ctx, r, views.TimezonePicker(r.l, r.user, time.Now()), err)
	}
	return err
}

func (b *Bot) onDigest(ctx context.Context, r *request) error {
	switch arg := r.data.Arg(0); arg {
	case "":
		if err := b.setState(ctx, r, cb.New(cb.KindDigest)); err != nil {
			return err
		}
		return b.show(ctx, r, views.DigestPicker(r.l, r.user))
	case cb.Off:
		r.toast = r.l.T("settings.saved")
		return b.saveDigest(ctx, r, "")
	default:
		clock, err := domain.ParseClockCode(arg)
		if err != nil {
			return domain.ErrNotFound
		}
		r.toast = r.l.T("settings.saved")
		return b.saveDigest(ctx, r, clock)
	}
}

func (b *Bot) saveDigest(ctx context.Context, r *request, clock string) error {
	if err := b.store.SetDigestTime(ctx, r.user.TgID, clock); err != nil {
		return err
	}
	r.user.DigestTime = clock
	if err := b.clearState(ctx, r); err != nil {
		return err
	}
	return b.showSettings(ctx, r, "")
}

func (b *Bot) inputDigest(ctx context.Context, r *request, text string) error {
	clock, err := domain.ParseClock(text)
	if err != nil {
		return b.reprompt(ctx, r, views.DigestPicker(r.l, r.user), err)
	}
	return b.saveDigest(ctx, r, clock)
}

func (b *Bot) onRemind(ctx context.Context, r *request) error {
	if r.data.Arg(0) == "" {
		return b.show(ctx, r, views.RemindPicker(r.l, r.user))
	}
	m := r.data.Int(0)
	if !slices.Contains(domain.RemindPresets, m) {
		return domain.ErrNotFound
	}
	if err := b.store.SetRemindBefore(ctx, r.user.TgID, m); err != nil {
		return err
	}
	r.user.RemindBeforeMin = m
	r.toast = r.l.T("settings.saved")
	return b.showSettings(ctx, r, "")
}

// setCommands registers the command menu in every supported language.
func (b *Bot) setCommands(ctx context.Context) {
	for _, lang := range domain.Langs {
		l := b.i18n.For(lang)
		params := &tgbot.SetMyCommandsParams{
			Commands: []models.BotCommand{
				{Command: "start", Description: l.T("cmd.start")},
				{Command: "settings", Description: l.T("cmd.settings")},
			},
		}
		// English is also the default for all other languages.
		if lang != domain.LangEN {
			params.LanguageCode = lang
		}
		if _, err := withRetry(ctx, b.log, "setMyCommands", func(ctx context.Context) (bool, error) {
			return b.api.SetMyCommands(ctx, params)
		}); err != nil {
			b.log.Warn("set commands", "lang", lang, "err", err)
		}
	}
}
