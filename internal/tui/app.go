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

type reviewCleanupMsg struct {
	base    string
	level   noticeLevel
	removed bool
	reason  string
	err     error
}

type commentsTickMsg struct{}

type commentsMsg struct {
	id   model.ID
	page forge.CommentPage
	err  string
}

type Mounter interface {
	Mount(ctx context.Context, it model.Item) (executor.Result, error)
}

const refreshTimeout = time.Duration(60e9)

const mountTimeout = time.Duration(300e9)

const actionTimeout = time.Duration(60e9)

const reviewCleanupTimeout = time.Duration(30e9)

const commentsTimeout = time.Duration(20e9)

const commentsPoll = time.Duration(200e6)

const maxBackoff = time.Duration(600e9)

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

	activeSection model.Section
	pos           map[model.Section]sectionPos
	prefixMode    prefixMode

	cursor int
	scroll int

	width, height int
	loading       bool
	backoff       time.Duration
	lastRefresh   time.Time

	tickPending bool

	readers int

	actionBusy  bool
	denied      map[model.ID]string
	selfDenied  map[model.ID]string
	actionCycle map[model.ID]int

	toast *toastManager

	comments map[model.ID]*commentState

	cycle int

	mounter   Mounter
	mountBusy bool

	simulator Simulator
	graphics  Graphics
	openURL   func(url string) error
	sim       simPanel
	simSeq    int

	retarget      retargetPanel
	branchSeq     int
	branchCache   map[repoKey]branchCache
	reviewLookup  ReviewLookup
	reviewRemover ReviewRemover

	mergeArmed       bool
	mergeArmedID     model.ID
	mergeBlockReason string
	deleteBranch     bool

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
		cfg:           cfg,
		adapters:      adapters,
		byForge:       byForge,
		streams:       map[streamKey]*stream{},
		statuses:      statuses,
		denied:        map[model.ID]string{},
		selfDenied:    map[model.ID]string{},
		actionCycle:   map[model.ID]int{},
		comments:      map[model.ID]*commentState{},
		events:        make(chan event, 256),
		toast:         newToastManager(),
		ctx:           ctx,
		cancel:        cancel,
		loading:       true,
		cycle:         1, // Init fires the first cycle
		readers:       1, // Init arms the first channel reader
		activeSection: model.SectionReview,
		pos:           map[model.Section]sectionPos{},
		branchCache:   map[repoKey]branchCache{},
		deleteBranch:  true,
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
				// Query bounded by a timeout, emission by the app's context, so critical events are never dropped.
				streamForge(ctx, appCtx, events, a, cycle, prev)
			}(a)
		}
		wg.Wait()
		sendEvent(appCtx, events, refreshDoneMsg{cycle: cycle})
	}()
	return nil
}

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

func (m *Model) tickCmd() tea.Cmd {
	d := m.tickInterval()
	if d <= 0 {
		return nil
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

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

	for i := range msg.items {
		msg.items[i].Section = msg.key.section
		if msg.key.section == model.SectionReview {
			msg.items[i].ReviewKind = msg.key.kind
		}
	}

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

func mergeItem(old, fresh model.Item) model.Item {
	if fresh.Section == "" {
		fresh.Section = old.Section
	}
	if fresh.ReviewKind == "" {
		fresh.ReviewKind = old.ReviewKind
	}
	return fresh
}

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

func (m *Model) rows() []model.Item {
	return m.sectionItems(m.activeSection)
}

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

func (m *Model) cycleSection() {
	m.setActiveSection(sectionCycle[(m.sectionCycleIndex()+1)%len(sectionCycle)])
}

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
