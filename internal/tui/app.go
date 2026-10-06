// Package tui implements prdash's cross-forge inbox in Bubbletea v2: three sections with each
// forge's rich data, an item detail, refresh with progressive loading, and approve/merge.
package tui

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"prdash/internal/cache"
	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/inbox"
	"prdash/internal/review/executor"
	"prdash/internal/state"
)

type event interface{}

type authMsg struct {
	cycle int
	forge string
	auth  model.AuthState
}

type pageMsg struct {
	cycle     int
	key       streamKey
	items     []model.Item
	next      string
	more      bool
	first     bool
	unchanged bool
	warnings  []model.Warning
}

type forgeDoneMsg struct {
	cycle int
	forge string
}

type refreshDoneMsg struct{ cycle int }

// cycle records the cycle in force when it was launched (see the policy in applyAction).
type actionMsg struct {
	cycle   int
	outcome forge.Outcome
}

type notifyMsg struct {
	text  string
	level noticeLevel
}

type toastTickMsg struct{}

type tickMsg struct{}

type mountMsg struct {
	result executor.Result
	err    error
}

// `base` is the merge warning captured when it was triggered, so the cleanup composes on that
// text rather than on the current one and does not overwrite what the merge brought.
type reviewCleanupMsg struct {
	base    string
	level   noticeLevel
	removed bool
	reason  string
	err     error
}

// A clock rather than a channel reader, so it does not disturb the single-reader invariant.
type commentsTickMsg struct{}

// id is the one asked for, not the one under the cursor: the answer is stored anyway because it will be
// needed again as soon as the selection returns, but it is only painted while still selected.
type commentsMsg struct {
	id   model.ID
	page forge.CommentPage
	err  string
}

type Mounter interface {
	Mount(ctx context.Context, it model.Item) (executor.Result, error)
}

// 60s; a literal because a const decl carries no coverage, so `*` here would be a mutant no test can reach (ADR 0011).
const refreshTimeout = time.Duration(60e9)

// 5m as a literal, same coverage argument as refreshTimeout above.
const mountTimeout = time.Duration(300e9)

// 60s as a literal, same coverage argument as refreshTimeout above.
const actionTimeout = time.Duration(60e9)

// Short because it must not leave the event hanging when the checkout does not answer.
// 30s as a literal, same coverage argument as refreshTimeout above.
const reviewCleanupTimeout = time.Duration(30e9)

// A read of a single PR: past this the problem is the forge and not worth waiting for, and the rest of
// the panel is still true. 20s as a literal, same coverage argument as refreshTimeout above.
const commentsTimeout = time.Duration(20e9)

// A clock and not an event, because looking is cheap: that spares it from being re-armed at every
// site where the selection changes — keys, pages arriving, actions — and going unasked at one.
// 200ms as a literal, same coverage argument as refreshTimeout above.
const commentsPoll = time.Duration(200e6)

// 10m as a literal, same coverage argument as refreshTimeout above.
const maxBackoff = time.Duration(600e9)

// The forge is unique per adapter, so it is enough with it plus section and kind.
type streamKey struct {
	forge   string
	section model.Section
	kind    model.ReviewKind
}

type stream struct {
	items      []model.Item
	cursor     string // cursor of the last page received
	headCursor string // "next" cursor of the first page of the last complete cycle
	complete   bool   // the list paginated fully in the last cycle
	more       bool
}

type streamHead struct {
	cursor   string
	complete bool
}

// The first page of a refresh matches the last complete cycle's header, so there is no need to keep
// paging (the rest did not change either) and what is loaded is kept.
func unchangedHead(prev streamHead, page forge.Page) bool {
	return prev.complete && page.More && page.Next != "" && page.Next == prev.cursor
}

type forgeStatus struct {
	forge     string
	host      string
	auth      model.AuthState
	warnings  []model.Warning
	updatedAt time.Time
	loading   bool
}

type sectionPos struct {
	cursor int
	scroll int
}

type noticeLevel int

const (
	levelNone noticeLevel = iota
	levelInfo
	levelOK
	levelWarn
	levelError
)

