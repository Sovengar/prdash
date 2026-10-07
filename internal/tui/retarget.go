package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/worktree"
)

const (
	retargetChooserWidth  = 64
	retargetRows          = 12
	retargetMinRows       = 3
	retargetChrome        = 5
	retargetMargin        = 2
	retargetCurrentSuffix = "  · current"
)

const retargetTimeout = time.Duration(30e9)

const branchCacheTTL = time.Duration(300e9)

type retargetState int

const (
	retargetClosed retargetState = iota
	retargetListing
	retargetChoosing
	retargetConfirm
)

type retargetPanel struct {
	state  retargetState
	item   model.Item
	all    []string
	view   []string
	win    int
	cursor int
	query  string
	chosen string
	errMsg string
}

type repoKey struct {
	forge   string
	host    string
	project string
}

func keyOf(it model.Item) repoKey {
	return repoKey{forge: it.Forge, host: it.Host, project: it.Ref.Project}
}

type branchCache struct {
	names     []string
	fetchedAt time.Time
}

type ReviewLookup interface {
	ActiveReview(it model.Item) (worktree.Worktree, bool)
}

type ReviewRemover interface {
	RemoveReview(ctx context.Context, it model.Item) (removed bool, reason string, err error)
}

func (m *Model) SetReviewRemover(r ReviewRemover) { m.reviewRemover = r }

type branchesMsg struct {
	seq    int
	key    repoKey
	names  []string
	errMsg string
}

func (m *Model) SetReviewLookup(l ReviewLookup) { m.reviewLookup = l }

func (m Model) openRetarget() (tea.Model, tea.Cmd) {
	it, _, ok := m.canAction(forge.ActionRetarget)
	if !ok {
		return m, nil
	}

	m.disarmMerge()
	m.retarget = retargetPanel{state: retargetListing, item: it}

	if names, ok := m.cachedBranches(keyOf(it)); ok {
		m.fillBranches(names)
		return m, nil
	}
	return m, m.fetchBranches(it)
}

func (m *Model) fetchBranches(it model.Item) tea.Cmd {
	m.branchSeq++
	seq, appCtx, events := m.branchSeq, m.ctx, m.events
	a := m.byForge[it.Forge]
	key := keyOf(it)
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, retargetTimeout)
		defer cancel()
		names, warns := a.Branches(ctx, it.Ref)
		msg := branchesMsg{seq: seq, key: key, names: names}
		if len(warns) > 0 {
			msg.errMsg = warnMsg(warns)
		}
		sendEvent(appCtx, events, msg)
	}()
	return nil
}

func warnMsg(warns []model.Warning) string {
	for _, w := range warns {
		if strings.TrimSpace(w.Msg) != "" {
			return w.Msg
		}
	}
	return "the branches could not be read"
}

func (m *Model) applyBranches(msg branchesMsg) {
	if msg.seq != m.branchSeq || m.retarget.state == retargetClosed {
		return
	}
	if msg.errMsg != "" {
		m.retarget.errMsg = msg.errMsg
		return
	}
	if len(msg.names) == 0 {
		m.retarget.errMsg = "the repository has no branches to choose from"
		return
	}
	m.storeBranches(msg.key, msg.names)
	m.fillBranches(msg.names)
}

func (m *Model) fillBranches(names []string) {
	m.retarget.all = orderBranches(names, m.retarget.item.TargetBranch)
	m.retarget.cursor, m.retarget.win, m.retarget.query = 0, 0, ""
	m.retarget.view = filterBranches(m.retarget.all, "")
	m.retarget.state = retargetChoosing
}

func (m *Model) cachedBranches(key repoKey) ([]string, bool) {
	entry, ok := m.branchCache[key]
	if !ok || !branchCacheFresh(entry.fetchedAt, time.Now()) {
		return nil, false
	}
	return entry.names, true
}

func branchCacheFresh(fetchedAt, now time.Time) bool {
	return now.Sub(fetchedAt) <= branchCacheTTL
}

func (m *Model) storeBranches(key repoKey, names []string) {
	if m.branchCache == nil {
		m.branchCache = map[repoKey]branchCache{}
	}
	m.branchCache[key] = branchCache{names: names, fetchedAt: time.Now()}
}

func orderBranches(names []string, base string) []string {
	seen := make(map[string]bool, len(names))
	rest := make([]string, 0, len(names))
	head := ""
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if name == base && head == "" {
			head = name
			continue
		}
		rest = append(rest, name)
	}
	sort.Strings(rest)
	if head == "" {
		return rest
	}
	return append([]string{head}, rest...)
}

