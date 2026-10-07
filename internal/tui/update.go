package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/review/executor"
	"prdash/internal/state"
	"prdash/internal/worktree"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.sim.viaGraphics {
			m.republishSimImage()
		} else {
			m.renderSimCells()
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case authMsg:
		if msg.cycle != m.cycle {
			return m.withPump(nil)
		}
		if st := m.statuses[msg.forge]; st != nil {
			st.auth = msg.auth
		}
		return m.withPump(nil)

	case pageMsg:
		if msg.cycle != m.cycle {
			return m.withPump(nil)
		}
		m.applyPage(msg)
		return m.withPump(nil)

	case forgeDoneMsg:
		if msg.cycle != m.cycle {
			return m.withPump(nil)
		}
		if st := m.statuses[msg.forge]; st != nil {
			st.loading = false
		}
		return m.withPump(nil)

	case refreshDoneMsg:
		if msg.cycle != m.cycle {
			return m.withPump(nil)
		}
		m.loading = false
		m.lastRefresh = time.Now()
		m.recomputeBackoff()
		m.saveSnapshot()
		return m.withPump(m.armTick())

	case tickMsg:
		m.tickPending = false
		if m.paused() {
			return m, m.armTick()
		}
		updated, cmd := m.beginRefresh()
		return updated, cmd

	case actionMsg:
		cmd := m.applyAction(msg.outcome, msg.cycle)
		return m.withPump(cmd)

	case notifyMsg:
		m.setNotice(msg.text, msg.level)
		return m, nil

	case toastTickMsg:
		m.toast.update()
		return m, tickToast()

	case mountMsg:
		m.mountBusy = false
		m.applyMount(msg.result, msg.err)
		return m.withPump(nil)

	case reviewCleanupMsg:
		m.applyReviewCleanup(msg)
		return m, nil

	case simMsg:
		m.applySim(msg)
		return m.withPump(nil)

	case branchesMsg:
		m.applyBranches(msg)
		return m.withPump(nil)

	case commentsTickMsg:
		return m, m.requestComments()

	case commentsMsg:
		m.applyComments(msg)
		return m.withPump(nil)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) withPump(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.releaseReader()
	return m, tea.Batch(cmd, m.armReader())
}

func (m *Model) releaseReader() {
	if m.readers > 0 {
		m.readers--
	}
}

// The re-read item is the most recent state the forge has, so it always applies.
func (m *Model) applyAction(out forge.Outcome, cycle int) tea.Cmd {
	m.actionBusy = false
	if out.HasItem {
		m.actionCycle[out.Item.ID()] = cycle
		delete(m.comments, out.Item.ID())
		m.applyItemUpdate(out.Item)
	}
	switch {
	case out.Perm:
		m.denied[out.ID] = out.Msg
		m.setNotice(string(out.Kind)+" disabled: "+out.Msg, levelWarn)
	case out.Conflict:
		m.setNotice("forge conflict: "+out.Msg, levelError)
	case out.Unmergeable:
		m.setNotice(string(out.Kind)+" refused: "+out.Msg, levelError)
	case out.OK:
		notice := actionDoneNotice(out)
		level := levelOK
		if out.DeleteMsg != "" {
			notice += " · branch not deleted: " + out.DeleteMsg
			level = levelWarn
		} else if stale := m.staleReviewNotice(out.Item); stale != "" && out.Kind == forge.ActionRetarget {
			notice += " · " + stale
			level = levelWarn
		}
		m.setNotice(notice, level)
		if triggersReviewCleanup(out) {
			return m.reviewCleanupCmd(out.Item, notice, level)
		}
	default:
		m.setNotice("error: "+out.Msg, levelError)
	}
	return nil
}

func triggersReviewCleanup(out forge.Outcome) bool {
	return out.Kind == forge.ActionMerge && out.OK
}

// In the background so the Update handler does not block on a git subprocess.
func (m *Model) reviewCleanupCmd(it model.Item, base string, level noticeLevel) tea.Cmd {
	if m.reviewRemover == nil {
		return nil
	}
	appCtx, remover := m.ctx, m.reviewRemover
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(appCtx, reviewCleanupTimeout)
		defer cancel()
		removed, reason, err := remover.RemoveReview(ctx, it)
		return reviewCleanupMsg{base: base, level: level, removed: removed, reason: reason, err: err}
	}
}

