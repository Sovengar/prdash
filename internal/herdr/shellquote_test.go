package herdr

import (
	"strings"
	"testing"
)

// The safe list is short on purpose.
func TestUnsafeThingsGetQuotedAndSafeOnesDoNot(t *testing.T) {
	safe := []string{
		"abc", "ABC", "a1", "0",
		"z", "Z", "9", "azAZ09",
		"pr-123", "feature_x", "main-2", "v1.2.3",
		"a-b_c.d", "-", "_", ".",
	}
	for _, s := range safe {
		if !shellSafe(s) {
			t.Errorf("shellSafe(%q) gave false: a name without weird signs needs no quotes", s)
		}
		if got := shellQuote(s); got != s {
			t.Errorf("shellQuote(%q) gave %q, want the text as it is", s, got)
		}
	}

	for _, s := range insetList() {
		if shellSafe(s) {
			t.Errorf("shellSafe(%q) gave true: a shell would interpret that text", s)
		}
		got := shellQuote(s)
		if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
			t.Errorf("shellQuote(%q) gave %q, and it has to go between single quotes", s, got)
		}
	}
}

func insetList() []string {
	return []string{
		"with space",
		"semi;colon",
		"$(command)",
		"`command`",
		"a|b", "a&b", "a>b", "a<b", "a;b",
		"a\nb", "a\tb",
		"quote'single",
		"dollar$",
		"tilde~",
		"asterisk*",
		"question?",
		"bracket[]",
		"brace{}",
		"parentheses()",
		"double\"quote",
		`\`,
		"#comment",
		"#123",
	}
}

// The case that cannot be treated like the others.
func TestEmptyStringGetsQuotedAndDoesNotDisappear(t *testing.T) {
	got := shellQuote("")
	if got != "''" {
		t.Errorf("shellQuote of the empty one gave %q, want %q: without quotes the argument "+
			"disappears from the command and the pane starts in the user's HOME", got, "''")
	}
	// shellSafe("") IS true —the loop sees no character and nothing dangerous— and that does not
	//let it out unquoted.
	if !shellSafe("") {
		t.Error("shellSafe of the empty one gave false: the loop sees no characters and there is " +
			"nothing to flag. What protects it is shellQuote's guard, not this one")
	}
}

// The classic `'\”` escape: closing the quote, escaping, reopening.
func TestSingleQuoteIsEscapedAndTheRestIsNot(t *testing.T) {
	got := shellQuote("it's")
	want := `'it'\''s'`
	if got != want {
		t.Errorf("shellQuote gave %q, want %q", got, want)
	}
	if resolved := resolveQuoted(got); resolved != "it's" {
		t.Errorf("the shell would read %q, want %q", resolved, "it's")
	}

	multiple := shellQuote("'a'b'c'")
	if r := resolveQuoted(multiple); r != "'a'b'c'" {
		t.Errorf("several quotes: the shell would read %q, want %q", r, "'a'b'c'")
	}

	if r := resolveQuoted(shellQuote("'")); r != "'" {
		t.Errorf("a lone quote gave %q", r)
	}
	// A string that already comes in single quotes is quoted whole and its inner quotes are left
	// alone.
	alreadyQuoted := "'prefix'"
	if r := resolveQuoted(shellQuote(alreadyQuoted)); r != alreadyQuoted {
		t.Errorf("an already quoted label gave %q, want %q", r, alreadyQuoted)
	}
}

// The smallest shell that resolves single quotes with that escape, which is all it takes to check it.
func resolveQuoted(quoted string) string {
	if len(quoted) < 2 || !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
		return quoted
	}
	inner := quoted[1 : len(quoted)-1]
	// Inside single quotes every quote has to be part of a `'\''`, so a plain replacement breaks.
	return strings.ReplaceAll(inner, `'\''`, "'")
}

// The mix is the real case: a label with spaces, quotes and slashes.
func TestArgumentWithEverythingDangerousTogetherDoesNotBreak(t *testing.T) {
	cases := []string{
		`fix "the thing"; rm -rf /`,
		"branch with 'quote' and space",
		`$(whoami)`,
		"new\nline",
		"tab\tinside",
		"accents-and-ñ",
		"%s %d %v\n",
		"*",
		`a'b'c"d`,
		"\\\\",
		"  with spaces around  ",
	}
	for _, original := range cases {
		quoted := shellQuote(original)
		if resolved := resolveQuoted(quoted); resolved != original {
			t.Errorf("the shell would read %q from %q, and it had to read %q", resolved, quoted, original)
		}
		// The mechanical property: a quoted string has an EVEN number of single quotes.
		if n := strings.Count(quoted, "'"); n%2 != 0 {
			t.Errorf("%q was quoted as %q, with an odd number of quotes", original, quoted)
		}
	}
}

// A PROPERTY assertion over the whole domain, not a table of cases.
func TestTextThatIsNotSafeNeverGoesOutUnquoted(t *testing.T) {
	for r := rune(32); r < rune(127); r++ {
		s := string(r)
		unquoted := shellQuote(s) == s
		if unquoted != shellSafe(s) {
			t.Errorf("the rune %q comes out %s and shellSafe flags it as %s: the two "+
				"have to match",
				r,
				map[bool]string{true: "unquoted", false: "quoted"}[unquoted],
				map[bool]string{true: "safe", false: "unsafe"}[shellSafe(s)])
		}
	}
	// Outside ASCII it is unknown: the sweep only covers printable ASCII, and writing that down
	// avoids guessing.
	for _, s := range []string{"ñ", "日", "🙂"} {
		if shellSafe(s) {
			t.Errorf("%q comes out unquoted; the non-ASCII are not in the safe list", s)
		}
	}
}
