// Package tui implementa el inbox cross-forge en Bubbletea v2: tres secciones
// (creados por mí / review asignados / menciones) con datos ricos de cada
// forge, detalle de ítem, refresco manual y automático con carga progresiva, y
// acciones approve/merge.
package tui

import (
	"context"
	"fmt"
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
)

// event es el mensaje unificado del canal de trabajo en segundo plano.
type event interface{}

// authMsg entrega el estado de autenticación de un forge.
type authMsg struct {
	cycle int
	forge string
	auth  model.AuthState
}

// pageMsg entrega una página de una lista de un forge. unchanged señala que la
// cabecera no cambió y que se conserva lo ya cargado.
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

// forgeDoneMsg marca el fin de la consulta de un forge.
type forgeDoneMsg struct {
	cycle int
	forge string
}

// refreshDoneMsg marca el fin de un ciclo de refresco.
type refreshDoneMsg struct{ cycle int }

// actionMsg entrega el resultado de una acción rápida. cycle registra el ciclo
// vigente al lanzarla (ver política en applyAction).
type actionMsg struct {
	cycle   int
	outcome forge.Outcome
}

// notifyMsg entrega un aviso efímero para la cabecera.
type notifyMsg struct {
	text  string
	level noticeLevel
}

// tickMsg dispara el refresco automático.
type tickMsg struct{}

// refreshTimeout es el límite de un ciclo completo de consulta a los forges.
const refreshTimeout = 60 * time.Second

// actionTimeout es el límite de una acción approve/merge (incluye releer).
const actionTimeout = 60 * time.Second

// maxBackoff es el tope del backoff por rate limit.
const maxBackoff = 10 * time.Minute

// streamKey identifica una lista paginable del inbox. El forge es único por
// adapter, así que basta con él (más sección y tipo).
type streamKey struct {
	forge   string
	section model.Section
	kind    model.ReviewKind
}

// stream acumula las páginas de una lista.
type stream struct {
	items      []model.Item
	cursor     string // cursor de la última página recibida
	headCursor string // cursor "next" de la primera página del último ciclo completo
	complete   bool   // la lista se paginó entera en el último ciclo
	more       bool
}

// streamHead es la cabecera recordada de un stream, para decidir si un refresco
// cambió algo sin volver a paginar todo.
type streamHead struct {
	cursor   string
	complete bool
}

// unchangedHead indica si la primera página de un refresco coincide con la
// cabecera del último ciclo completo: en ese caso no hace falta seguir
// paginando (el resto tampoco cambió) y se conserva lo cacheado.
func unchangedHead(prev streamHead, page forge.Page) bool {
	return prev.complete && page.More && page.Next != "" && page.Next == prev.cursor
}

// forgeStatus es el estado de consulta de un forge.
type forgeStatus struct {
	forge     string
	host      string
	auth      model.AuthState
	warnings  []model.Warning
	updatedAt time.Time
	loading   bool
}

// noticeLevel clasifica el aviso de la cabecera.
type noticeLevel int

const (
	levelNone noticeLevel = iota
	levelInfo
	levelOK
	levelWarn
	levelError
)

// Model es el modelo raíz de la TUI.
type Model struct {
	cfg      config.Config
	adapters []forge.Adapter
	byForge  map[string]forge.Adapter

	streams  map[streamKey]*stream
	statuses map[string]*forgeStatus
	inbox    inbox.Inbox

	cursor int

	detailOpen bool
	detailID   model.ID
	detailItem model.Item // último estado conocido, por si el ítem sale del inbox

	width, height int
	loading       bool
	backoff       time.Duration
	lastRefresh   time.Time

	// tickPending evita armar una segunda cadena de auto-refresco mientras ya
	// hay un tick agendado.
	tickPending bool

	// readers es el número de lectores del canal en vuelo. El invariante es 1:
	// cada evento del canal consume un lector y withPump lo rearma; ninguna
	// rama que no lea del canal debe armar uno (si no, se filtran goroutines).
	readers int

	actionBusy bool
	denied     map[model.ID]string

	notice string
	level  noticeLevel

	cycle int

	events  chan event
	ctx     context.Context
	cancel  context.CancelFunc
	spinner spinner.Model

	// cachePath es la ruta del snapshot; vacía = sin cache (tests).
	cachePath string
}

