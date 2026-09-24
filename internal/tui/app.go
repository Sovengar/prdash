// Package tui implementa el inbox cross-forge en Bubbletea v2: tres secciones
// (creados por mí / review asignados / menciones) con datos ricos de cada
// forge y una bomba de eventos que reparte los resultados en segundo plano.
//
// Es de solo lectura: consulta los forges y pinta el estado; las acciones
// (detalle, approve, merge, montar review) llegan en etapas posteriores.
package tui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

// event es el mensaje unificado del canal de trabajo en segundo plano.
type event interface{}

// forgeResultMsg entrega el resultado de consultar un forge.
type forgeResultMsg struct{ result inbox.ForgeResult }

// refreshDoneMsg marca el fin de un ciclo de refresco.
type refreshDoneMsg struct{}

// refreshTimeout es el límite de un ciclo completo de consulta a los forges.
const refreshTimeout = 60 * time.Second

// forgeStatus es el estado de consulta de un forge, para el indicador de
// "última actualización" y la degradación explícita por forge.
type forgeStatus struct {
	Forge     string
	Host      string
	Auth      model.AuthState
	Warnings  []model.Warning
	UpdatedAt time.Time
}

// Model es el modelo raíz de la TUI.
type Model struct {
	cfg      config.Config
	adapters []forge.Adapter

	results  []inbox.ForgeResult
	inbox    inbox.Inbox
	statuses []forgeStatus

	cursor      int
	width       int
	height      int
	loading     bool
	lastRefresh time.Time

	events  chan event
	ctx     context.Context
	cancel  context.CancelFunc
	spinner spinner.Model
}

// New construye el modelo con la config y los adapters habilitados. Arranca en
// estado "cargando": Init lanza el primer refresco.
func New(cfg config.Config, adapters []forge.Adapter) Model {
	ctx, cancel := context.WithCancel(context.Background())

	statuses := make([]forgeStatus, 0, len(adapters))
	for _, a := range adapters {
		statuses = append(statuses, forgeStatus{
			Forge: a.Forge(),
			Host:  a.Host(),
			Auth:  model.AuthState{Forge: a.Forge(), OK: true},
		})
	}

	m := Model{
		cfg:      cfg,
		adapters: adapters,
		statuses: statuses,
		events:   make(chan event, 128),
		ctx:      ctx,
		cancel:   cancel,
		loading:  true,
	}
	m.spinner = spinner.New(spinner.WithSpinner(spinner.Dot))
	return m
}

// Init lanza el primer refresco y arma la bomba de eventos.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.startRefreshCmd(),
		waitForEvent(m.events),
		m.spinner.Tick,
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

// sendEvent publica en el canal respetando la cancelación.
func sendEvent(ctx context.Context, ch chan<- event, ev event) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}

// startRefreshCmd consulta todos los forges en paralelo y emite un resultado
// por forge según van llegando.
func (m *Model) startRefreshCmd() tea.Cmd {
	m.loading = true
	appCtx := m.ctx
	events := m.events
	adapters := append([]forge.Adapter(nil), m.adapters...)

	go func() {
		ctx, cancel := context.WithTimeout(appCtx, refreshTimeout)
		defer cancel()

		var wg sync.WaitGroup
		for _, a := range adapters {
			wg.Add(1)
			go func(a forge.Adapter) {
				defer wg.Done()
				sendEvent(ctx, events, forgeResultMsg{result: forge.Collect(ctx, a)})
			}(a)
		}
		wg.Wait()
		sendEvent(appCtx, events, refreshDoneMsg{})
	}()
	return nil
}

// rebuild reconsolida el inbox a partir de los resultados por forge y
// recomputa el estado de cada forge.
func (m *Model) rebuild() {
	m.inbox = inbox.Build(m.results)
	m.clampCursor()
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

// sectionItems devuelve los ítems de una sección.
func (m *Model) sectionItems(kind model.Section) []model.Item {
	return m.inbox.Section(kind).Items
}

// sectionProblems devuelve los mensajes de "no se pudo consultar" de una
// sección, derivados de los warnings de esa sección en cualquier forge.
func (m *Model) sectionProblems(kind model.Section) []string {
	var out []string
	for _, s := range m.statuses {
		for _, w := range s.Warnings {
			if w.Section != kind {
				continue
			}
			out = append(out, fmt.Sprintf("%s: no se pudo consultar (%s)", s.Forge, problemLabel(w.Kind)))
		}
	}
	return out
}

// problemLabel traduce el tipo de warning a una etiqueta corta.
func problemLabel(kind string) string {
	switch kind {
	case "auth":
		return "sin autenticar"
	case "timeout":
		return "timeout"
	case "parse":
		return "respuesta ilegible"
	case "network":
		return "sin conexión"
	default:
		return kind
	}
}

// lastRefreshLabel resume el momento del último refresco.
func (m *Model) lastRefreshLabel(now time.Time) string {
	if m.lastRefresh.IsZero() {
		return "sin datos"
	}
	d := now.Sub(m.lastRefresh)
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
