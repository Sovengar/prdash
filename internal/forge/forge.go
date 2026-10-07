package forge

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
	"prdash/internal/state"
)

type Query struct {
	Section    model.Section
	ReviewKind model.ReviewKind // requested/assigned (review only)
	Cursor     string
}

type Page struct {
	Items []model.Item
	Next  string
	More  bool
}

var Streams = []Query{
	{Section: model.SectionAuthored},
	{Section: model.SectionReview, ReviewKind: model.ReviewRequested},
	{Section: model.SectionReview, ReviewKind: model.ReviewAssigned},
	{Section: model.SectionMentions},
}

const CommentLimit = 5

type CommentPage struct {
	Comments []model.Comment
	Total    int
}

func (p CommentPage) KeepLast() CommentPage {
	p.Comments = p.Comments[max(0, len(p.Comments)-CommentLimit):]
	return p
}

type Adapter interface {
	Forge() string
	Host() string
	Auth(ctx context.Context) model.AuthState
	List(ctx context.Context, q Query) (Page, []model.Warning)
	ItemState(ctx context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning)
	Comments(ctx context.Context, ref model.RepoRef, number int) (CommentPage, []model.Warning)
	Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning
	Merge(ctx context.Context, ref model.RepoRef, number int, req MergeRequest) []model.Warning
	Retarget(ctx context.Context, ref model.RepoRef, number int, branch string) []model.Warning
	Branches(ctx context.Context, ref model.RepoRef) ([]string, []model.Warning)
}

type MergeRequest struct {
	Mode         MergeMode
	HeadSHA      string
	DeleteBranch bool
}

type PageResult struct {
	Query    Query
	Items    []model.Item
	Next     string
	More     bool
	Warnings []model.Warning
	First    bool
}

func Stream(ctx context.Context, a Adapter, emit func(PageResult) bool) {
	var wg sync.WaitGroup
	for _, q := range Streams {
		wg.Add(1)
		go func(q Query) {
			defer wg.Done()
			streamQuery(ctx, a, q, emit)
		}(q)
	}
	wg.Wait()
}

func streamQuery(ctx context.Context, a Adapter, q Query, emit func(PageResult) bool) {
	cursor := q.Cursor
	first := true
	for {
		page, warns := a.List(ctx, Query{Section: q.Section, ReviewKind: q.ReviewKind, Cursor: cursor})
		cont := emit(PageResult{
			Query:    q,
			Items:    page.Items,
			Next:     page.Next,
			More:     page.More,
			Warnings: warns,
			First:    first,
		})
		if !cont || !page.More || page.Next == "" || ctx.Err() != nil || hasKind(warns, "ratelimit") {
			return
		}
		cursor = page.Next
		first = false
	}
}

