package bot

import (
	"context"
	"time"

	cb "github.com/Mimic890/fusen/internal/callback"
	"github.com/Mimic890/fusen/internal/domain"
	"github.com/Mimic890/fusen/internal/views"
)

func (b *Bot) homeScreen(ctx context.Context, r *request) (views.Screen, error) {
	projects, err := b.store.ListProjects(ctx, r.user.TgID, false)
	if err != nil {
		return views.Screen{}, err
	}
	due, err := b.store.DueSummary(ctx, r.user.TgID, time.Now(), r.user.Location())
	if err != nil {
		return views.Screen{}, err
	}
	archived, err := b.store.CountArchived(ctx, r.user.TgID)
	if err != nil {
		return views.Screen{}, err
	}
	// Quick input from the home screen goes to the inbox.
	if err := b.store.SetCurrentProject(ctx, r.user.TgID, 0); err != nil {
		return views.Screen{}, err
	}
	return views.Home(r.l, projects, due, archived), nil
}

func (b *Bot) showHome(ctx context.Context, r *request, notice string) error {
	s, err := b.homeScreen(ctx, r)
	if err != nil {
		return err
	}
	return b.show(ctx, r, s.WithNotice(notice))
}

func (b *Bot) showProjectManage(ctx context.Context, r *request, id int64, notice string) error {
	p, err := b.store.GetProject(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	cols, err := b.store.ListColumns(ctx, p.ID)
	if err != nil {
		return err
	}
	return b.show(ctx, r, views.Project(r.l, p, cols).WithNotice(notice))
}

func (b *Bot) onNoop(context.Context, *request) error { return nil }

func (b *Bot) onHome(ctx context.Context, r *request) error {
	return b.showHome(ctx, r, "")
}

func (b *Bot) onArchive(ctx context.Context, r *request) error {
	projects, err := b.store.ListProjects(ctx, r.user.TgID, true)
	if err != nil {
		return err
	}
	return b.show(ctx, r, views.Archive(r.l, projects))
}

func (b *Bot) onProject(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	p, err := b.store.GetProject(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	return b.showBoardOf(ctx, r, p, 0, 0, "")
}

func (b *Bot) onProjectManage(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	return b.showProjectManage(ctx, r, id, "")
}

func (b *Bot) projectNewPrompt(r *request) views.Screen {
	return views.Prompt(r.l, r.l.T("project.prompt_new"), cb.New(cb.KindHome))
}

func (b *Bot) onProjectNew(ctx context.Context, r *request) error {
	if err := b.setState(ctx, r, cb.New(cb.KindProjectNew)); err != nil {
		return err
	}
	return b.show(ctx, r, b.projectNewPrompt(r))
}

func (b *Bot) projectRenamePrompt(r *request, p domain.Project) views.Screen {
	return views.Prompt(r.l, r.l.T("project.prompt_rename", "name", views.Escape(p.Name)), cb.New(cb.KindProjectManage, p.ID))
}

func (b *Bot) onProjectRename(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	p, err := b.store.GetProject(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	if err := b.setState(ctx, r, cb.New(cb.KindProjectRename, p.ID)); err != nil {
		return err
	}
	return b.show(ctx, r, b.projectRenamePrompt(r, p))
}

func (b *Bot) onProjectArchive(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	archive := r.data.Arg(1) == "1"
	if err := b.store.SetProjectArchived(ctx, r.user.TgID, id, archive); err != nil {
		return err
	}
	if archive {
		r.toast = r.l.T("project.archived")
		return b.showHome(ctx, r, "")
	}
	r.toast = r.l.T("project.restored")
	return b.showProjectManage(ctx, r, id, "")
}

func (b *Bot) onProjectDelete(ctx context.Context, r *request) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	if r.data.Arg(1) == cb.Confirm {
		if err := b.store.DeleteProject(ctx, r.user.TgID, id); err != nil {
			return err
		}
		r.toast = r.l.T("project.deleted")
		return b.showHome(ctx, r, "")
	}

	p, err := b.store.GetProject(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	if p.IsInbox {
		return domain.ErrInboxProtected
	}
	return b.show(ctx, r, views.ConfirmDeleteProject(r.l, p))
}

func (b *Bot) inputProjectNew(ctx context.Context, r *request, text string) error {
	name, err := domain.NormalizeName(text)
	if isInputError(err) {
		return b.reprompt(ctx, r, b.projectNewPrompt(r), err)
	}
	p, err := b.store.CreateProject(ctx, r.user.TgID, name)
	if err != nil {
		return err
	}
	if err := b.clearState(ctx, r); err != nil {
		return err
	}
	return b.showBoardOf(ctx, r, p, 0, 0, r.l.T("project.created"))
}

func (b *Bot) inputProjectRename(ctx context.Context, r *request, text string) error {
	id, err := r.data.ID(0)
	if err != nil {
		return err
	}
	p, err := b.store.GetProject(ctx, r.user.TgID, id)
	if err != nil {
		return err
	}
	name, err := domain.NormalizeName(text)
	if isInputError(err) {
		return b.reprompt(ctx, r, b.projectRenamePrompt(r, p), err)
	}
	if err := b.store.RenameProject(ctx, r.user.TgID, p.ID, name); err != nil {
		return err
	}
	if err := b.clearState(ctx, r); err != nil {
		return err
	}
	return b.showProjectManage(ctx, r, p.ID, r.l.T("project.renamed"))
}