func filterBranches(all []string, query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return slices.Clone(all)
	}
	out := make([]string, 0, len(all))
	for _, name := range all {
		if strings.Contains(strings.ToLower(name), q) {
			out = append(out, name)
		}
	}
	return out
}

func (m *Model) applyQuery() {
	m.retarget.view = filterBranches(m.retarget.all, m.retarget.query)
	m.retarget.cursor, m.retarget.win = 0, 0
}

func (m *Model) clampRetargetCursor() {
	if len(m.retarget.view) == 0 {
		m.retarget.cursor = 0
		return
	}
	m.retarget.cursor = min(max(0, m.retarget.cursor), len(m.retarget.view)-1)
}

func (m Model) selectedBranch() (string, bool) {
	if m.retarget.cursor < 0 || m.retarget.cursor >= len(m.retarget.view) {
		return "", false
	}
	return m.retarget.view[m.retarget.cursor], true
}

func (m *Model) closeRetarget() {
	m.branchSeq++
	m.retarget = retargetPanel{}
}

func (m Model) handleRetargetKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		m.closeRetarget()
		m.cancel()
		return m, tea.Quit
	case "esc":
		if m.retarget.state == retargetConfirm {
			m.retarget.state = retargetChoosing
			return m, nil
		}
		m.closeRetarget()
		return m, nil
	}

	switch m.retarget.state {
	case retargetListing:
		return m, nil

	case retargetConfirm:
		if key == "enter" {
			return m, m.startRetarget(m.retarget.chosen)
		}
		return m, nil

	default:
		return m.handleRetargetSearchKey(msg, key)
	}
}

func (m Model) handleRetargetSearchKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		branch, ok := m.selectedBranch()
		if !ok {
			return m, nil
		}
		if branch == m.retarget.item.TargetBranch {
			m.closeRetarget()
			m.setNotice("the target branch is already "+branch, levelInfo)
			return m, nil
		}
		m.retarget.chosen = branch
		m.retarget.state = retargetConfirm
		return m, nil

	case "up":
		m.moveRetargetCursor(-1)
		return m, nil
	case "down":
		m.moveRetargetCursor(1)
		return m, nil

	case "ctrl+u":
		if m.retarget.query != "" {
			m.retarget.query = ""
			m.applyQuery()
		}
		return m, nil

	case "backspace":
		if r := []rune(m.retarget.query); len(r) > 0 {
			m.retarget.query = string(r[:len(r)-1])
			m.applyQuery()
		}
		return m, nil
	}

	if m.retarget.query == "" {
		switch key {
		case "k":
			m.moveRetargetCursor(-1)
			return m, nil
		case "j":
			m.moveRetargetCursor(1)
			return m, nil
		}
	}

	if msg.Text != "" && !msg.Mod.Contains(tea.ModCtrl) {
		m.retarget.query += msg.Text
		m.applyQuery()
	}
	return m, nil
}

func (m *Model) moveRetargetCursor(delta int) {
	m.retarget.cursor += delta
	m.clampRetargetCursor()
	m.retarget.win = m.retargetWindow()
}

func (m *Model) retargetWindow() int {
	return retargetWindowFor(m.retarget.win, m.retarget.cursor, m.retargetRows(), len(m.retarget.view))
}

func retargetWindowFor(win, cursor, rows, total int) int {
	if cursor < win {
		win = cursor
	} else if cursor >= win+rows {
		win = cursor - rows + 1
	}
	return max(0, min(win, max(0, total-rows)))
}

func (m Model) retargetVisible() ([]string, int) {
	return retargetVisibleFrom(m.retarget.view, m.retarget.win, m.retargetRows())
}

func retargetVisibleFrom(view []string, win, rows int) ([]string, int) {
	start := min(win, len(view))
	return view[start:][:min(rows, len(view)-start)], start
}

func (m Model) retargetRows() int {
	free := m.height - 2*retargetMargin - retargetChrome
	return min(retargetRows, max(retargetMinRows, free))
}

func (m Model) retargetBoxWidth() int {
	return min(m.contentWidth(), retargetChooserWidth)
}

func (m Model) retargetOverlay() (string, bool) {
	switch m.retarget.state {
	case retargetListing:
		return m.retargetBusyBox(), true
	case retargetChoosing:
		return m.retargetSearchBox(), true
	case retargetConfirm:
		return m.retargetConfirmBox(), true
	default:
		return "", false
	}
}

