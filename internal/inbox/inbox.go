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
	collected, warnings := collectBySection(inputs)
	authority := assignAuthority(collected)

	out := make([]Section, 0, len(sectionOrder))
	for _, kind := range sectionOrder {
		out = append(out, Section{Kind: kind, Items: dedupeAndSort(collected[kind], kind, authority)})
	}
	return Inbox{Sections: out, Warnings: warnings}
}

// collectBySection agrupa los ítems de cada sección y acumula los warnings.
func collectBySection(inputs []ForgeResult) (map[model.Section][]model.Item, []model.Warning) {
	collected := map[model.Section][]model.Item{}
	var warnings []model.Warning
	for _, in := range inputs {
		collected[model.SectionAuthored] = append(collected[model.SectionAuthored], in.Authored...)
		collected[model.SectionReview] = append(collected[model.SectionReview], in.Review...)
		collected[model.SectionMentions] = append(collected[model.SectionMentions], in.Mentions...)
		warnings = append(warnings, in.Warnings...)
	}
	return collected, warnings
}

// assignAuthority decide, para cada identidad, en qué sección debe aparecer.
func assignAuthority(collected map[model.Section][]model.Item) map[model.ID]model.Section {
	best := map[model.ID]model.Section{}
	for _, kind := range sectionOrder {
		for _, it := range collected[kind] {
			id := it.ID()
			if prev, ok := best[id]; !ok || rank(kind) < rank(prev) {
				best[id] = kind
			}
		}
	}
	return best
}

// dedupeAndSort conserva solo los ítems cuya autoridad cae en esta sección,
// eliminando repeticiones, y los ordena por atención.
func dedupeAndSort(items []model.Item, kind model.Section, authority map[model.ID]model.Section) []model.Item {
	seen := map[model.ID]bool{}
	out := make([]model.Item, 0, len(items))
	for _, it := range items {
		id := it.ID()
		if authority[id] != kind || seen[id] {
			continue
		}
		seen[id] = true
		it.Section = kind
		out = append(out, it)
	}
	sortItems(out)
	return out
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