type Model struct {
	cfg      config.Config
	adapters []forge.Adapter
	byForge  map[string]forge.Adapter

	streams  map[streamKey]*stream
	statuses map[string]*forgeStatus
	inbox    inbox.Inbox

	// Assigned by default: it carries the assigned work, and the rest is one `tab` away.
	activeSection model.Section
	// The active section's are the cursor/scroll fields.
	pos map[model.Section]sectionPos
	// Global rather than per section: the hint bar names it once and it applies to whatever is painted.
	// Memory only, not persisted, so reopening the program goes back to common.
	prefixMode prefixMode

	cursor int
	// The view cannot re-adjust it (View cannot mutate the model): syncScroll maintains it on cursor moves
	// and rebuild when new data arrives.
	scroll int

	width, height int
	loading       bool
	backoff       time.Duration
	lastRefresh   time.Time

	// Keeps a second auto-refresh chain from starting while one is already scheduled.
	tickPending bool

	// The invariant is 1: every channel event consumes a reader and withPump arms it back, and no branch
	// that does not read the channel may arm one (or goroutines leak).
	readers int

	actionBusy bool
	denied     map[model.ID]string
	// Unlike `denied`, which the forge imposes and a successful refresh clears, this is a local deterministic
	// rule: derived from the item and the viewer's login on every render, not stored.
	selfDenied map[model.ID]string
	// So a refresh page captured before the action cannot revert its re-read state (see
	// reconcileFirstPage).
	actionCycle map[model.ID]int

	toast *toastManager

	// The state and not just the list is what tells "this PR has no comments" from "I have not asked
	// yet". Cached per item: re-asking every cycle would only make the card flicker.
	comments map[model.ID]*commentState

	cycle int

	mounter   Mounter
	mountBusy bool

	simulator Simulator
	graphics  Graphics
	// Without it, testing the good path of `openBrowserCmd` means RUNNING it, and that launches the
	// real browser on whoever runs the tests. A tea.Cmd is returned and then called.
	openURL func(url string) error
	sim     simPanel
	simSeq  int

	// Shares the capture rule with the simulation overlay —open, it takes the whole keyboard— and the two
	// are mutually exclusive: neither opens from inside the other.
	retarget  retargetPanel
	branchSeq int
	// Without it, opening the popup twice in a row on the same repo paginated the branches twice to change
	// your mind once.
	branchCache map[repoKey]branchCache
	// nil means nobody to ask, and the warning is lost.
	reviewLookup ReviewLookup
	// A separate port from reviewLookup because a capability that deletes cannot inherit a "read-only and
	// degradable" contract. nil means no auto-delete: the merge works and the worktree is kept.
	reviewRemover ReviewRemover

	// mergeArmedID pins the item that was armed, because a refresh can move the cursor in between and the
	// merge must go out on what the user confirmed, not on what is under it now.
	mergeArmed   bool
	mergeArmedID model.ID
	// Not a veto: it is the line the confirmation teaches so the second press is informed.
	mergeBlockReason string
	// Session-scoped, not per item, and toggled by `tab` while armed where it is visible. Starts true
	// because deleting the branch of a merged PR is what the forges do on their own.
	deleteBranch bool

	events  chan event
	ctx     context.Context
	cancel  context.CancelFunc
	spinner spinner.Model

	cachePath string
}

func New(cfg config.Config, adapters []forge.Adapter) Model {
	ctx, cancel := context.WithCancel(context.Background())

	statuses := make(map[string]*forgeStatus, len(adapters))
	byForge := make(map[string]forge.Adapter, len(adapters))
	for _, a := range adapters {
		statuses[a.Forge()] = &forgeStatus{
			forge:   a.Forge(),
			host:    a.Host(),
			auth:    model.AuthState{Forge: a.Forge(), OK: true},
			loading: true,
		}
		byForge[a.Forge()] = a
	}

	m := Model{
		cfg:         cfg,
		adapters:    adapters,
		byForge:     byForge,
		streams:     map[streamKey]*stream{},
		statuses:    statuses,
		denied:      map[model.ID]string{},
		selfDenied:  map[model.ID]string{},
		actionCycle: map[model.ID]int{},
		comments:    map[model.ID]*commentState{},
		events:      make(chan event, 256),
		toast:       newToastManager(),
		ctx:         ctx,
		cancel:      cancel,
		loading:     true,
		cycle:       1, // Init fires the first cycle
		readers:     1, // Init arms the first channel reader
		// Assigned is the section shown on open: it carries the assigned work and the rest is one `tab` away.
		activeSection: model.SectionReview,
		pos:           map[model.Section]sectionPos{},
		branchCache:   map[repoKey]branchCache{},
		// Branch deletion asked for by default; `tab` in the merge confirmation turns it off.
		deleteBranch: true,
	}
	m.tickPending = cfg.RefreshInterval > 0
	m.spinner = spinner.New(spinner.WithSpinner(spinner.Dot))

	if path, err := cache.Path(); err == nil {
		m.cachePath = path
		if f, ok := cache.Load(path); ok {
			m.applySnapshot(f)
		}
	}
	m.rebuild()
	return m
}

