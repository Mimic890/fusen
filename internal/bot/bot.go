// Package bot handles Telegram updates and drives the live screen.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/config"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/i18n"
	"github.com/Mimic890/fusen/internal/store"
	"github.com/Mimic890/fusen/internal/views"
)

// updateTimeout bounds the handling of a single update, including 429 retries.
const updateTimeout = 2 * time.Minute

// Bot is the Telegram front end.
type Bot struct {
	api   *tgbot.Bot
	store *store.Store
	i18n  *i18n.Bundle
	cfg   config.Config
	log   *slog.Logger

	locks sync.Map // user ID -> *sync.Mutex

	routes map[string]handler      // callback kind -> handler
	inputs map[string]inputHandler // state kind -> text input handler
}

// request carries per-update context for handlers.
type request struct {
	user domain.User
	l    i18n.Localizer
	data cb.Data // callback payload or decoded pending state
	// toast is shown as the callback query answer.
	toast string
}

type (
	handler      func(ctx context.Context, r *request) error
	inputHandler func(ctx context.Context, r *request, text string) error
)

// New creates the bot and verifies the token with getMe.
func New(cfg config.Config, st *store.Store, tr *i18n.Bundle, log *slog.Logger) (*Bot, error) {
	return newBot(cfg, st, tr, log)
}

func newBot(cfg config.Config, st *store.Store, tr *i18n.Bundle, log *slog.Logger, opts ...tgbot.Option) (*Bot, error) {
	b := &Bot{store: st, i18n: tr, cfg: cfg, log: log}
	b.routes = map[string]handler{
		cb.KindNoop:           b.onNoop,
		cb.KindHome:           b.onHome,
		cb.KindArchive:        b.onArchive,
		cb.KindProject:        b.onProject,
		cb.KindProjectManage:  b.onProjectManage,
		cb.KindProjectNew:     b.onProjectNew,
		cb.KindProjectRename:  b.onProjectRename,
		cb.KindProjectArchive: b.onProjectArchive,
		cb.KindProjectDelete:  b.onProjectDelete,
		cb.KindBoard:          b.onBoard,
		cb.KindView:           b.onView,
		cb.KindColumns:        b.onColumns,
		cb.KindColumn:         b.onColumn,
		cb.KindColumnNew:      b.onColumnNew,
		cb.KindColumnRename:   b.onColumnRename,
		cb.KindColumnMove:     b.onColumnMove,
		cb.KindColumnDone:     b.onColumnDone,
		cb.KindColumnDelete:   b.onColumnDelete,
		cb.KindTaskNew:        b.onTaskNew,
		cb.KindTask:           b.onTask,
		cb.KindTaskMove:       b.onTaskMove,
		cb.KindTaskEdit:       b.onTaskEdit,
		cb.KindTaskTitle:      b.onTaskTitle,
		cb.KindTaskDesc:       b.onTaskDesc,
		cb.KindTaskTags:       b.onTaskTags,
		cb.KindTaskPriority:   b.onTaskPriority,
		cb.KindTaskDelete:     b.onTaskDelete,
		cb.KindSettings:       b.onSettings,
		cb.KindSetLang:        b.onSetLang,
		cb.KindSetView:        b.onSetView,
		cb.KindTimezone:       b.onTimezone,
		cb.KindDigest:         b.onDigest,
		cb.KindRemind:         b.onRemind,
		cb.KindChecklist:      b.onChecklist,
		cb.KindChecklistAdd:   b.onChecklistAdd,
		cb.KindItemToggle:     b.onItemToggle,
		cb.KindItemDelete:     b.onItemDelete,
	}
	b.inputs = map[string]inputHandler{
		cb.KindProjectNew:    b.inputProjectNew,
		cb.KindProjectRename: b.inputProjectRename,
		cb.KindColumnNew:     b.inputColumnNew,
		cb.KindColumnRename:  b.inputColumnRename,
		cb.KindTaskNew:       b.inputTaskNew,
		cb.KindTaskTitle:     b.inputTaskTitle,
		cb.KindTaskDesc:      b.inputTaskDesc,
		cb.KindTaskTags:      b.inputTaskTags,
		cb.KindChecklistAdd:  b.inputChecklistAdd,
		cb.KindTimezone:      b.inputTimezone,
		cb.KindDigest:        b.inputDigest,
	}

	api, err := tgbot.New(cfg.BotToken, append([]tgbot.Option{
		tgbot.WithDefaultHandler(b.handleUpdate),
		tgbot.WithAllowedUpdates(tgbot.AllowedUpdates{"message", "callback_query"}),
		tgbot.WithErrorsHandler(func(err error) { log.Error("telegram", "err", err) }),
	}, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("init telegram client: %w", err)
	}
	b.api = api
	return b, nil
}

// Run polls for updates until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	b.setCommands(ctx)
	b.api.Start(ctx)
}

