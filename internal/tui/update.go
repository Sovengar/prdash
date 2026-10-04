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
		// With the image in the graphics layer it has to be placed again — Herdr places by cells, not by
		// Relative — or it stays in the old rectangle, which is where the box was before the resize.
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
			// Stale cycle: its data is dropped but the bomb is ALWAYS re-armed, or channel readers are lost
			// and the refresh dies.
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
			// Stale cycle (cycles should not overlap). It touches no data, no `loading` and no tick chain;
			// it only re-arms the bomb because it consumed a channel event.
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
		// Prunes the expired warnings and re-arms the tick. It does not touch the events channel: it is a
		// clock, not a reader.
		m.toast.update()
		return m, tickToast()

	case mountMsg:
		m.mountBusy = false
		m.applyMount(msg.result, msg.err)
		return m.withPump(nil)

	case reviewCleanupMsg:
		// Produced by reviewCleanupCmd, not the events channel: it consumes no reader so none is re-armed
		// (arming one would leak a goroutine per merge). Same pattern as notifyMsg.
		m.applyReviewCleanup(msg)
		return m, nil

	case simMsg:
		m.applySim(msg)
		return m.withPump(nil)

	case branchesMsg:
		m.applyBranches(msg)
		return m.withPump(nil)

	case commentsTickMsg:
		// The tick is always re-armed, whether anything was queried or not: that is what keeps the chain
		// alive and makes the next selection change noticeable without having to remember it at the change site.
		return m, m.requestComments()

	case commentsMsg:
		m.applyComments(msg)
		return m.withPump(nil)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// One reader in, exactly one armed back out.
func (m Model) withPump(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.releaseReader()
	return m, tea.Batch(cmd, m.armReader())
}

func (m *Model) releaseReader() {
	if m.readers > 0 {
		m.readers--
	}
}

// The re-read item is the most recent state the forge has — it is read AFTER the action — so it
// always applies; dropping it would revert the item. The cycle is per item so a page captured before
// the action cannot overwrite it.
func (m *Model) applyAction(out forge.Outcome, cycle int) tea.Cmd {
	m.actionBusy = false
	if out.HasItem {
		m.actionCycle[out.Item.ID()] = cycle
		// An action can write to the conversation (an approve leaves a review note), and this is the
		// ONLY invalidation of that cache: the inbox refresh does not clear it because re-asking every cycle
		// would make the card flicker.
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
		// The reason already comes translated and says what to do, so the header only names the action:
		// "forge conflict" there would lie, because a refresh does not rebase a branch.
		m.setNotice(string(out.Kind)+" refused: "+out.Msg, levelError)
	case out.OK:
		// The branch delete is warned about even when the merge went: the notice has to say both things,
		// because "merge ok" alone leaves it unknown whether the branch it was asked to delete is gone.
		notice := actionDoneNotice(out)
		level := levelOK
		if out.DeleteMsg != "" {
			notice += " · branch not deleted: " + out.DeleteMsg
			level = levelWarn
		} else if stale := m.staleReviewNotice(out.Item); stale != "" && out.Kind == forge.ActionRetarget {
			// Same for a mounted review: moving the base does not touch it, so it stands with whatever the item
			// had. Warned, not fixed, because the worktree is the user's.
			notice += " · " + stale
			level = levelWarn
		}
		m.setNotice(notice, level)
		// Only a merge that actually went triggers the worktree cleanup. The base warning is captured here and
		// travels in the command so the notice recomposes on top of it.
		if triggersReviewCleanup(out) {
			return m.reviewCleanupCmd(out.Item, notice, level)
		}
	default:
		m.setNotice("error: "+out.Msg, levelError)
	}
	return nil
}

// Only a merge that went. Isolated as the trigger so a test can pin it deterministically.
func triggersReviewCleanup(out forge.Outcome) bool {
	return out.Kind == forge.ActionMerge && out.OK
}

// In the background so the Update handler does not block on a git subprocess, and the result
// arrives with the base warning already captured, so the notice recomposes on it instead of
// overwriting what the merge brought.
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

// A no-op (no review mounted, or the path already gone) re-emits nothing: the merge notice stays the
// same and re-stacking it would only duplicate it.
func (m *Model) applyReviewCleanup(msg reviewCleanupMsg) {
	if !msg.removed && msg.reason == "" && msg.err == nil {
		return
	}
	text, level := reviewCleanupNotice(msg.base, msg.level, msg.removed, msg.reason, msg.err)
	// The merge warning is UPDATED rather than stacked: the merge text must not appear twice.
	m.setNoticeReplacing(msg.base, text, level)
}

// Never suppresses the merge's facts; it prefixes them with the worktree's outcome.
func reviewCleanupNotice(base string, baseLevel noticeLevel, removed bool, reason string, err error) (string, noticeLevel) {
	switch {
	case err != nil:
		// The merge did go: a failure to delete the worktree is a warning, not a failed action.
		return base + " · could not remove the worktree: " + err.Error(), levelWarn
	case removed:
		level := levelOK
		if baseLevel == levelWarn {
			level = levelWarn
		}
		return base + " · worktree removed", level
	case reason != "":
		return base + " · " + keptReviewNotice(reason), levelWarn
	default:
		return base, baseLevel
	}
}

