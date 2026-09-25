package parse

import "testing"

func TestParseReviewURLGitHub(t *testing.T) {
	hosts := map[string]string{"github.com": "github"}
	ref, n, ok := ParseReviewURL("https://github.com/acme/widget/pull/42", hosts, nil)
	if !ok || n != 42 {
		t.Fatalf("ok=%v n=%d", ok, n)
	}
	if ref.Forge != "github" || ref.Host != "github.com" || ref.Project != "acme/widget" || ref.Owner != "acme" || ref.Name != "widget" {
		t.Fatalf("ref = %+v", ref)
	}
}

func TestParseReviewURLGitHubWithSuffix(t *testing.T) {
	if _, n, ok := ParseReviewURL("https://github.com/acme/widget/pull/7/files#diff", nil, nil); !ok || n != 7 {
		t.Fatalf("ok=%v n=%d", ok, n)
	}
}

func TestParseReviewURLGitHubEnterpriseHost(t *testing.T) {
	hosts := map[string]string{"github.enterprise.com": "github"}
	ref, n, ok := ParseReviewURL("https://github.enterprise.com/o/r/pull/3", hosts, nil)
	if !ok || n != 3 || ref.Forge != "github" || ref.Host != "github.enterprise.com" {
		t.Fatalf("ref=%+v n=%d ok=%v", ref, n, ok)
	}
}

// TestParseReviewURLGitLabNestedGroup codifica que un host con relative URL
// root se normaliza al MISMO Project que la vía API (sin el prefijo "git").
func TestParseReviewURLGitLabNestedGroup(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": "gitlab"}
	prefixes := map[string]string{"gitlab.example.com": "git"}
	ref, n, ok := ParseReviewURL("https://gitlab.example.com/git/sub/proj/-/merge_requests/11", hosts, prefixes)
	if !ok || n != 11 {
		t.Fatalf("ok=%v n=%d", ok, n)
	}
	if ref.Forge != "gitlab" || ref.Project != "sub/proj" || ref.Owner != "sub" || ref.Name != "proj" {
		t.Fatalf("ref = %+v", ref)
	}
}

// TestParseReviewURLStripsClonePrefix cubre el HIGH: con prefijo y sin él, la
// identidad siempre es la del proyecto sin el relative URL root.
func TestParseReviewURLStripsClonePrefix(t *testing.T) {
	hosts := map[string]string{
		"gitlab.example.com":    "gitlab",
		"github.enterprise.com": "github",
		"gitlab.multi.example":  "gitlab",
	}
	cases := []struct {
		name     string
		raw      string
		prefixes map[string]string
		want     string
	}{
		{
			"gitlab subcarpeta",
			"https://gitlab.example.com/git/sub/proj/-/merge_requests/5",
			map[string]string{"gitlab.example.com": "git"},
			"sub/proj",
		},
		{
			"gitlab raíz",
			"https://gitlab.example.com/sub/proj/-/merge_requests/5",
			map[string]string{"gitlab.example.com": "git"},
			"sub/proj",
		},
		{
			"gitlab sin mapa de prefijos",
			"https://gitlab.example.com/sub/proj/-/merge_requests/5",
			nil,
			"sub/proj",
		},
		{
			"gitlab prefijo multisegmento",
			"https://gitlab.multi.example/git/repos/sub/proj/-/merge_requests/5",
			map[string]string{"gitlab.multi.example": "git/repos"},
			"sub/proj",
		},
		{
			"github enterprise con prefijo",
			"https://github.enterprise.com/ent/acme/widget/pull/3",
			map[string]string{"github.enterprise.com": "ent"},
			"acme/widget",
		},
		{
			"github sin prefijo",
			"https://github.enterprise.com/acme/widget/pull/3",
			map[string]string{"github.enterprise.com": "ent"},
			"acme/widget",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, _, ok := ParseReviewURL(tc.raw, hosts, tc.prefixes)
			if !ok {
				t.Fatalf("no resolvió %q", tc.raw)
			}
			if ref.Project != tc.want {
				t.Fatalf("Project = %q, quiero %q", ref.Project, tc.want)
			}
		})
	}
}

// TestParseReviewURLDoesNotStripLookalike: un proyecto cuyo primer segmento se
// parece al prefijo pero no coincide no se recorta (sin falsos positivos).
func TestParseReviewURLDoesNotStripLookalike(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": "gitlab"}
	ref, _, ok := ParseReviewURL("https://gitlab.example.com/gitrepo/sub/proj/-/merge_requests/5", hosts,
		map[string]string{"gitlab.example.com": "git"})
	if !ok || ref.Project != "gitrepo/sub/proj" {
		t.Fatalf("ref = %+v ok=%v", ref, ok)
	}
}

// TestParseReviewURLRejectsWhenPrefixEatsProject: si el prefijo consume todo el
// path no hay proyecto y la URL no es un review válido.
func TestParseReviewURLRejectsWhenPrefixEatsProject(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": "gitlab"}
	if _, _, ok := ParseReviewURL("https://gitlab.example.com/git/sub/-/merge_requests/5", hosts,
		map[string]string{"gitlab.example.com": "git/sub"}); ok {
		t.Fatal("sin proyecto restante no debería resolverse")
	}
}

func TestParseReviewURLInfersForgeWithoutHostMap(t *testing.T) {
	ref, _, ok := ParseReviewURL("https://gitlab.com/g/p/-/merge_requests/5", nil, nil)
	if !ok || ref.Forge != "gitlab" {
		t.Fatalf("ref = %+v ok=%v", ref, ok)
	}
	ref, _, ok = ParseReviewURL("https://unknown.example/o/r/pull/9", nil, nil)
	if !ok || ref.Forge != "github" {
		t.Fatalf("ref = %+v ok=%v", ref, ok)
	}
}

func TestParseReviewURLRejectsNonReview(t *testing.T) {
	for _, raw := range []string{
		"",
		"https://github.com/acme/widget",
		"https://github.com/acme/widget/issues/1",
		"https://github.com/acme/widget/pull/abc",
		"not a url",
	} {
		if _, _, ok := ParseReviewURL(raw, nil, nil); ok {
			t.Errorf("%q no debería resolverse a un review", raw)
		}
	}
}