func Collect(ctx context.Context, a Adapter) inbox.ForgeResult {
	res := inbox.ForgeResult{Forge: a.Forge(), Host: a.Host()}
	if auth := a.Auth(ctx); !auth.OK {
		res.Warnings = append(res.Warnings, model.Warning{
			Forge: a.Forge(),
			Kind:  "auth",
			Msg:   auth.Reason,
		})
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, q := range Streams {
		wg.Add(1)
		go func(q Query) {
			defer wg.Done()
			streamQuery(ctx, a, q, func(p PageResult) bool {
				mu.Lock()
				defer mu.Unlock()
				switch q.Section {
				case model.SectionAuthored:
					res.Authored = append(res.Authored, p.Items...)
				case model.SectionReview:
					res.Review = append(res.Review, p.Items...)
				case model.SectionMentions:
					res.Mentions = append(res.Mentions, p.Items...)
				}
				res.Warnings = append(res.Warnings, StampSection(p.Warnings, q.Section)...)
				return true
			})
		}(q)
	}
	wg.Wait()
	return res
}

func StampSection(warns []model.Warning, section model.Section) []model.Warning {
	for i := range warns {
		if warns[i].Section == "" {
			warns[i].Section = section
		}
	}
	return warns
}

func hasKind(warns []model.Warning, kind string) bool {
	for _, w := range warns {
		if w.Kind == kind {
			return true
		}
	}
	return false
}

type ActionKind string

const (
	ActionApprove  ActionKind = "approve"
	ActionMerge    ActionKind = "merge"
	ActionRetarget ActionKind = "retarget"
)

type MergeMode string

const (
	MergeCommit MergeMode = "merge"
	Rebase      MergeMode = "rebase"
	Squash      MergeMode = "squash"
)

func (m MergeMode) Label() string {
	switch m {
	case MergeCommit:
		return "merge commit"
	case Rebase:
		return "rebase"
	case Squash:
		return "squash"
	default:
		return string(m)
	}
}

func (m MergeMode) Valid() bool {
	switch m {
	case MergeCommit, Rebase, Squash:
		return true
	default:
		return false
	}
}

func ErrUnknownMergeMode(mode MergeMode) error {
	return fmt.Errorf("unknown merge mode %q: expected merge, rebase or squash", string(mode))
}

var ErrMissingHeadSHA = errors.New(
	"the forge did not report the head commit, so the merge cannot be pinned to what was reviewed")

var ErrMissingBaseBranch = errors.New("the new target branch is empty: there is nothing to retarget to")

func AllowedModes(rules model.MergeRules) []MergeMode {
	if !rules.Known {
		return []MergeMode{Rebase, MergeCommit, Squash}
	}
	var out []MergeMode
	if rules.Rebase {
		out = append(out, Rebase)
	}
	if rules.MergeCommit {
		out = append(out, MergeCommit)
	}
	if rules.Squash {
		out = append(out, Squash)
	}
	return out
}

func AllowsMode(rules model.MergeRules, mode MergeMode) bool {
	return slices.Contains(AllowedModes(rules), mode)
}

type Outcome struct {
	Kind        ActionKind
	ID          model.ID
	Mode        MergeMode // strategy used; only meaningful for ActionMerge
	OK          bool
	Conflict    bool // the item changed (closed/merged/gone): it needs a refresh
	Perm        bool // action disabled by permissions or unsupported
	Unmergeable bool
	Msg         string
	Item        model.Item
	HasItem     bool

	DeleteBranch bool
	DeleteMsg    string

	Base     string
	FromBase string
}

func runOn(seed Outcome, ctx context.Context, a Adapter, kind ActionKind, ref model.RepoRef, number int, exec func(cur model.Item) []model.Warning) (Outcome, []model.Warning) {
	out := seed
	out.Kind, out.ID = kind, model.With(ref.Forge, ref.Host, ref.Project, number)

	cur, warns := a.ItemState(ctx, ref, number)
	if stage := checkBeforeAction(cur, warns); stage != nil {
		stage.Kind, stage.ID = kind, out.ID
		return *stage, nil
	}
	out.Item, out.HasItem = cur, true

	actionWarns := exec(cur)
	out.OK, out.Conflict, out.Perm, out.Msg = classifyAction(actionWarns)
	out.Unmergeable = hasKind(actionWarns, "unmergeable")

	if it, w := a.ItemState(ctx, ref, number); len(w) == 0 && it.Number != 0 {
		out.Item, out.HasItem = it, true
	}
	return out, actionWarns
}

func RunAction(ctx context.Context, a Adapter, kind ActionKind, ref model.RepoRef, number int, req MergeRequest) Outcome {
	if kind != ActionApprove && kind != ActionMerge {
		return Outcome{
			Kind:         kind,
			ID:           model.With(ref.Forge, ref.Host, ref.Project, number),
			Mode:         req.Mode,
			DeleteBranch: req.DeleteBranch,
			Msg:          "prdash does not implement the " + string(kind) + " action",
		}
	}

	out, actionWarns := runOn(
		Outcome{Mode: req.Mode, DeleteBranch: req.DeleteBranch},
		ctx, a, kind, ref, number,
		func(cur model.Item) []model.Warning {
			// No `default` for an unknown Kind: RunAction cuts before here and two places drift.
			if kind == ActionApprove {
				return a.Approve(ctx, ref, number)
			}
			req.HeadSHA = cur.HeadSHA
			return a.Merge(ctx, ref, number, req)
		},
	)

	if kind == ActionMerge && req.DeleteBranch && out.HasItem {
		if !out.OK && state.Derive(out.Item) == state.StateMerged {
			out.OK, out.Conflict, out.Perm, out.Msg = true, false, false, ""
			out.DeleteMsg = firstMsg(actionWarns)
		}
		if out.OK && out.Item.IsFork {
			out.DeleteMsg = "the branch lives in a fork"
		}
	}
	return out
}

func RunRetarget(ctx context.Context, a Adapter, ref model.RepoRef, number int, branch string) Outcome {
	if strings.TrimSpace(branch) == "" {
		return Outcome{
			Kind: ActionRetarget,
			ID:   model.With(ref.Forge, ref.Host, ref.Project, number),
			Perm: true,
			Msg:  ErrMissingBaseBranch.Error(),
			Item: model.Item{},
		}
	}
	out, _ := runOn(
		Outcome{Base: branch},
		ctx, a, ActionRetarget, ref, number,
		func(model.Item) []model.Warning { return a.Retarget(ctx, ref, number, branch) },
	)
	return out
}

func checkBeforeAction(cur model.Item, warns []model.Warning) *Outcome {
	if hasKind(warns, "notfound") {
		return &Outcome{Conflict: true, Msg: "the item no longer exists in the forge"}
	}
	if hasKind(warns, "permission") || hasKind(warns, "auth") {
		return &Outcome{Perm: true, Msg: firstMsg(warns)}
	}
	if len(warns) > 0 || cur.Number == 0 {
		msg := firstMsg(warns)
		if msg == "" {
			msg = "could not re-read the item"
		}
		out := &Outcome{Conflict: true, Msg: msg}
		if cur.Number != 0 {
			out.Item, out.HasItem = cur, true
		}
		return out
	}
	if ok, reason := state.Actionable(cur); !ok {
		return &Outcome{Conflict: true, Msg: reason, Item: cur, HasItem: true}
	}
	return nil
}

func classifyAction(warns []model.Warning) (ok, conflict, perm bool, msg string) {
	if len(warns) == 0 {
		return true, false, false, ""
	}
	msg = firstMsg(warns)
	switch {
	case hasKind(warns, "permission"), hasKind(warns, "auth"), hasKind(warns, "unsupported"):
		return false, false, true, msg
	case hasKind(warns, "selfreview"):
		return false, false, true, state.SelfReviewReason
	case hasKind(warns, "unmergeable"):
		return false, false, false, state.UnmergeableReason
	case hasKind(warns, "notfound"), hasKind(warns, "conflict"),
		hasKind(warns, "ratelimit"), hasKind(warns, "network"), hasKind(warns, "timeout"):
		return false, true, false, msg
	default:
		return false, false, false, msg
	}
}

func firstMsg(warns []model.Warning) string {
	if len(warns) == 0 {
		return ""
	}
	return warns[0].Msg
}

func EscapeGraphQL(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	).Replace(s)
}
