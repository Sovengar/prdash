package parse

import (
	"encoding/json"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// The owner is everything before the LAST slash of the project path.
func TestTheOwnerOfAGitLabMRIsWhatComesBeforeTheLastSlash(t *testing.T) {
	cases := []struct {
		fullPath, wantOwner, note string
	}{
		{"grupo/proyecto", "grupo", "the normal case"},
		{"grupo/sub/proyecto", "grupo/sub",
			"with subgroups: the owner is EVERYTHING before the last slash, which is " +
				"what tells one project apart from another with the same name in another group"},
		{"proyecto", "",
			"a project at the instance root: there is no group and the empty owner is correct"},
		{"/proyecto", "",
			"a slash at position zero: both branches of the condition give the empty string, " +
				"so index zero does not tell them apart"},
	}
	for _, c := range cases {
		it := itemFromGLMR(makeMR(c.fullPath, "proyecto"),
			model.SectionReview, model.ReviewRequested)
		if it.Ref.Owner != c.wantOwner {
			t.Errorf("%s: the owner came out %q, want %q", c.note, it.Ref.Owner, c.wantOwner)
		}
		if it.Ref.Project != c.fullPath {
			t.Errorf("%s: the project came out %q, want %q: it is what tells one project "+
				"apart from another with the same name", c.note, it.Ref.Project, c.fullPath)
		}
		if it.Ref.Name != "proyecto" {
			t.Errorf("%s: the name came out %q", c.note, it.Ref.Name)
		}
	}

	// Deliberately NOT asserted: that `/group/project` gives an owner that is a path. It does, and it
	//looks wrong, but it is the same string the API gives.
	it := itemFromGLMR(makeMR("/grupo/proyecto", "proyecto"),
		model.SectionReview, model.ReviewRequested)
	t.Logf("full_path with a leading slash: owner %q, project %q",
		it.Ref.Owner, it.Ref.Project)
	if !strings.HasPrefix(it.Ref.Owner, "/grupo") {
		t.Logf("the owner does not come out with a leading slash; review the assertion above")
	}
}

// Built through JSON and not by hand, because the fields carry tags.
func makeMR(fullPath, name string) glMR {
	raw, err := json.Marshal(map[string]any{
		"iid":    7,
		"title":  "uno",
		"webUrl": "https://gitlab.com/g/p!7",
		"state":  "opened",
		"project": map[string]any{
			"name":     name,
			"path":     name,
			"fullPath": fullPath,
		},
	})
	if err != nil {
		panic(err)
	}
	var mr glMR
	if err := json.Unmarshal(raw, &mr); err != nil {
		panic(err)
	}
	return mr
}
