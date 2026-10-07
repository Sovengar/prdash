package bitbucket

import (
	"context"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

const ForgeName = "bitbucket"

type Adapter struct {
	host string
}

var _ forge.Adapter = (*Adapter)(nil)

func New(host string) *Adapter {
	if host == "" {
		host = "bitbucket.org"
	}
	return &Adapter{host: host}
}

func (a *Adapter) Forge() string { return ForgeName }

func (a *Adapter) Host() string { return a.host }

func (a *Adapter) Auth(context.Context) model.AuthState {
	return model.AuthState{Forge: ForgeName, OK: false, Reason: "not implemented in this version"}
}

func (a *Adapter) List(_ context.Context, q forge.Query) (forge.Page, []model.Warning) {
	return forge.Page{}, a.unsupported(q.Section)
}

func (a *Adapter) ItemState(context.Context, model.RepoRef, int) (model.Item, []model.Warning) {
	return model.Item{}, a.unsupported("")
}

func (a *Adapter) Comments(context.Context, model.RepoRef, int) (forge.CommentPage, []model.Warning) {
	return forge.CommentPage{}, a.unsupported("")
}

func (a *Adapter) Approve(context.Context, model.RepoRef, int) []model.Warning {
	return a.unsupported("")
}

func (a *Adapter) Merge(context.Context, model.RepoRef, int, forge.MergeRequest) []model.Warning {
	return a.unsupported("")
}

func (a *Adapter) Retarget(context.Context, model.RepoRef, int, string) []model.Warning {
	return a.unsupported("")
}

func (a *Adapter) Branches(context.Context, model.RepoRef) ([]string, []model.Warning) {
	return nil, a.unsupported("")
}

func (a *Adapter) unsupported(section model.Section) []model.Warning {
	return []model.Warning{{
		Forge:   ForgeName,
		Section: section,
		Kind:    "unsupported",
		Msg:     "Bitbucket is not supported in this version",
	}}
}
