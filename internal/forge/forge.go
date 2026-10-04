// Package forge defines the contract every forge implements. The methods return items plus typed
// warnings and never a hard error: a down or unauthenticated forge must not empty the inbox.
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
	ReviewKind model.ReviewKind // requested/assigned (solo review)
	Cursor     string
}

type Page struct {
	Items []model.Item
	Next  string // cursor de la página siguiente
	More  bool   // quedan páginas
}

var Streams = []Query{
	{Section: model.SectionAuthored},
	{Section: model.SectionReview, ReviewKind: model.ReviewRequested},
	{Section: model.SectionReview, ReviewKind: model.ReviewAssigned},
	{Section: model.SectionMentions},
}

// Exactly what the detail pane paints, so we never pay for rows nobody sees.
const CommentLimit = 5

// Total is separate from the items because they do not have to match: the pane shows CommentLimit
// and the total is what lets it say "5 of 23". Total is a lower bound, never an overcount, so a short
// count reads as "there is more" instead of making anyone decide wrongly.
type CommentPage struct {
	Comments []model.Comment
	Total    int
}

// Trims the TAIL, not the head: the request is reversed, so what overflows at the front is exactly
// what the pane was never going to show. Total stays untouched.
func (p CommentPage) KeepLast() CommentPage {
	if len(p.Comments) > CommentLimit {
		p.Comments = p.Comments[len(p.Comments)-CommentLimit:]
	}
	return p
}

type Adapter interface {
	Forge() string
	Host() string
	Auth(ctx context.Context) model.AuthState
	List(ctx context.Context, q Query) (Page, []model.Warning)
	ItemState(ctx context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning)
	// Oldest of the most recent first, capped by CommentLimit rather than a parameter: the pane has a
	// fixed height and a query costs what it returns.
	Comments(ctx context.Context, ref model.RepoRef, number int) (CommentPage, []model.Warning)
	Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning
	Merge(ctx context.Context, ref model.RepoRef, number int, req MergeRequest) []model.Warning
	// Not approve/merge under another name: it changes what the PR integrates against, so from here the
	// diff, the mergeability and the CI are different and the forge no longer remembers the merge. No
	// head SHA pin, because moving the base integrates nothing.
	// An empty branch is an explicit warning, never an argv with an empty flag: the forge would read
	// that as "leave the item with no target branch".
	Retarget(ctx context.Context, ref model.RepoRef, number int, branch string) []model.Warning
	// From the forge and nowhere else: asking the local clone would only return the refs it fetched,
	// which are not the ones the forge can integrate.
	// Free text would let a typo be accepted silently, which is the worst place to find out that the
	// PR points at `main` instead of `main-2`.
	Branches(ctx context.Context, ref model.RepoRef) ([]string, []model.Warning)
}

// A struct rather than loose parameters because the three only make sense together: a pin without a
// mode is not a merge, and deleting the branch without a pin is a blind merge.
type MergeRequest struct {
	// Not optional: `gh pr merge` without a strategy flag falls into an interactive prompt that hangs.
	Mode MergeMode
	// The adapter MUST pin the merge to it: between the inbox refresh and the keypress the branch can
	// have advanced, and without the pin the merge integrates commits nobody reviewed. Empty means "the
	// forge did not report it", which becomes a warning and never an unpinned merge.
	HeadSHA string
	// A POST-integration effect the adapter must understand as such: on GitHub the flag that asks for
	// it also names the branch, so deleting is part of the same command. If the delete fails the merge
	// already happened, so the result is NOT a failed merge (see Outcome.DeleteMsg).
	DeleteBranch bool
}

type PageResult struct {
	Query    Query
	Items    []model.Item
	Next     string
	More     bool
	Warnings []model.Warning
	First    bool // primera página de la lista
}

// Each page goes out through emit, which must be safe for concurrent use and returns false to stop
// that list (e.g. when its header did not change).
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

// Adapters check this before building their argv: an unknown mode has to be an explicit warning, not
// an empty flag, or the CLI hangs on a prompt.
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

// Without the pin the source branch can have advanced between the read and the keypress, so a forge
// that does not report a head SHA is one we cannot merge safely from here.
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
	Mode        MergeMode // estrategia usada; solo tiene sentido en ActionMerge
	OK          bool      // la acción se aplicó
	Conflict    bool      // el ítem cambió (cerrado/mergeado/ausente): hay que refrescar
	Perm        bool      // acción deshabilitada por permisos o por no soportado
	Unmergeable bool
	Msg         string // motivo para la UI
	Item        model.Item
	HasItem     bool // Item trae el estado releído

	DeleteBranch bool
	// Separate from Msg because a failed delete is not a failed merge: the item IS merged and retrying is
	// not the advice. Without the split, "merge failed" on an already-merged item sends the user
	// looking for a forge state that does not exist.
	DeleteMsg string

	// FromBase is filled by whoever held the read, not by the adapter: the forge never looks at the old
	// base, it only writes the new one, and the warning has to be able to say "main → release/2.0".
	Base     string
	FromBase string
}

// The executor gets the RE-READ, not the copy the TUI held: it is the read closest to the action and
// so the one with the smallest window between what was observed and what is applied.
// It also gets the action's raw warnings, because a post-process must not lose the original reason:
// the branch delete runs inside the same command as the merge.
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
	// An action that does not exist is NOT dispatched, and the cut is BEFORE `runOn`: `runOn` re-reads the
	// item, so dispatching an impossible action costs a round trip to GitHub, and the answer is not a
	// forge response so it does not belong to `classifyAction`. Full reasoning in ADR 0008.
	// The Outcome carries no flags on purpose: not a conflict (the item did not change) and not a perm
	// (which records the item as denied forever). It is the caller's bug, not the item's or the session's.
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
			// The `default` that used to catch an unknown Kind is gone: RunAction cuts before reaching
			// here. Keeping it would be a second place where a new Kind dispatches silently, and two places
			// saying the same thing drift.
			if kind == ActionApprove {
				return a.Approve(ctx, ref, number)
			}
			req.HeadSHA = cur.HeadSHA
			return a.Merge(ctx, ref, number, req)
		},
	)

	// The branch delete rides in the SAME command as the merge, so its failure surfaces as the whole
	// command's even though the integration already happened: no push, a protected branch, or a merge
	// queue that rejects -d before merging. runOn's re-read tells the two apart without an extra call:
	// if the item comes back merged, the merge went and the delete did not.
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

// Shares runOn's path with approve/merge — same guards, same classification, same re-read — instead of
// having its own: these are the three actions on an open item and they have exactly the same reasons to
// refuse. The only difference is what the adapter is asked for, and that is what the branch name says.
//
// No head SHA pin, and not by oversight: moving the base integrates nothing, it only changes what the
// comparison is against, so what the pin protects (integrating what nobody reviewed) cannot happen here.
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
	switch {
	case hasKind(warns, "notfound"):
		return &Outcome{Conflict: true, Msg: "the item no longer exists in the forge"}
	case hasKind(warns, "permission"), hasKind(warns, "auth"):
		return &Outcome{Perm: true, Msg: firstMsg(warns)}
	case len(warns) > 0 || cur.Number == 0:
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
		// Not a conflict in the sense of "the item changed": that resolves itself and so warns about a
		// refresh. Here the branches collide and do not fix themselves, so the reason is the canonical one
		// (which says what to do) rather than the CLI's English. Not a permission either: recording the item
		// as denied would cost it the merge for good, and a rebase fixes it.
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