func (b *Bot) handleUpdate(ctx context.Context, _ *tgbot.Bot, upd *models.Update) {
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			b.log.Error("panic in update handler", "update_id", upd.ID, "panic", p, "stack", string(debug.Stack()))
		}
	}()

	switch {
	case upd.Message != nil:
		b.onMessage(ctx, upd.Message)
	case upd.CallbackQuery != nil:
		b.onCallback(ctx, upd.CallbackQuery)
	}
}

// lock serializes handling of updates from one user.
func (b *Bot) lock(userID int64) func() {
	v, _ := b.locks.LoadOrStore(userID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (b *Bot) newRequest(ctx context.Context, from *models.User) (*request, error) {
	lang := domain.LangFromCode(from.LanguageCode)
	u, created, err := b.store.EnsureUser(ctx, store.NewUser{
		TgID:      from.ID,
		Lang:      lang,
		Timezone:  b.cfg.DefaultTZ,
		InboxName: b.i18n.For(lang).T("project.inbox_name"),
		Columns:   defaultColumns(b.i18n.For(lang)),
	})
	if err != nil {
		return nil, err
	}
	if created {
		b.log.Info("new user", "user_id", u.TgID, "lang", u.Lang)
	}
	return &request{user: u, l: b.i18n.For(u.Lang)}, nil
}

func (b *Bot) onMessage(ctx context.Context, m *models.Message) {
	if m.Chat.Type != models.ChatTypePrivate || m.From == nil {
		return
	}
	if !b.cfg.IsAllowed(m.From.ID) {
		b.log.Info("access denied", "user_id", m.From.ID)
		b.sendText(ctx, m.Chat.ID, b.i18n.For(domain.LangFromCode(m.From.LanguageCode)).T("access_denied"))
		return
	}

	unlock := b.lock(m.From.ID)
	defer unlock()

	r, err := b.newRequest(ctx, m.From)
	if err != nil {
		b.log.Error("load user", "user_id", m.From.ID, "err", err)
		return
	}

	switch {
	case command(m.Text) == "start":
		b.cmdStart(ctx, r, m)
	case command(m.Text) == "settings":
		b.cmdSettings(ctx, r, m)
	case r.user.State != "":
		b.onInput(ctx, r, m)
	}
}

// command returns the bot command name of text ("/start@fusenboard_bot x" -> "start").
func command(text string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	if !strings.HasPrefix(first, "/") {
		return ""
	}
	name, _, _ := strings.Cut(first[1:], "@")
	return strings.ToLower(name)
}

func (b *Bot) cmdStart(ctx context.Context, r *request, m *models.Message) {
	b.dropMessage(ctx, r, m.ID)
	if err := b.clearState(ctx, r); err != nil {
		b.log.Error("clear state", "user_id", r.user.TgID, "err", err)
	}
	if err := b.reopen(ctx, r); err != nil {
		b.log.Error("start", "user_id", r.user.TgID, "err", err)
	}
}

func (b *Bot) onInput(ctx context.Context, r *request, m *models.Message) {
	// Keep the chat clean: user input is removed once handled.
	defer b.dropMessage(ctx, r, m.ID)

	state, err := cb.Decode(r.user.State)
	h, ok := b.inputs[state.Kind]
	if err != nil || !ok {
		b.log.Warn("unknown state", "user_id", r.user.TgID, "state", r.user.State)
		_ = b.clearState(ctx, r)
		return
	}
	r.data = state
	if err := h(ctx, r, m.Text); err != nil {
		_ = b.clearState(ctx, r)
		if err := b.showHome(ctx, r, b.userError(r, err)); err != nil {
			b.log.Error("show home", "user_id", r.user.TgID, "err", err)
		}
	}
}

func (b *Bot) onCallback(ctx context.Context, q *models.CallbackQuery) {
	var toast string
	defer func() { b.answerCallback(ctx, q.ID, toast) }()

	var msgID int
	switch {
	case q.Message.Message != nil:
		if q.Message.Message.Chat.Type != models.ChatTypePrivate {
			return
		}
		msgID = q.Message.Message.ID
	case q.Message.InaccessibleMessage != nil:
		msgID = q.Message.InaccessibleMessage.MessageID
	default:
		return
	}

	if !b.cfg.IsAllowed(q.From.ID) {
		toast = b.i18n.For(domain.LangFromCode(q.From.LanguageCode)).T("access_denied")
		return
	}

	unlock := b.lock(q.From.ID)
	defer unlock()

	r, err := b.newRequest(ctx, &q.From)
	if err != nil {
		b.log.Error("load user", "user_id", q.From.ID, "err", err)
		return
	}
	defer func() { toast = r.toast }()

	data, err := cb.Decode(q.Data)
	if err != nil {
		b.log.Warn("bad callback data", "user_id", r.user.TgID, "data", q.Data)
		return
	}
	r.data = data

	if data.Kind == cb.KindReopen {
		if err := b.reopen(ctx, r); err != nil {
			b.log.Error("reopen", "user_id", r.user.TgID, "err", err)
		}
		return
	}

	if msgID != r.user.LiveMessageID {
		r.toast = r.l.T("stale.toast")
		b.markStale(ctx, r, msgID)
		return
	}

	// Any navigation cancels pending input; handlers may set a new state.
	if err := b.clearState(ctx, r); err != nil {
		b.log.Error("clear state", "user_id", r.user.TgID, "err", err)
	}

	h, ok := b.routes[data.Kind]
	if !ok {
		b.log.Warn("unknown callback", "user_id", r.user.TgID, "data", q.Data)
		return
	}
	if err := h(ctx, r); err != nil {
		r.toast = b.userError(r, err)
	}
}

// userError maps an error to a message for the user, logging unexpected ones.
func (b *Bot) userError(r *request, err error) string {
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, cb.ErrNoArg), errors.Is(err, cb.ErrBadIDArg):
		return r.l.T("error.not_found")
	case errors.Is(err, domain.ErrEmptyName):
		return r.l.T("error.empty_name")
	case errors.Is(err, domain.ErrNameTooLong):
		return r.l.T("error.name_too_long", "max", domain.MaxNameLen)
	case errors.Is(err, domain.ErrTitleTooLong):
		return r.l.T("error.title_too_long", "max", domain.MaxTitleLen)
	case errors.Is(err, domain.ErrDescriptionTooLong):
		return r.l.T("error.description_too_long", "max", domain.MaxDescriptionLen)
	case errors.Is(err, domain.ErrItemTooLong):
		return r.l.T("error.item_too_long", "max", domain.MaxItemLen)
	case errors.Is(err, domain.ErrTooManyItems):
		return r.l.T("error.too_many_items", "max", domain.MaxChecklistItems)
	case errors.Is(err, domain.ErrTooManyTags):
		return r.l.T("error.too_many_tags", "max", domain.MaxTags)
	case errors.Is(err, domain.ErrTagTooLong):
		return r.l.T("error.tag_too_long", "max", domain.MaxTagLen)
	case errors.Is(err, domain.ErrInboxProtected):
		return r.l.T("error.inbox_protected")
	case errors.Is(err, domain.ErrColumnNotEmpty):
		return r.l.T("error.column_not_empty")
	case errors.Is(err, domain.ErrLastColumn):
		return r.l.T("error.last_column")
	case errors.Is(err, domain.ErrBadTimezone):
		return r.l.T("error.bad_timezone")
	case errors.Is(err, domain.ErrBadClock):
		return r.l.T("error.bad_clock")
	case errors.Is(err, domain.ErrTooManyColumns):
		return r.l.T("error.too_many_columns", "max", domain.MaxColumns)
	}
	b.log.Error("handler failed", "user_id", r.user.TgID, "kind", r.data.Kind, "args", r.data.Args, "err", err)
	return r.l.T("error.generic")
}