func (m *Model) SetMounter(mounter Mounter) { m.mounter = mounter }

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.launchRefresh(m.cycle),
		waitForEvent(m.events),
		m.spinner.Tick,
		m.tickCmd(),
		tickToast(),
		m.commentsCmd(),
	)
}

// Does not consume the events channel (it comes from tea.Every), so the single-reader invariant holds.
func tickToast() tea.Cmd {
	return tea.Every(toastTickInterval, func(time.Time) tea.Msg {
		return toastTickMsg{}
	})
}

func waitForEvent(ch <-chan event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}

func (m *Model) armReader() tea.Cmd {
	m.readers++
	return waitForEvent(m.events)
}

func sendEvent(ctx context.Context, ch chan<- event, ev event) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}

// The previous `more` is residue of the cycle that just ended: without clearing it, a lost page would
// leave the indicator stuck and the auto-refresh paused for good. The new pages will set it again.
func (m *Model) beginRefresh() (Model, tea.Cmd) {
	m.loading = true
	m.cycle++
	for _, st := range m.statuses {
		st.loading = true
		st.warnings = nil
	}
	for _, s := range m.streams {
		s.more = false
	}
	return *m, m.launchRefresh(m.cycle)
}

func (m *Model) launchRefresh(cycle int) tea.Cmd {
	appCtx := m.ctx
	events := m.events
	adapters := append([]forge.Adapter(nil), m.adapters...)

	prev := make(map[streamKey]streamHead, len(m.streams))
	for key, s := range m.streams {
		prev[key] = streamHead{cursor: s.headCursor, complete: s.complete}
	}

	go func() {
		ctx, cancel := context.WithTimeout(appCtx, refreshTimeout)
		defer cancel()

		var wg sync.WaitGroup
		for _, a := range adapters {
			wg.Add(1)
			go func(a forge.Adapter) {
				defer wg.Done()
				// The QUERY is bounded by a timeout; the EMISSION uses the app's context, so a timeout discards
				// neither pages nor a forge's end (both critical events) and does not hang the cycle.
				streamForge(ctx, appCtx, events, a, cycle, prev)
			}(a)
		}
		wg.Wait()
		sendEvent(appCtx, events, refreshDoneMsg{cycle: cycle})
	}()
	return nil
}

// queryCtx bounds the query (timeout, cancellation); emitCtx bounds event delivery. The events are
// critical: with emitCtx, which is the app's, they are not dropped when the query timeout expires.
func streamForge(queryCtx, emitCtx context.Context, events chan<- event, a forge.Adapter, cycle int, prev map[streamKey]streamHead) {
	sendEvent(emitCtx, events, authMsg{cycle: cycle, forge: a.Forge(), auth: a.Auth(queryCtx)})
	forge.Stream(queryCtx, a, func(p forge.PageResult) bool {
		key := streamKey{forge: a.Forge(), section: p.Query.Section, kind: p.Query.ReviewKind}
		if p.First && unchangedHead(prev[key], forge.Page{Next: p.Next, More: p.More}) {
			sendEvent(emitCtx, events, pageMsg{cycle: cycle, key: key, unchanged: true})
			return false
		}
		sendEvent(emitCtx, events, pageMsg{
			cycle:    cycle,
			key:      key,
			items:    p.Items,
			next:     p.Next,
			more:     p.More,
			first:    p.First,
			warnings: p.Warnings,
		})
		return true
	})
	sendEvent(emitCtx, events, forgeDoneMsg{cycle: cycle, forge: a.Forge()})
}