// A no-op re-emits nothing: re-stacking would duplicate the merge notice.
func (m *Model) applyReviewCleanup(msg reviewCleanupMsg) {
	if !msg.removed && msg.reason == "" && msg.err == nil {
		return
	}
	text, level := reviewCleanupNotice(msg.base, msg.level, msg.removed, msg.reason, msg.err)
	m.setNoticeReplacing(msg.base, text, level)
}

// Never suppresses the merge's facts; it prefixes them with the worktree's outcome.
func reviewCleanupNotice(base string, baseLevel noticeLevel, removed bool, reason string, err error) (string, noticeLevel) {
	if err != nil {
		return base + " · could not remove the worktree: " + err.Error(), levelWarn
	}
	if removed {
		level := levelOK
		if baseLevel == levelWarn {
			level = levelWarn
		}
		return base + " · worktree removed", level
	}
	if reason != "" {
		return base + " · " + keptReviewNotice(reason), levelWarn
	}
	return base, baseLevel
}

func keptReviewNotice(reason string) string {
	if reason == worktree.KeptUncommitted {
		return "merged, but the worktree has uncommitted changes — kept"
	}
	return "worktree kept: " + reason
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.mergeArmed {
		return m.handleMergeArmed(msg, key)
	}

	if m.sim.state != simClosed {
		return m.handleSimKey(msg, key)
	}

	if m.retarget.state != retargetClosed {
		return m.handleRetargetKey(msg, key)
	}

	switch key {
	case "q", "ctrl+c":
		m.cancel()
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(m.cursor - 1)
		return m, nil
	case "down", "j":
		m.moveCursor(m.cursor + 1)
		return m, nil
	case "home":
		m.goTop()
		return m, nil
	case "end":
		m.moveCursor(len(m.rows()))
		return m, nil
	case "pgup":
		m.pageBy(-m.pageRows())
		return m, nil
	case "pgdown":
		m.pageBy(m.pageRows())
		return m, nil
	}

	switch m.cfg.ActionForKey(key) {
	case "section-next":
		m.cycleSection()
		return m, nil
	case "prefix-mode":
		m.cyclePrefixMode()
		return m, nil
	case "refresh":
		return m.startRefresh()
	case "approve":
		return m, m.startAction(forge.ActionApprove, forge.MergeRequest{})
	case "merge":
		return m.armMerge()
	case "mount-review":
		return m.startMount()
	case "simulate":
		return m.openSimulator()
	case "retarget":
		return m.openRetarget()
	case "open-browser":
		it, ok := m.selected()
		if !ok || it.URL == "" {
			m.setNotice("no URL to open", levelWarn)
			return m, nil
		}
		return m, m.openBrowserCmd(it.URL)
	case "quit":
		m.cancel()
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) cyclePrefixMode() {
	m.prefixMode = m.prefixMode.next()
	m.syncScroll()
}

// A hard block prevents arming; a soft block still arms and is named by the confirmation.
func (m Model) armMerge() (tea.Model, tea.Cmd) {
	it, _, ok := m.canAction(forge.ActionMerge)
	if !ok {
		return m, nil
	}
	if block := state.MergeBlock(it); block.Hard {
		m.setNotice("merge blocked: "+block.Reason, levelWarn)
		return m, nil
	} else {
		m.mergeBlockReason = block.Reason
	}
	m.mergeArmed = true
	m.mergeArmedID = it.ID()
	return m, nil
}

// A key that is not a mode consumes the press: delegating once let `m` then `a` approve.
func (m Model) handleMergeArmed(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	var mode forge.MergeMode
	switch key {
	case "m":
		mode = forge.MergeCommit
	case "r":
		mode = forge.Rebase
	case "s":
		mode = forge.Squash
	case "tab":
		m.deleteBranch = !m.deleteBranch
		return m, nil
	case "esc":
		m.disarmMerge()
		m.setNotice("merge cancelled", levelInfo)
		return m, nil
	case "q", "ctrl+c":
		m.disarmMerge()
		m.cancel()
		return m, tea.Quit
	default:
		m.disarmMerge()
		return m, nil
	}

	// The merge goes out on the item the user confirmed, or it does not go out.
	it, ok := m.selected()
	if !ok || it.ID() != m.mergeArmedID {
		m.disarmMerge()
		m.setNotice("the selected item changed: press merge again", levelWarn)
		return m, nil
	}
	// Refused here beats emitting a merge the forge rejects with a vaguer message.
	if !forge.AllowsMode(it.Merge, mode) {
		m.disarmMerge()
		m.setNotice("the repository does not allow "+mode.Label()+" merges", levelWarn)
		return m, nil
	}
	m.disarmMerge()
	return m, m.startAction(forge.ActionMerge, forge.MergeRequest{Mode: mode, DeleteBranch: m.deleteBranch})
}

func (m *Model) disarmMerge() {
	m.mergeArmed = false
	m.mergeArmedID = model.ID{}
	m.mergeBlockReason = ""
}

// No channel reader armed: the local refresh consumes no events.
func (m Model) startRefresh() (tea.Model, tea.Cmd) {
	if m.loading {
		m.setNotice("refresh in progress", levelInfo)
		return m, nil
	}
	updated, cmd := m.beginRefresh()
	return updated, cmd
}

// Checked before arming: a confirmation that cannot execute ends in disappointment.
func (m *Model) canAction(kind forge.ActionKind) (model.Item, forge.Adapter, bool) {
	it, ok := m.selected()
	if !ok {
		m.setNotice("select an item first", levelWarn)
		return model.Item{}, nil, false
	}
	a, ok := m.canActionOn(kind, it)
	return it, a, ok
}

func (m *Model) canActionOn(kind forge.ActionKind, it model.Item) (forge.Adapter, bool) {
	a := m.byForge[it.Forge]
	if a == nil {
		m.setNotice("unknown forge: "+it.Forge, levelError)
		return nil, false
	}
	if st := m.statuses[it.Forge]; st != nil && !st.auth.OK {
		m.setNotice("action disabled: "+it.Forge+": "+authReason(st.auth), levelWarn)
		return nil, false
	}
	if reason := m.denied[it.ID()]; reason != "" {
		m.setNotice(string(kind)+" disabled: "+reason, levelWarn)
		return nil, false
	}
	// Cut before spending the CLI call: no forge allows approving your own.
	if reason := m.selfDenied[it.ID()]; reason != "" && kind == forge.ActionApprove {
		m.setNotice(reason, levelWarn)
		return nil, false
	}
	if m.actionBusy {
		m.setNotice("an action is already running", levelWarn)
		return nil, false
	}
	if ok, reason := state.Actionable(it); !ok {
		m.setNotice(reason, levelWarn)
		return nil, false
	}
	return a, true
}

// Named only when the forge did not complain: a fork PR never deletes it.
func actionDoneNotice(out forge.Outcome) string {
	switch out.Kind {
	case forge.ActionMerge:
		notice := string(out.Kind) + " (" + out.Mode.Label() + ") ok"
		if out.DeleteBranch {
			notice += " · branch deleted"
		}
		return notice
	case forge.ActionRetarget:
		if out.FromBase == "" {
			return string(out.Kind) + " (→ " + out.Base + ") ok"
		}
		return string(out.Kind) + " (" + out.FromBase + " → " + out.Base + ") ok"
	default:
		return string(out.Kind) + " ok"
	}
}

func (m *Model) startAction(kind forge.ActionKind, req forge.MergeRequest) tea.Cmd {
	it, a, ok := m.canAction(kind)
	if !ok {
		return nil
	}
	return m.launchAction(kind, it, a, actionProgressNotice(kind, req.Mode),
		func(ctx context.Context) forge.Outcome {
			return forge.RunAction(ctx, a, kind, it.Ref, it.Number, req)
		})
}

// `actionBusy` is set here only, so two functions cannot forget one of them.
func (m *Model) launchAction(kind forge.ActionKind, it model.Item, a forge.Adapter, notice string, exec func(context.Context) forge.Outcome) tea.Cmd {
	m.actionBusy = true
	m.setNotice(notice, levelInfo)

	appCtx, events, cycle := m.ctx, m.events, m.cycle
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, actionTimeout)
		defer cancel()
		sendEvent(appCtx, events, actionMsg{cycle: cycle, outcome: exec(ctx)})
	}()
	return nil
}