func keptReviewNotice(reason string) string {
	if reason == worktree.KeptUncommitted {
		return "merged, but the worktree has uncommitted changes — kept"
	}
	return "worktree kept: " + reason
}

// There is no full-screen view to open or close: the item's card is always in the bottom panel, so
// every key works at all times.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Merge is the only two-stage action. While armed only the mode key, `esc` and quitting count;
	// any other key disarms and behaves as if the merge had not been pressed, so an out-of-time `m` does
	// not leave the view waiting for a second press.
	if m.mergeArmed {
		return m.handleMergeArmed(msg, key)
	}

	// The simulation overlay takes the whole keyboard while open: it is a question with concrete answers
	// and any other key closes it, so letting one through would fire actions on an item the user is no
	// longer looking at.
	if m.sim.state != simClosed {
		return m.handleSimKey(msg, key)
	}

	// The retarget popup does the same for the same reason. It comes after the simulation one and not before
	// because the two are mutually exclusive (neither opens from inside the other), so the order only
	// decides which wins if they ever overlap, and reading order wins.
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

// It does not move the cursor but it does change what the ITEM column measures and, in full and leaf, it
// removes the prefix line: without resyncing the scroll, a scrolled list would leave the cursor outside
// the window exactly when the mode changed. Same reason goTop carries its own scroll.
func (m *Model) cyclePrefixMode() {
	m.prefixMode = m.prefixMode.next()
	m.syncScroll()
}

// A hard block prevents arming: it is a property of the forge and no key lifts it. A soft block
// still arms, and naming the mode IS the confirmation, because naming a strategy having read that
// the CI is red is having decided.
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

// No default mode, so the confirmation and the choice are one gesture. `tab` is the exception: it
// picks no mode and does NOT disarm, being the other decision the merge names. A key that is not a
// mode CONSUMES the press and cancels: it used to delegate to handleKey, so `m` then `a` approved.
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
		// `tab` with the merge armed toggles the delete and is not "next section": the armed state keeps
		// waiting for the mode key, which is the only thing that can fire it.
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

	// A refresh may have moved the cursor between arming and confirming. If it is no longer the same
	// item, the merge goes out on what the user confirmed or it does not go out.
	it, ok := m.selected()
	if !ok || it.ID() != m.mergeArmedID {
		m.disarmMerge()
		m.setNotice("the selected item changed: press merge again", levelWarn)
		return m, nil
	}
	// Checked against the copy on screen and not against RunAction's re-read on purpose: a refresh between
	// arming and confirming may have changed the rules, and refusing here a mode the repo no longer
	// allows is more honest than emitting a merge the forge will reject with a less clear message.
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

// No channel reader armed: the local refresh consumes no events, so the bomb keeps its single reader.
func (m Model) startRefresh() (tea.Model, tea.Cmd) {
	if m.loading {
		m.setNotice("refresh in progress", levelInfo)
		return m, nil
	}
	updated, cmd := m.beginRefresh()
	return updated, cmd
}

// Shared by arming a merge, opening the branch picker and executing it: arming an action that then
// cannot be executed would leave the user with a confirmation that can only end in disappointment.
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
	// No forge allows approving your own: cut here, before spending the CLI call and its re-read, and
	// without waiting for the rejection. The reason already says what happened, so it carries no prefix.
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

// The branch is named only when it was asked for and the forge did not complain, which is the only case
// where it can be asserted as deleted: a failed delete is reported by RunAction in DeleteMsg, and a
// fork PR never deletes it.
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

// What changes between actions is the three arguments, which is why it is split out. `actionBusy`
// is set HERE because that is what stops two actions stepping on each other, and two functions
// setting it would forget once.
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
		return string(kind) + " (" + mode.Label() + ") en curso…"
	}
	return string(kind) + " en curso…"
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

// All cursor movement goes through here: moving without syncScroll could leave the row outside the
// window with the selection elsewhere on screen.
func (m *Model) moveCursor(row int) {
	m.cursor = row
	m.clampCursor()
	m.syncScroll()
}

// Moving the cursor is not enough: the auto-scroll would put the row under the top edge, taking the
// prefix line and the column header off screen.
func (m *Model) goTop() {
	m.cursor = 0
	m.scroll = 0
}

// Moving only the cursor would leave the list nearly still and the detail panel jumping from item to
// item.
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
	// Only the interior, so no border is stepped on. There is no `if len(toasts) > 0` and there used
	// to be: overlayToasts returns the view intact with no warnings, so the guard only stayed alive in
	// the allowlist.
	v.text = overlayToasts(v.text, m.toast.blocks(m.contentWidth()), m.contentWidth(), v.rows)
	// The popup goes after the toasts so it ends up above them: it is the layer the user just opened, and
	// a warning must not cover it.
	if box, ok := m.simOverlay(); ok {
		v.text = overlayCentered(v.text, box, m.contentWidth())
	}
	// The retarget popup goes last for the same reason the simulation one goes after the toasts, and in
	// the same order between the two: if they ever overlapped, the one opened later wins.
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
