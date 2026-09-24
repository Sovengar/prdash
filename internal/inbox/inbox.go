// Package inbox consolida los resultados de todos los forges en las tres
// secciones del inbox, deduplica por identidad y ordena por atención.
//
// Es puro: no toca red, subproceso, TOML ni disco. Recibe ya los ítems de
// cada forge y decide qué se muestra, dónde y en qué orden.
package inbox

import (
	"sort"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

// ForgeResult es lo que devuelve consultar un forge: los ítems por sección
// más los warnings de las secciones que no se pudieron leer.
type ForgeResult struct {
	Forge    string
	Host     string
	Authored []model.Item
	Review   []model.Item
	Mentions []model.Item
	Warnings []model.Warning
}

// Section agrupa los ítems de una sección en el orden en que se pintan.
type Section struct {
	Kind  model.Section
	Items []model.Item
}

// Inbox es el resultado consolidado y deduplicado del inbox cross-forge.
type Inbox struct {
	Sections []Section
	Warnings []model.Warning
}

// sectionOrder fija el orden de pintado y la autoridad entre secciones:
// authored > review > mentions.
var sectionOrder = []model.Section{
	model.SectionAuthored,
	model.SectionReview,
	model.SectionMentions,
}

// Build consolida los resultados de todos los forges. Deduplica por identidad
// (forge, host, proyecto y número) y asigna cada ítem a la sección de mayor
// autoridad; el resto de sus apariciones se descarta. Cada sección sale
// ordenada por score de atención y, a igual score, por actualización y número.
func Build(inputs []ForgeResult) Inbox {
	collected := map[model.Section][]model.Item{}
	var warnings []model.Warning

	for _, in := range inputs {
		collected[model.SectionAuthored] = append(collected[model.SectionAuthored], in.Authored...)
		collected[model.SectionReview] = append(collected[model.SectionReview], in.Review...)
		collected[model.SectionMentions] = append(collected[model.SectionMentions], in.Mentions...)
		warnings = append(warnings, in.Warnings...)
	}

	// Autoridad: la sección de mayor prioridad gana para cada identidad.
	best := map[model.ID]model.Section{}
	for _, kind := range sectionOrder {
		for _, it := range collected[kind] {
			id := it.ID()
			prev, ok := best[id]
			if !ok || rank(kind) < rank(prev) {
				best[id] = kind
			}
		}
	}

	out := make([]Section, 0, len(sectionOrder))
	for _, kind := range sectionOrder {
		seen := map[model.ID]bool{}
		items := make([]model.Item, 0, len(collected[kind]))
		for _, it := range collected[kind] {
			id := it.ID()
			if best[id] != kind || seen[id] {
				continue
			}
			seen[id] = true
			it.Section = kind
			items = append(items, it)
		}
		sortItems(items)
		out = append(out, Section{Kind: kind, Items: items})
	}

	return Inbox{Sections: out, Warnings: warnings}
}

// Section devuelve la sección pedida; si no existe, una vacía.
func (in Inbox) Section(kind model.Section) Section {
	for _, s := range in.Sections {
		if s.Kind == kind {
			return s
		}
	}
	return Section{Kind: kind}
}

// Empty indica si el inbox no tiene ningún ítem.
func (in Inbox) Empty() bool {
	for _, s := range in.Sections {
		if len(s.Items) > 0 {
			return false
		}
	}
	return true
}

func rank(kind model.Section) int {
	for i, k := range sectionOrder {
		if k == kind {
			return i
		}
	}
	return len(sectionOrder)
}

func sortItems(items []model.Item) {
	sort.SliceStable(items, func(i, j int) bool {
		si, sj := state.Derive(items[i]).Score(), state.Derive(items[j]).Score()
		if si != sj {
			return si > sj
		}
		if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		return items[i].Number < items[j].Number
	})
}