// New construye el modelo con la config y los adapters habilitados. Pinta el
// snapshot cacheado si existe y arranca el primer refresco en Init.
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
		cfg:      cfg,
		adapters: adapters,
		byForge:  byForge,
		streams:  map[streamKey]*stream{},
		statuses: statuses,
		denied:   map[model.ID]string{},
		events:   make(chan event, 256),
		ctx:      ctx,
		cancel:   cancel,
		loading:  true,
		cycle:    1, // el primer ciclo lo lanza Init
		readers:  1, // Init arma el primer lector del canal
	}
	// Init arma el primer tick si el auto-refresco está habilitado.
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

// Init lanza el primer refresco, la bomba de eventos, el spinner y el tick.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.launchRefresh(m.cycle),
		waitForEvent(m.events),
		m.spinner.Tick,
		m.tickCmd(),
	)
}

// waitForEvent lee UN evento del canal: el patrón de Bubbletea es devolver un
// Cmd por evento y rearmarlo tras cada uno.
func waitForEvent(ch <-chan event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}

// armReader arma un lector del canal y refleja el invariante en el contador.
func (m *Model) armReader() tea.Cmd {
	m.readers++
	return waitForEvent(m.events)
}

// sendEvent publica en el canal respetando la cancelación.
func sendEvent(ctx context.Context, ch chan<- event, ev event) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}

// beginRefresh marca el arranque de un ciclo: incrementa el contador (para
// descartar eventos viejos), deja los forges en carga, limpia sus warnings y
// reinicia la paginación de cada lista. El `more` anterior es residuo del ciclo
// que acaba de terminar (si no, una página perdida dejaría el indicador pegado
// y el auto-refresco pausado para siempre); las páginas nuevas lo volverán a
// marcar. Devuelve el modelo actualizado y el Cmd que lanza las consultas.
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

// launchRefresh consulta todos los forges en paralelo y emite sus páginas de
// forma progresiva. No muta el modelo: el ciclo va en cada mensaje.
func (m *Model) launchRefresh(cycle int) tea.Cmd {
	appCtx := m.ctx
	events := m.events
	adapters := append([]forge.Adapter(nil), m.adapters...)

	// Cabeceras recordadas para el refresco incremental (comparación por cursor).
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
				// La consulta se acota por timeout; la EMISIÓN va con el ctx
				// de la app para que un timeout no descarte páginas ni el fin
				// de un forge (eventos críticos) y cuelgue el ciclo.
				streamForge(ctx, appCtx, events, a, cycle, prev)
			}(a)
		}
		wg.Wait()
		sendEvent(appCtx, events, refreshDoneMsg{cycle: cycle})
	}()
	return nil
}

// streamForge consulta un forge y emite sus páginas, aplicando el corte del
// refresco incremental por cursor: si la cabecera de una lista no cambió, se
// emite un mensaje "unchanged" y no se sigue paginando.
//
// queryCtx acota la consulta (timeout, cancelación); emitCtx acota la entrega
// de eventos. Los eventos son críticos: con emitCtx (el de la app) no se
// descartan al vencer el timeout de la consulta.
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