func (m Model) retargetBusyBox() string {
	width := m.retargetBoxWidth()
	if m.retarget.errMsg != "" {
		body := styleError.Render(m.retarget.errMsg) + "\n" + styleHint.Render("esc close")
		return borderedBox(" retarget "+refLabel(m.retarget.item), body, width)
	}
	body := m.spinner.View() + styleInfo.Render(" reading the branches…")
	body += "\n" + styleHint.Render("esc close")
	return borderedBox(" retarget "+refLabel(m.retarget.item), body, width)
}

func (m Model) retargetSearchBox() string {
	width := m.retargetBoxWidth()
	it := m.retarget.item
	inner := max(8, width-2)

	from := it.TargetBranch
	if from == "" {
		from = "unknown"
	}
	header := styleRef.Render("from ") + styleCount.Render(from) + styleDim.Render("  ·  ") +
		styleDim.Render(branchCountLabel(m))

	var body []string
	if m.retarget.errMsg != "" {
		body = append(body, styleError.Render(m.retarget.errMsg))
	} else if len(m.retarget.view) == 0 {
		body = append(body, styleEmpty.Render("no branch matches the filter"))
	} else {
		names, start := m.retargetVisible()
		for i, name := range names {
			marker := "  "
			if start+i == m.retarget.cursor {
				marker = styleCursor.Render("▸ ")
			}
			suffix := ""
			room := inner - ansi.StringWidth(marker)
			if name == it.TargetBranch {
				suffix = styleDim.Render(retargetCurrentSuffix)
				room -= ansi.StringWidth(retargetCurrentSuffix)
			}
			body = append(body, marker+styleRef.Render(padRight(ansi.Truncate(name, room, ""), room))+suffix)
		}
	}
	body = append(body, "")

	filter := styleDim.Render("filter ")
	if m.retarget.query == "" {
		filter += styleEmpty.Render("(type to search)")
	} else {
		filter += styleOK.Render(m.retarget.query)
	}
	help := styleHint.Render("↑↓ move · enter choose · esc close")

	return borderedBox(" retarget "+refLabel(it),
		strings.Join(append([]string{header}, append(body, filter, help)...), "\n"), width)
}

func branchCountLabel(m Model) string {
	total := len(m.retarget.all)
	if m.retarget.query == "" {
		return pluralBranches(total)
	}
	return fmt.Sprintf("%d of %s match", len(m.retarget.view), pluralBranches(total))
}

func pluralBranches(n int) string {
	if n == 1 {
		return "1 branch"
	}
	return fmt.Sprintf("%d branches", n)
}

func (m Model) retargetConfirmBox() string {
	width := m.retargetBoxWidth()
	from := m.retarget.item.TargetBranch
	if from == "" {
		from = "unknown"
	}
	flow := styleCount.Render(padRight(from, max(1, (width-6)/2))) +
		styleDim.Render(" → ") + styleOK.Render(m.retarget.chosen)

	body := flow + "\n\n" +
		styleDim.Render("the diff, the checks and the mergeability are recomputed") + "\n" +
		styleHint.Render("enter apply · esc back")
	return borderedBox(" retarget "+refLabel(m.retarget.item), body, width)
}

func (m *Model) startRetarget(branch string) tea.Cmd {
	it := m.retarget.item
	a, ok := m.canActionOn(forge.ActionRetarget, it)
	if !ok {
		m.closeRetarget()
		return nil
	}
	from := it.TargetBranch
	m.closeRetarget()
	return m.launchAction(forge.ActionRetarget, it, a,
		retargetProgressNotice(from, branch),
		func(ctx context.Context) forge.Outcome {
			out := forge.RunRetarget(ctx, a, it.Ref, it.Number, branch)
			out.FromBase = from
			return out
		})
}

func retargetProgressNotice(from, to string) string {
	if from == "" {
		return string(forge.ActionRetarget) + " (→ " + to + ") in progress…"
	}
	return string(forge.ActionRetarget) + " (" + from + " → " + to + ") in progress…"
}

func (m Model) staleReviewNotice(it model.Item) string {
	if m.reviewLookup == nil || it.Number == 0 {
		return ""
	}
	wt, ok := m.reviewLookup.ActiveReview(it)
	if !ok || wt.Path == "" {
		return ""
	}
	return "the mounted review still has the old base: mount the review again to pick it up"
}