func actionProgressNotice(kind forge.ActionKind, mode forge.MergeMode) string {
	if kind == forge.ActionMerge {
		return string(kind) + " (" + mode.Label() + ") in progress…"
	}
	return string(kind) + " in progress…"
}

func (m Model) startMount() (tea.Model, tea.Cmd) {
	it, ok := m.selected()
	if !ok {
		m.setNotice("select an item first", levelWarn)
		return m, nil
	}
	if m.mounter == nil {
		m.setNotice("mounting a review requires Herdr", levelWarn)
		return m, nil
	}
	if m.mountBusy {
		m.setNotice("a review mount is already running", levelWarn)
		return m, nil
	}
	m.mountBusy = true
	m.setNotice("mounting review…", levelInfo)

	appCtx := m.ctx
	events := m.events
	mounter := m.mounter
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, mountTimeout)
		defer cancel()
		res, err := mounter.Mount(ctx, it)
		sendEvent(appCtx, events, mountMsg{result: res, err: err})
	}()
	return m, nil
}

func (m *Model) applyMount(res executor.Result, err error) {
	text, level := mountNotice(res, err)
	m.setNotice(text, level)
}

func mountNotice(res executor.Result, err error) (string, noticeLevel) {
	if err != nil {
		return "could not mount review: " + err.Error(), levelError
	}
	if res.Herdr {
		return fmt.Sprintf("review mounted: %d panes in %d tabs, %s", res.Plan.PaneCount(), len(res.Plan.Tabs), res.Worktree.Path), levelOK
	}
	return "the review layout requires Herdr; the worktree was mounted at " + res.Worktree.Path, levelWarn
}