// Nil when the refresh is disabled (interval 0).
func (m *Model) tickCmd() tea.Cmd {
	d := m.tickInterval()
	if d <= 0 {
		return nil
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

// One tick chain, no matter how many Update calls arrive.
func (m *Model) armTick() tea.Cmd {
	if m.tickPending || m.tickInterval() <= 0 {
		return nil
	}
	m.tickPending = true
	return m.tickCmd()
}

func (m *Model) tickInterval() time.Duration {
	base := m.cfg.RefreshInterval
	if base <= 0 {
		return 0
	}
	return base + m.backoff
}

// Pagination is deliberately NOT consulted: the cycle is already paused while it pages, and a
// `more` surviving the end of the cycle is residue whose consultation would freeze the tick for good.
func (m *Model) paused() bool {
	return m.loading || m.actionBusy
}

func (m *Model) sectionLoadingMore(kind model.Section) bool {
	for key, s := range m.streams {
		if key.section == kind && s.more {
			return true
		}
	}
	return false
}

// An unchanged message keeps what is loaded and closes the stream's pagination.
func (m *Model) applyPage(msg pageMsg) {
	s := m.streams[msg.key]
	if s == nil {
		s = &stream{}
		m.streams[msg.key] = s
	}

	if msg.unchanged {
		s.more = false
		s.complete = true
		if st := m.statuses[msg.key.forge]; st != nil {
			st.updatedAt = time.Now()
		}
		m.rebuild()
		return
	}

	// Sealed here so the rules depending on them (the own-approval veto) do not depend on every adapter
	// stamping them.
	for i := range msg.items {
		msg.items[i].Section = msg.key.section
		if msg.key.section == model.SectionReview {
			msg.items[i].ReviewKind = msg.key.kind
		}
	}

	// An item seen in a successful refresh stops being denied: the permissions may be there now, or the
	// user is retrying with fresh state.
	for i := range msg.items {
		delete(m.denied, msg.items[i].ID())
	}

	if msg.first {
		s.items = m.reconcileFirstPage(s.items, msg.items, msg.cycle)
		s.headCursor = msg.next
	} else {
		s.items = append(s.items, msg.items...)
	}
	s.cursor = msg.next
	s.more = msg.more
	// A degraded fallback brings partial data and is not marked complete, so the incremental refresh
	// does not freeze it.
	if !msg.more && !hasDegraded(msg.warnings) {
		s.complete = true
	}

	if st := m.statuses[msg.key.forge]; st != nil {
		st.updatedAt = time.Now()
		st.warnings = appendWarnings(st.warnings, forge.StampSection(msg.warnings, msg.key.section))
	}
	m.rebuild()
}

func (m *Model) applyItemUpdate(it model.Item) {
	for _, s := range m.streams {
		for i := range s.items {
			if s.items[i].ID() == it.ID() {
				s.items[i] = mergeItem(s.items[i], it)
				m.rebuild()
				return
			}
		}
	}
	key := streamKey{forge: it.Forge, section: it.Section, kind: it.ReviewKind}
	s := m.streams[key]
	if s == nil {
		s = &stream{}
		m.streams[key] = s
	}
	s.items = append(s.items, it)
	m.rebuild()
}

// ItemState does not know the inbox section.
func mergeItem(old, fresh model.Item) model.Item {
	if fresh.Section == "" {
		fresh.Section = old.Section
	}
	if fresh.ReviewKind == "" {
		fresh.ReviewKind = old.ReviewKind
	}
	return fresh
}

// The rest of the page does win.
func (m *Model) reconcileFirstPage(old, fresh []model.Item, cycle int) []model.Item {
	if len(m.actionCycle) == 0 {
		return fresh
	}
	out := make([]model.Item, 0, len(fresh))
	for _, it := range fresh {
		if ac, ok := m.actionCycle[it.ID()]; ok && ac >= cycle {
			if prev, found := findItem(old, it.ID()); found {
				out = append(out, prev)
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

func findItem(items []model.Item, id model.ID) (model.Item, bool) {
	for _, it := range items {
		if it.ID() == id {
			return it, true
		}
	}
	return model.Item{}, false
}

func (m *Model) rebuild() {
	m.inbox = inbox.Build(m.forgeResults())
	m.refreshSelfDenied()
	m.clampCursor()
	// A refresh can change how many lines each section takes, so the scroll is readjusted to keep the
	// cursor inside the window.
	m.syncScroll()
}

func (m *Model) forgeResults() []inbox.ForgeResult {
	names := m.sortedForgeNames()
	out := make([]inbox.ForgeResult, 0, len(names))
	for _, name := range names {
		st := m.statuses[name]
		r := inbox.ForgeResult{
			Forge:    name,
			Host:     st.host,
			Authored: m.streamItems(name, model.SectionAuthored, ""),
			Mentions: m.streamItems(name, model.SectionMentions, ""),
			Warnings: st.warnings,
		}
		r.Review = append(r.Review, m.streamItems(name, model.SectionReview, model.ReviewRequested)...)
		r.Review = append(r.Review, m.streamItems(name, model.SectionReview, model.ReviewAssigned)...)
		out = append(out, r)
	}
	return out
}

func (m *Model) sortedForgeNames() []string {
	names := make([]string, 0, len(m.statuses))
	for name := range m.statuses {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m *Model) streamItems(forgeName string, section model.Section, kind model.ReviewKind) []model.Item {
	s := m.streams[streamKey{forge: forgeName, section: section, kind: kind}]
	if s == nil {
		return nil
	}
	return s.items
}

func (m *Model) viewerLogin(forgeName string) string {
	if st := m.statuses[forgeName]; st != nil {
		return st.auth.Login
	}
	return ""
}

// Derived from the item and the viewer's login, so a refresh bringing the item again re-marks it:
// nothing has to remember to clear it. There is no merge veto: the author CAN merge.
func (m *Model) refreshSelfDenied() {
	denied := make(map[model.ID]string)
	for _, it := range m.rows() {
		if ok, reason := state.CanApprove(it, m.viewerLogin(it.Forge)); !ok {
			denied[it.ID()] = reason
		}
	}
	m.selfDenied = denied
}

func (m *Model) clampCursor() {
	rows := m.rows()
	if len(rows) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = min(max(0, m.cursor), len(rows)-1)
}

// The inbox paints one section at a time, so the cursor and the navigation only walk that one; the
// rest is summarised in the border legend's count.
func (m *Model) rows() []model.Item {
	return m.sectionItems(m.activeSection)
}

// Not the legend's order (Mine · Assigned · Mentioned, the inbox's authority order): the cycle starts
// at the default section and the legend keeps the stacking order.
var sectionCycle = []model.Section{
	model.SectionReview,
	model.SectionMentions,
	model.SectionAuthored,
}

func (m *Model) sectionCycleIndex() int {
	for i, s := range sectionCycle {
		if s == m.activeSection {
			return i
		}
	}
	return 0
}

// Always cycles, even to an empty section: its state and count are exactly what the user wants to be
// able to see.
func (m *Model) cycleSection() {
	m.setActiveSection(sectionCycle[(m.sectionCycleIndex()+1)%len(sectionCycle)])
}

// The own-approval veto is recomputed: it derives from the visible items, and those are now another
// section's.
func (m *Model) setActiveSection(kind model.Section) {
	if m.pos == nil {
		m.pos = map[model.Section]sectionPos{}
	}
	m.pos[m.activeSection] = sectionPos{cursor: m.cursor, scroll: m.scroll}
	m.activeSection = kind
	p := m.pos[kind]
	m.cursor, m.scroll = p.cursor, p.scroll
	m.refreshSelfDenied()
	m.clampCursor()
	m.syncScroll()
}

func (m *Model) selected() (model.Item, bool) {
	rows := m.rows()
	if len(rows) == 0 || m.cursor >= len(rows) {
		return model.Item{}, false
	}
	return rows[m.cursor], true
}

func (m *Model) sectionItems(kind model.Section) []model.Item {
	return m.inbox.Section(kind).Items
}

func (m *Model) sectionProblems(kind model.Section) []string {
	var out []string
	for _, name := range m.sortedForgeNames() {
		st := m.statuses[name]
		for _, w := range st.warnings {
			if w.Section != kind {
				continue
			}
			out = append(out, problemText(name, w))
		}
	}
	return out
}

func problemText(forgeName string, w model.Warning) string {
	if w.Kind == "degraded" {
		return fmt.Sprintf("%s: %s", forgeName, w.Msg)
	}
	return fmt.Sprintf("%s: could not be queried (%s)", forgeName, problemLabel(w.Kind))
}

func hasDegraded(warns []model.Warning) bool {
	for _, w := range warns {
		if w.Kind == "degraded" {
			return true
		}
	}
	return false
}

func problemLabel(kind string) string {
	switch kind {
	case "auth":
		return "not authenticated"
	case "timeout":
		return "timeout"
	case "ratelimit":
		return "rate limited"
	case "parse":
		return "unreadable response"
	case "unsupported":
		return "unsupported"
	case "validation":
		return "rejected"
	case "network":
		return "no connection"
	default:
		return kind
	}
}

func lastRefreshLabel(since time.Time, now time.Time) string {
	if since.IsZero() {
		return "no data"
	}
	d := now.Sub(since)
	if d < time.Second {
		return "now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}

func (m *Model) applySnapshot(f cache.File) {
	for _, cs := range f.Streams {
		key := streamKey{forge: cs.Forge, section: cs.Section, kind: cs.Kind}
		m.streams[key] = &stream{items: cs.Items, cursor: cs.Cursor}
		if st := m.statuses[cs.Forge]; st != nil && cs.Host != "" {
			st.host = cs.Host
		}
	}
}

func (m *Model) snapshot() cache.File {
	keys := make([]streamKey, 0, len(m.streams))
	for key := range m.streams {
		keys = append(keys, key)
	}
	// The snapshot order is compared between runs, so the keys need one total order over the whole
	// key: forge, then section, then kind.
	slices.SortFunc(keys, func(a, b streamKey) int {
		return cmp.Or(
			cmp.Compare(a.forge, b.forge),
			cmp.Compare(a.section, b.section),
			cmp.Compare(a.kind, b.kind),
		)
	})

	f := cache.File{SavedAt: time.Now()}
	for _, key := range keys {
		s := m.streams[key]
		host := ""
		if st := m.statuses[key.forge]; st != nil {
			host = st.host
		}
		f.Streams = append(f.Streams, cache.Stream{
			Forge:   key.forge,
			Host:    host,
			Section: key.section,
			Kind:    key.kind,
			Cursor:  s.cursor,
			Items:   s.items,
		})
	}
	return f
}

func (m *Model) saveSnapshot() {
	if m.cachePath == "" {
		return
	}
	f := m.snapshot()
	path := m.cachePath
	go func() { _ = cache.Save(path, f) }()
}

func (m *Model) recomputeBackoff() {
	limited := false
	for _, st := range m.statuses {
		for _, w := range st.warnings {
			if w.Kind == "ratelimit" || w.Kind == "timeout" {
				limited = true
			}
		}
	}
	if !limited {
		m.backoff = 0
		return
	}
	base := m.cfg.RefreshInterval
	if base <= 0 {
		base = 60 * time.Second
	}
	if m.backoff == 0 {
		m.backoff = base
	} else {
		m.backoff = min(m.backoff*2, maxBackoff)
	}
}

// Pagination can repeat the same warning on every page.
func appendWarnings(dst, src []model.Warning) []model.Warning {
	for _, w := range src {
		dup := false
		for _, e := range dst {
			if e.Section == w.Section && e.Kind == w.Kind && e.Msg == w.Msg {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, w)
		}
	}
	return dst
}

// levelNone produces nothing: a warning with no level never gets painted.
func toastForLevel(level noticeLevel) (toastLevel, bool) {
	switch level {
	case levelOK:
		return toastSuccess, true
	case levelError:
		return toastError, true
	case levelWarn:
		return toastWarning, true
	case levelInfo:
		return toastInfo, true
	default:
		return toastInfo, false
	}
}

func (m *Model) setNoticeReplacing(prev, text string, level noticeLevel) {
	if lvl, ok := toastForLevel(level); ok {
		m.toast.replace(prev, text, lvl)
	}
}

func (m *Model) setNotice(text string, level noticeLevel) {
	if lvl, ok := toastForLevel(level); ok {
		m.toast.show(text, lvl)
	}
}

// Because wiring is seven `SetX` calls and a missing one does NOT break compilation: the failure
// surfaces on the keypress as a "missing dependency" naming none of the seven.
type Wiring struct {
	Mounter       Mounter
	Simulator     Simulator
	Graphics      Graphics
	ReviewLookup  ReviewLookup
	ReviewRemover ReviewRemover
}

func (m *Model) Wiring() Wiring {
	return Wiring{
		Mounter:       m.mounter,
		Simulator:     m.simulator,
		Graphics:      m.graphics,
		ReviewLookup:  m.reviewLookup,
		ReviewRemover: m.reviewRemover,
	}
}