// isInputError reports whether err is a validation error the user can fix by
// sending another message.
func isInputError(err error) bool {
	for _, e := range []error{
		domain.ErrEmptyName, domain.ErrNameTooLong, domain.ErrTitleTooLong, domain.ErrDescriptionTooLong,
		domain.ErrItemTooLong, domain.ErrTooManyItems, domain.ErrTooManyTags, domain.ErrTagTooLong,
		domain.ErrBadTimezone, domain.ErrBadClock,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// reprompt shows the prompt again with the validation error, keeping the pending input.
func (b *Bot) reprompt(ctx context.Context, r *request, prompt views.Screen, err error) error {
	return b.show(ctx, r, prompt.WithNotice(b.userError(r, err)))
}

func (b *Bot) setState(ctx context.Context, r *request, state cb.Data) error {
	s := state.String()
	if err := b.store.SetState(ctx, r.user.TgID, s); err != nil {
		return err
	}
	r.user.State = s
	return nil
}

func (b *Bot) clearState(ctx context.Context, r *request) error {
	if r.user.State == "" {
		return nil
	}
	if err := b.store.SetState(ctx, r.user.TgID, ""); err != nil {
		return err
	}
	r.user.State = ""
	return nil
}

func (b *Bot) dropMessage(ctx context.Context, r *request, msgID int) {
	if err := b.deleteMessage(ctx, r.user.TgID, msgID); err != nil {
		b.log.Debug("delete user message", "user_id", r.user.TgID, "err", err)
	}
}

// show renders s on the live screen, opening a new one if editing fails.
func (b *Bot) show(ctx context.Context, r *request, s views.Screen) error {
	if r.user.LiveMessageID != 0 {
		err := b.editScreen(ctx, r.user.TgID, r.user.LiveMessageID, s)
		if err == nil {
			return nil
		}
		b.log.Debug("edit live screen", "user_id", r.user.TgID, "err", err)
	}
	return b.openLive(ctx, r, s)
}

// reopen sends a fresh live screen with the home view.
func (b *Bot) reopen(ctx context.Context, r *request) error {
	s, err := b.homeScreen(ctx, r)
	if err != nil {
		return err
	}
	return b.openLive(ctx, r, s)
}

// openLive sends s as a new live screen and retires the previous one.
func (b *Bot) openLive(ctx context.Context, r *request, s views.Screen) error {
	id, err := b.sendScreen(ctx, r.user.TgID, s)
	if err != nil {
		return fmt.Errorf("send live screen: %w", err)
	}
	old := r.user.LiveMessageID
	if err := b.store.SetLiveMessage(ctx, r.user.TgID, id); err != nil {
		return err
	}
	r.user.LiveMessageID = id
	if old != 0 && old != id {
		if err := b.deleteMessage(ctx, r.user.TgID, old); err != nil {
			// Messages older than 48 hours cannot be deleted.
			b.markStale(ctx, r, old)
		}
	}
	return nil
}

func (b *Bot) markStale(ctx context.Context, r *request, msgID int) {
	if err := b.editScreen(ctx, r.user.TgID, msgID, views.Stale(r.l)); err != nil {
		b.log.Debug("mark stale", "user_id", r.user.TgID, "msg_id", msgID, "err", err)
	}
}
