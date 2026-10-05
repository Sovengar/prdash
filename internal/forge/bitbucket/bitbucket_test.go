package bitbucket

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestConformance(t *testing.T) {
	testutil.RunConformance(t, New("bitbucket.org"), testutil.ConformanceOptions{Unsupported: true})
}

// The two OK:false are indistinguishable by the boolean, so the REASON is what decides what the
// operator does.
func TestAuthReportsNotImplementedRatherThanNotAuthenticated(t *testing.T) {
	auth := New("bitbucket.org").Auth(context.Background())

	if auth.OK {
		t.Error("Auth says Bitbucket is operational: it is not")
	}
	if auth.Forge != "bitbucket" {
		t.Errorf("Forge = %q, want bitbucket: the state is painted next to the forge name",
			auth.Forge)
	}
	if !strings.Contains(strings.ToLower(auth.Reason), "not implemented") {
		t.Errorf("Reason = %q, and it must say that it is NOT IMPLEMENTED rather than that "+
			"there is no session: they are problems with different fixes", auth.Reason)
	}
	// The reason must NOT contain the word "authenticated": that is what sends the user off to
	// configure auth instead of looking at the project.
	if strings.Contains(strings.ToLower(auth.Reason), "auth") {
		t.Errorf("Reason = %q mentions authentication: it would send the user to check the "+
			"token of something that has no fix", auth.Reason)
	}
}

// Same pattern as the other partially-implemented forges.
func TestUnsupportedOperationsWarnWithoutQueryingTheForge(t *testing.T) {
	a := New("bitbucket.org")

	for _, c := range []struct {
		name string
		q    forge.Query
	}{
		{"review listing", forge.Query{Section: model.SectionReview}},
		{"mentions listing", forge.Query{Section: model.SectionMentions}},
		{"authored listing", forge.Query{Section: model.SectionAuthored}},
	} {
		page, warns := a.List(context.Background(), c.q)
		if len(page.Items) != 0 {
			t.Errorf("%s: %d items from an unsupported operation", c.name, len(page.Items))
		}
		if len(warns) == 0 {
			t.Errorf("%s: no warning: the inbox would look empty without an explanation", c.name)
			continue
		}
		if warns[0].Kind != "unsupported" {
			t.Errorf("%s: kind %q, want unsupported", c.name, warns[0].Kind)
		}
		if warns[0].Section != c.q.Section {
			t.Errorf("%s: the warning does not carry the requested section (carries %q)", c.name, warns[0].Section)
		}
	}
}