// All cursor movement goes through here so the selection can never leave the window.
func (m *Model) moveCursor(row int) {
	m.cursor = row
	m.clampCursor()
	m.syncScroll()
}

// Resets the scroll too: the auto-scroll would otherwise hide the prefix line and the header.
func (m *Model) goTop() {
	m.cursor = 0
	m.scroll = 0
}

// Moves the scroll too, or the list barely moves while the detail jumps.
func (m *Model) pageBy(delta int) {
	m.cursor += delta
	m.scroll += delta
	m.clampCursor()
	m.syncScroll()
}

func (m *Model) pageRows() int {
	view := m.layout().bodyLines
	if view <= 0 {
		return 6
	}
	return view
}

func (m Model) View() tea.View {
	var v view
	lay := m.layout()
	it, ok := m.selected()
	v = m.compose(lay, m.listSection(lay), m.detailSection(it, ok, lay.detailLines))
	v.text = overlayToasts(v.text, m.toast.blocks(m.contentWidth()), m.contentWidth(), v.rows)
	// Popups after the toasts, so a warning cannot cover what the user just opened.
	if box, ok := m.simOverlay(); ok {
		v.text = overlayCentered(v.text, box, m.contentWidth())
	}
	if box, ok := m.retargetOverlay(); ok {
		v.text = overlayCentered(v.text, box, m.contentWidth())
	}
	out := tea.NewView(v.text)
	out.AltScreen = true
	return out
}

func (m *Model) renderItem(it model.Item, sec model.Section, lay refLayout, selected bool, inner int) string {
	prefix := "  "
	if selected {
		prefix = styleCursor.Render("▸ ")
	}
	return prefix + renderCells(itemCells(it, sec, m.viewerLogin(it.Forge), lay), lay, inner)
}

func (m *Model) contentWidth() int {
	return max(38, m.outerWidth()-2)
}

func (m *Model) forgesStatusLine(now time.Time) string {
	names := m.sortedForgeNames()

	parts := make([]string, 0, len(names))
	for _, name := range names {
		st := m.statuses[name]
		mark := "✓"
		if !st.auth.OK {
			mark = "✗"
		}
		label := name + " " + mark + " " + lastRefreshLabel(st.updatedAt, now)
		if st.loading {
			label += "…"
		}
		parts = append(parts, label)
	}
	return styleCount.Render(strings.Join(parts, " · "))
}
