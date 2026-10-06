package github

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// One contract across all of them: what happens when the forge answers something unreadable.

func ghReturning(t *testing.T, body string) string {
	t.Helper()
	script, _ := ghThatLogs(t, body)
	return script
}

func onlyWarning(t *testing.T, warns []model.Warning, where string) model.Warning {
	t.Helper()
	if len(warns) != 1 {
		t.Fatalf("%s: %d warnings, want 1: %+v", where, len(warns), warns)
	}
	return warns[0]
}

// HTML is the case chosen, because that is what a corporate proxy returns.
func TestListWithOutputThatIsNotJSONWarnsAndDoesNotBreak(t *testing.T) {
	for _, c := range []struct {
		name   string
		output string
	}{
		{"html from a proxy", "cat <<'EOF'\n<html><body>Proxy Authentication Required</body></html>\nEOF"},
		// The truncation goes through a heredoc: with echo the script has a dangling quote and what
		//fails is the SHELL, not the parsing.
		{"truncated json", "cat <<'JSON'\n{\"data\":{\"search\":{\"nodes\":[\nJSON"},
		{"empty json", "printf ''"},
		{"array instead of object", `echo '[]'`},
		{"field that is not a list", `echo '{"data":{"search":{"nodes":"I am not a list"}}}'`},
	} {
		a := New("github.com", ghReturning(t, c.output))
		page, warns := a.List(context.Background(), forge.Query{Section: model.SectionReview})

		// No panic and no items: an empty page with a warning is "I do not know", and a page with
		// items is data.
		if len(page.Items) != 0 {
			t.Errorf("%s: returned %d items from unreadable output", c.name, len(page.Items))
		}
		w := onlyWarning(t, warns, c.name)
		if w.Kind != "parse" {
			t.Errorf("%s: warning of kind %q, want parse: unreadable data is neither a rate "+
				"limit nor a permissions problem", c.name, w.Kind)
		}
		if w.Section != model.SectionReview {
			t.Errorf("%s: the warning does not carry the section (carries %q)", c.name, w.Section)
		}
		if strings.TrimSpace(w.Msg) == "" {
			t.Errorf("%s: the warning ended up empty", c.name)
		}
	}
}

// Misclassifying here is the real damage: a parse warning must not read as "you have no work".
func TestTheParseWarningDoesNotSayThereIsNothing(t *testing.T) {
	a := New("github.com", ghReturning(t, `echo 'I am not json'`))
	_, warns := a.List(context.Background(), forge.Query{Section: model.SectionReview})
	w := onlyWarning(t, warns, "list")

	for _, kind := range []string{"notfound", "empty", "ok"} {
		if w.Kind == kind {
			t.Errorf("unreadable output was classified as %q, and that makes the TUI assert "+
				"something false about the inbox", kind)
		}
	}
	// The message has to be recognisable as a data problem: "parse" is not in the text.
	if !strings.Contains(strings.ToLower(w.Msg), "json") &&
		!strings.Contains(strings.ToLower(w.Msg), "parse") {
		t.Logf("the warning does not mention the format: %q", w.Msg)
	}
}

// ItemState is more dangerous than List, because its result is what the card paints.
func TestReadingTheStateOfAPRWithUnreadableOutputWarnsAndDoesNotReturnAFakeItem(t *testing.T) {
	a := New("github.com", ghReturning(t, `echo 'broken response'`))
	it, warns := a.ItemState(context.Background(),
		model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)

	w := onlyWarning(t, warns, "ItemState")
	if w.Kind != "parse" {
		t.Errorf("warning of kind %q, want parse", w.Kind)
	}
	if it.ID() != (model.ID{}) || it.Number != 0 || it.HeadSHA != "" || it.Title != "" {
		t.Errorf("returned a half item instead of the zero value: %+v", it)
	}
}

// The difference with the other two: an invented comment is visible text.
func TestTheConversationWithUnreadableOutputWarnsAndDoesNotPaintInventedComments(t *testing.T) {
	a := New("github.com", ghReturning(t, `echo '{{{not json'`))
	page, warns := a.Comments(context.Background(),
		model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)

	w := onlyWarning(t, warns, "Comments")
	if w.Kind != "parse" {
		t.Errorf("warning of kind %q, want parse", w.Kind)
	}
	if len(page.Comments) != 0 {
		t.Errorf("returned %d comments from unreadable output", len(page.Comments))
	}
	if page.Total != 0 {
		t.Errorf("Total = %d with unreadable output", page.Total)
	}
	// The warning carries no section: the conversation is painted on the item's card, not in a
	// column.
	if w.Section != "" {
		t.Errorf("the conversation warning carries section %q; it should only be on the card", w.Section)
	}
}

// Both degrade to the same thing and must be told apart.
func TestABinaryFailureAndUnreadableOutputAreNotConfused(t *testing.T) {
	q := forge.Query{Section: model.SectionReview}

	// The binary fails: the warning gets the class that matches the failure, NOT "parse".
	down := New("github.com", ghReturning(t, "echo 'gh: no such host' >&2\nexit 1"))
	_, warns := down.List(context.Background(), q)
	w := onlyWarning(t, warns, "binary down")
	if w.Kind == "parse" {
		t.Error("a failing binary was classified as parse: the retry would use the wrong " +
			"backoff and the warning would lie about the cause")
	}
	if strings.TrimSpace(w.Msg) == "" {
		t.Error("the warning for the down binary is empty")
	}

	// The binary works and the output is garbage: parse, for sure.
	broken := New("github.com", ghReturning(t, `echo 'I am not json'`))
	_, warns = broken.List(context.Background(), q)
	if got := onlyWarning(t, warns, "broken output").Kind; got != "parse" {
		t.Errorf("unreadable output gave kind %q, want parse", got)
	}
}