// tickCmd programa el siguiente refresco automático. Devuelve nil si el
// refresco está deshabilitado (intervalo 0).
func (m *Model) tickCmd() tea.Cmd {
	d := m.tickInterval()
	if d <= 0 {
		return nil
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

// armTick arma el siguiente tick solo si no hay uno pendiente y el
// auto-refresco está habilitado: garantiza una única cadena de ticks.
func (m *Model) armTick() tea.Cmd {
	if m.tickPending || m.tickInterval() <= 0 {
		return nil
	}
	m.tickPending = true
	return m.tickCmd()
}

// tickInterval es el intervalo efectivo del auto-refresco, con backoff.
func (m *Model) tickInterval() time.Duration {
	base := m.cfg.RefreshInterval
	if base <= 0 {
		return 0
	}
	return base + m.backoff
}

// paused indica si el auto-refresco debe esperar: hay una acción en curso o un
// refresco activo.
//
// La paginación no se consulta aquí a propósito: mientras se pagina de verdad
// el ciclo sigue en vuelo (`loading`), así que ya está pausado. Un `more` que
// sobrevive al fin del ciclo es residuo (p. ej. la página que lo cerraba se
// perdió o el forge cortó por rate limit): mirarlo congelaría el tick para
// siempre y el inbox no volvería a refrescar.
func (m *Model) paused() bool {
	return m.loading || m.actionBusy
}

// sectionLoadingMore indica si una sección tiene páginas pendientes.
func (m *Model) sectionLoadingMore(kind model.Section) bool {
	for key, s := range m.streams {
		if key.section == kind && s.more {
			return true
		}
	}
	return false
}

// applyPage incorpora una página al stream correspondiente. La primera página
// reemplaza la lista (refresco incremental: el resto de listas conservan su
// contenido hasta que llegue su página). Un mensaje unchanged conserva lo ya
// cargado y cierra la paginación del stream.
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

	// Un ítem visto en un refresco exitoso deja de estar denegado: puede que
	// los permisos ya estén (o el usuario reintente con estado renovado).
	for i := range msg.items {
		delete(m.denied, msg.items[i].ID())
	}

	if msg.first {
		s.items = msg.items
		s.headCursor = msg.next
	} else {
		s.items = append(s.items, msg.items...)
	}
	s.cursor = msg.next
	s.more = msg.more
	// Un fallback degradado trae datos parciales: no se marca como completo,
	// para que el refresco incremental no lo congele.
	if !msg.more && !hasDegraded(msg.warnings) {
		s.complete = true
	}

	if st := m.statuses[msg.key.forge]; st != nil {
		st.updatedAt = time.Now()
		st.warnings = appendWarnings(st.warnings, forge.StampSection(msg.warnings, msg.key.section))
	}
	m.rebuild()
}

// applyItemUpdate reemplaza un ítem conocido por su versión releída; si no
// estaba, lo añade a su sección.
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

// mergeItem conserva la sección y el tipo de review del ítem original si el
// releído no los trae (ItemState no conoce la sección del inbox).
func mergeItem(old, fresh model.Item) model.Item {
	if fresh.Section == "" {
		fresh.Section = old.Section
	}
	if fresh.ReviewKind == "" {
		fresh.ReviewKind = old.ReviewKind
	}
	return fresh
}

// rebuild recompone el inbox a partir de los streams y reajusta el cursor.
func (m *Model) rebuild() {
	m.inbox = inbox.Build(m.forgeResults())
	m.clampCursor()
}

// forgeResults compone un resultado por forge de forma determinista.
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

// sortedForgeNames devuelve los nombres de forge ordenados.
func (m *Model) sortedForgeNames() []string {
	names := make([]string, 0, len(m.statuses))
	for name := range m.statuses {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// streamItems devuelve los ítems de un stream concreto.
func (m *Model) streamItems(forgeName string, section model.Section, kind model.ReviewKind) []model.Item {
	s := m.streams[streamKey{forge: forgeName, section: section, kind: kind}]
	if s == nil {
		return nil
	}
	return s.items
}

// clampCursor mantiene el cursor dentro de las filas navegables.
func (m *Model) clampCursor() {
	rows := m.rows()
	if len(rows) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = min(max(0, m.cursor), len(rows)-1)
}

// rows devuelve los ítems de todas las secciones en orden de pintado.
func (m *Model) rows() []model.Item {
	var out []model.Item
	for _, s := range m.inbox.Sections {
		out = append(out, s.Items...)
	}
	return out
}

// selected devuelve el ítem bajo el cursor, si lo hay.
func (m *Model) selected() (model.Item, bool) {
	rows := m.rows()
	if len(rows) == 0 || m.cursor >= len(rows) {
		return model.Item{}, false
	}
	return rows[m.cursor], true
}

// liveDetail deriva el ítem del detalle del estado vivo del inbox por su
// identidad, de modo que refleje refrescos y acciones sin volver a abrirlo. Si
// el ítem ya no está en el inbox, devuelve el último estado conocido.
func (m *Model) liveDetail() model.Item {
	for _, it := range m.rows() {
		if it.ID() == m.detailID {
			return it
		}
	}
	return m.detailItem
}

// sectionItems devuelve los ítems de una sección.
func (m *Model) sectionItems(kind model.Section) []model.Item {
	return m.inbox.Section(kind).Items
}

// sectionProblems devuelve los mensajes de "no se pudo consultar" o de datos
// parciales de una sección, derivados de los warnings de esa sección.
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

// problemText compone el aviso de un warning para una sección.
func problemText(forgeName string, w model.Warning) string {
	if w.Kind == "degraded" {
		return fmt.Sprintf("%s: %s", forgeName, w.Msg)
	}
	return fmt.Sprintf("%s: no se pudo consultar (%s)", forgeName, problemLabel(w.Kind))
}

// hasDegraded indica si algún warning marca datos parciales.
func hasDegraded(warns []model.Warning) bool {
	for _, w := range warns {
		if w.Kind == "degraded" {
			return true
		}
	}
	return false
}

// problemLabel traduce el tipo de warning a una etiqueta corta.
func problemLabel(kind string) string {
	switch kind {
	case "auth":
		return "sin autenticar"
	case "timeout":
		return "timeout"
	case "ratelimit":
		return "límite de peticiones"
	case "parse":
		return "respuesta ilegible"
	case "unsupported":
		return "no soportado"
	case "network":
		return "sin conexión"
	default:
		return kind
	}
}

// lastRefreshLabel resume el momento de la última actualización de una fuente.
func lastRefreshLabel(since time.Time, now time.Time) string {
	if since.IsZero() {
		return "sin datos"
	}
	d := now.Sub(since)
	switch {
	case d < time.Second:
		return "ahora"
	case d < time.Minute:
		return fmt.Sprintf("hace %ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("hace %dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("hace %dh", int(d.Hours()))
	}
}

// applySnapshot vuelca el cache en los streams (sin cursor de paginación).
func (m *Model) applySnapshot(f cache.File) {
	for _, cs := range f.Streams {
		key := streamKey{forge: cs.Forge, section: cs.Section, kind: cs.Kind}
		m.streams[key] = &stream{items: cs.Items, cursor: cs.Cursor}
		if st := m.statuses[cs.Forge]; st != nil && cs.Host != "" {
			st.host = cs.Host
		}
	}
}

// snapshot compone el documento de cache a partir de los streams.
func (m *Model) snapshot() cache.File {
	keys := make([]streamKey, 0, len(m.streams))
	for key := range m.streams {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].forge != keys[j].forge {
			return keys[i].forge < keys[j].forge
		}
		if keys[i].section != keys[j].section {
			return keys[i].section < keys[j].section
		}
		return keys[i].kind < keys[j].kind
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

// saveSnapshot persiste el snapshot sin bloquear la UI.
func (m *Model) saveSnapshot() {
	if m.cachePath == "" {
		return
	}
	f := m.snapshot()
	path := m.cachePath
	go func() { _ = cache.Save(path, f) }()
}

// recomputeBackoff ajusta el backoff del auto-refresco según los warnings del
// último ciclo (límite de peticiones o timeout).
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

// appendWarnings añade warnings sin duplicar los ya presentes (la paginación
// puede repetir el mismo aviso en cada página).
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

// setNotice fija el aviso de la cabecera.
func (m *Model) setNotice(text string, level noticeLevel) {
	m.notice = text
	m.level = level
}
