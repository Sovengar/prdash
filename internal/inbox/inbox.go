package inbox

import (
	"cmp"
	"slices"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

type ForgeResult struct {
	Forge    string
	Host     string
	Authored []model.Item
	Review   []model.Item
	Mentions []model.Item
	Warnings []model.Warning
}

type Section struct {
	Kind  model.Section
	Items []model.Item
}

type Inbox struct {
	Sections []Section
	Warnings []model.Warning
}

var sectionOrder = []model.Section{
	model.SectionAuthored,
	model.SectionReview,
	model.SectionMentions,
}

// Each identity lands in its most authoritative section; the rest of its appearances are dropped.
// Ties on attention score break by update time, then by number.
func Build(inputs []ForgeResult) Inbox {
	collected, warnings := collectBySection(inputs)
	authority := assignAuthority(collected)

	out := make([]Section, 0, len(sectionOrder))
	for _, kind := range sectionOrder {
		out = append(out, Section{Kind: kind, Items: dedupeAndSort(collected[kind], kind, authority)})
	}
	return Inbox{Sections: out, Warnings: warnings}
}

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

// sectionOrder walks sections from best to worst authority, so the FIRST writer of an identity
// wins and the loop needs no comparison at all: the old `rank(kind) < rank(prev)` had an
// equality case no input can reach (distinct sections have distinct ranks), which made its
// boundary mutant one no test could kill.
func assignAuthority(collected map[model.Section][]model.Item) map[model.ID]model.Section {
	best := map[model.ID]model.Section{}
	for _, kind := range sectionOrder {
		for _, it := range collected[kind] {
			id := it.ID()
			if _, ok := best[id]; !ok {
				best[id] = kind
			}
		}
	}
	return best
}

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

func (in Inbox) Section(kind model.Section) Section {
	for _, s := range in.Sections {
		if s.Kind == kind {
			return s
		}
	}
	return Section{Kind: kind}
}

func (in Inbox) Empty() bool {
	for _, s := range in.Sections {
		if len(s.Items) > 0 {
			return false
		}
	}
	return true
}

// The score key has its operands swapped because the most urgent item goes first; a tie falls to the
// update time (the newest one first) and then to the number.
func sortItems(items []model.Item) {
	slices.SortStableFunc(items, func(a, b model.Item) int {
		sa, sb := state.Derive(a).Score(), state.Derive(b).Score()
		return cmp.Or(
			cmp.Compare(sb, sa),
			b.UpdatedAt.Compare(a.UpdatedAt),
			cmp.Compare(a.Number, b.Number),
		)
	})
}
