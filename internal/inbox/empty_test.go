package inbox

import (
	"testing"

	"prdash/internal/forge/model"
)

// The two functions the TUI asks what to paint, and both have a "nothing here" answer that is
//not a value but a state.

// The case to watch is the UNKNOWN section's class.
func TestAskingForASectionThatIsNotThereReturnsItEmptyButWithItsClass(t *testing.T) {
	in := Inbox{Sections: []Section{
		{Kind: model.SectionAuthored},
		{Kind: model.SectionReview},
	}}

	review := in.Section(model.SectionReview)
	if review.Kind != model.SectionReview {
		t.Errorf("Kind = %q, want %q", review.Kind, model.SectionReview)
	}
	if len(review.Items) != 0 {
		t.Errorf("the review section brings %d items, and the fixture puts none", len(review.Items))
	}

	// An absent class: empty BUT with its name, which is the case the "sections" table needs.
	absent := in.Section("a-made-up-section")
	if absent.Kind != "a-made-up-section" {
		t.Errorf("the absent section came back with Kind %q: the column name would be lost", absent.Kind)
	}
	if len(absent.Items) != 0 {
		t.Errorf("the absent section brings %d items", len(absent.Items))
	}

	// The returned section is not attached to the inbox: changing it does not change the inbox.
	absent.Items = append(absent.Items, model.Item{Number: 99})
	if len(in.Section("a-made-up-section").Items) != 0 {
		t.Error("modifying the returned section changed the inbox")
	}

	// And with an inbox with no sections: the same answer.
	empty := Inbox{}
	if s := empty.Section(model.SectionReview); s.Kind != model.SectionReview || len(s.Items) != 0 {
		t.Errorf("an inbox with no sections returned %+v", s)
	}
}

// The walk is over SECTIONS and not over items.
func TestEmptyIsFalseAsSoonAsThereIsAnItemInAnySection(t *testing.T) {
	for _, c := range []struct {
		name      string
		in        Inbox
		wantEmpty bool
	}{
		{"a freshly built inbox", Inbox{}, true},
		{"sections without items", Inbox{Sections: []Section{
			{Kind: model.SectionReview}, {Kind: model.SectionMentions},
		}}, true},
		{"an item in the first section", Inbox{Sections: []Section{
			{Kind: model.SectionReview, Items: []model.Item{{Number: 1}}},
			{Kind: model.SectionMentions},
		}}, false},
		{"an item ONLY in the last section", Inbox{Sections: []Section{
			{Kind: model.SectionReview},
			{Kind: model.SectionAuthored},
			{Kind: model.SectionMentions, Items: []model.Item{{Number: 7}}},
		}}, false},
		{"a nil entry", Inbox{Sections: []Section{
			{Kind: model.SectionReview, Items: []model.Item{}},
		}}, true},
	} {
		if got := c.in.Empty(); got != c.wantEmpty {
			t.Errorf("%s: Empty() = %v, want %v", c.name, got, c.wantEmpty)
		}
	}
}

// Not a make-up check: they are the two functions the TUI uses to decide what to paint.
func TestEmptyAndCountHaveToAgree(t *testing.T) {
	in := Inbox{Sections: []Section{
		{Kind: model.SectionReview, Items: []model.Item{{Number: 1}, {Number: 2}}},
		{Kind: model.SectionAuthored},
	}}

	total := 0
	for _, s := range in.Sections {
		total += len(s.Items)
	}
	if total != 2 {
		t.Fatalf("the fixture has %d items, not 2", total)
	}
	if in.Empty() {
		t.Error("an inbox with two items said it was empty")
	}
	if len(in.Section(model.SectionReview).Items) != total {
		t.Errorf("the visible section has %d items and the total is %d",
			len(in.Section(model.SectionReview).Items), total)
	}
}
