package inbox

import (
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// The two survivors are the order and the authority, and both are comparisons whose boundary
//is a tie.

// The order has three tie-breaks and the last one is the number.
func TestTheOrderIsAttentionDateAndNumber(t *testing.T) {
	date := time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)

	item := func(n int, score string, when time.Time) model.Item {
		it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com",
			Project: "acme/widget", Owner: "acme", Name: "widget"}, n)
		it.Title, it.UpdatedAt, it.ReviewDecision = "pr", when, score
		return it
	}
	numbers := func(items []model.Item) []int {
		out := make([]int, len(items))
		for i, it := range items {
			out[i] = it.Number
		}
		return out
	}
	equal := func(got, want []int) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	// The THIRD tie-break, the one you can see without thinking.
	items := []model.Item{
		item(4, "approved", date), item(2, "approved", date),
		item(3, "approved", date), item(1, "approved", date),
	}
	sortItems(items)
	if got := numbers(items); !equal(got, []int{1, 2, 3, 4}) {
		t.Errorf("with equal attention and date it gave %v, want [1 2 3 4]. The third "+
			"tie-break is the number: without it the order would be the arrival order, which "+
			"depends on which forge answered first and changes between runs", got)
	}

	items = []model.Item{
		item(1, "approved", date), item(3, "approved", date),
		item(2, "approved", date), item(4, "approved", date),
	}
	sortItems(items)
	if got := numbers(items); !equal(got, []int{1, 2, 3, 4}) {
		t.Errorf("with equal attention and date but another input order it gave %v, "+
			"want [1 2 3 4]", got)
	}

	items = []model.Item{
		item(1, "approved", date), item(2, "approved", date.Add(time.Hour)),
		item(3, "approved", date.Add(48*time.Hour)),
	}
	sortItems(items)
	if got := numbers(items); !equal(got, []int{3, 2, 1}) {
		t.Errorf("with equal attention and different dates it gave %v, want [3 2 1]: the most "+
			"recent update wins", got)
	}

	items = []model.Item{
		item(9, "approved", date), item(1, "approved", date.Add(time.Hour)),
	}
	sortItems(items)
	if got := numbers(items); !equal(got, []int{1, 9}) {
		t.Errorf("with the number backwards and the date backwards it gave %v, want [1 9]", got)
	}

	// The FIRST tie-break, the one that kills a `>=`: attention beats everything else.
	items = []model.Item{
		item(1, "approved", date),
		item(2, "approved", date.Add(time.Hour)),
		item(9, "changes_requested", date.Add(-72*time.Hour)),
		item(3, "approved", date.Add(48*time.Hour)),
	}
	sortItems(items)
	if items[0].Number != 9 {
		t.Errorf("with changes requested the first is %d, want the 9. Attention beats the date "+
			"and the number: it is the only one that needs an action",
			items[0].Number)
	}

	brokenChecks := item(1, "approved", date)
	brokenChecks.State = "OPEN"
	brokenChecks.Checks.State = model.ChecksFailing

	items = []model.Item{
		item(5, "", date),                  // pending
		item(4, "approved", date),          // approved
		item(3, "review_required", date),   // review pending
		item(2, "changes_requested", date), // changes requested
		brokenChecks,                       // broken checks
	}
	sortItems(items)
	if got := numbers(items); !equal(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("the whole state order gave %v, want [1 2 3 4 5]", got)
	}
}

// An item in several sections stays in the most authoritative one.
func TestTheSectionWithTheMostAuthorityKeepsTheItem(t *testing.T) {
	mk := func(n int) model.Item {
		it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com",
			Project: "acme/widget", Owner: "acme", Name: "widget"}, n)
		it.Title = "pr"
		return it
	}

	// All THREE declared sections: it stays in the first. Repeated twenty times because Go's map
	//order is not guaranteed.
	for i := range 20 {
		best := assignAuthority(map[model.Section][]model.Item{
			model.SectionMentions: {mk(1)},
			model.SectionAuthored: {mk(1)},
			model.SectionReview:   {mk(1)},
		})
		if got := best[mk(1).ID()]; got != model.SectionAuthored {
			t.Fatalf("pass %d: an item in all three sections ended in %q, want %q. Authority "+
				"is the declared order, not the map traversal",
				i, got, model.SectionAuthored)
		}
	}

	for _, s := range []model.Section{
		model.SectionAuthored, model.SectionReview, model.SectionMentions,
	} {
		best := assignAuthority(map[model.Section][]model.Item{s: {mk(2)}})
		if got := best[mk(2).ID()]; got != s {
			t.Errorf("an item only in %q ended in %q", s, got)
		}
	}

	// A section prdash does NOT declare is not consulted at all: the item does not enter.
	undeclared := model.Section("undeclared")
	best := assignAuthority(map[model.Section][]model.Item{undeclared: {mk(3)}})
	if got, ok := best[mk(3).ID()]; ok {
		t.Errorf("an undeclared section gave authority %q: `assignAuthority` walks the "+
			"declared sections, and if it looked at the map keys the authority would depend "+
			"on what each adapter brings", got)
	}

	// The same item in a declared and an undeclared one: the declared wins.
	best = assignAuthority(map[model.Section][]model.Item{
		undeclared:            {mk(4)},
		model.SectionMentions: {mk(4)},
	})
	if got := best[mk(4).ID()]; got != model.SectionMentions {
		t.Errorf("with one declared and one not it ended in %q, want %q", got, model.SectionMentions)
	}
}
