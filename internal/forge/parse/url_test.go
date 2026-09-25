package parse

import "testing"

func TestParseReviewURLGitHub(t *testing.T) {
	hosts := map[string]string{"github.com": "github"}
	ref, n, ok := ParseReviewURL("https://github.com/acme/widget/pull/42", hosts)
	if !ok || n != 42 {
		t.Fatalf("ok=%v n=%d", ok, n)
	}
	if ref.Forge != "github" || ref.Host != "github.com" || ref.Project != "acme/widget" || ref.Owner != "acme" || ref.Name != "widget" {
		t.Fatalf("ref = %+v", ref)
	}
}

func TestParseReviewURLGitHubWithSuffix(t *testing.T) {
	if _, n, ok := ParseReviewURL("https://github.com/acme/widget/pull/7/files#diff", nil); !ok || n != 7 {
		t.Fatalf("ok=%v n=%d", ok, n)
	}
}

func TestParseReviewURLGitHubEnterpriseHost(t *testing.T) {
	hosts := map[string]string{"github.enterprise.com": "github"}
	ref, n, ok := ParseReviewURL("https://github.enterprise.com/o/r/pull/3", hosts)
	if !ok || n != 3 || ref.Forge != "github" || ref.Host != "github.enterprise.com" {
		t.Fatalf("ref=%+v n=%d ok=%v", ref, n, ok)
	}
}

func TestParseReviewURLGitLabNestedGroup(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": "gitlab"}
	ref, n, ok := ParseReviewURL("https://gitlab.example.com/git/sub/proj/-/merge_requests/11", hosts)
	if !ok || n != 11 {
		t.Fatalf("ok=%v n=%d", ok, n)
	}
	if ref.Forge != "gitlab" || ref.Project != "git/sub/proj" || ref.Owner != "sub" || ref.Name != "proj" {
		t.Fatalf("ref = %+v", ref)
	}
}

func TestParseReviewURLInfersForgeWithoutHostMap(t *testing.T) {
	ref, _, ok := ParseReviewURL("https://gitlab.com/g/p/-/merge_requests/5", nil)
	if !ok || ref.Forge != "gitlab" {
		t.Fatalf("ref = %+v ok=%v", ref, ok)
	}
	ref, _, ok = ParseReviewURL("https://unknown.example/o/r/pull/9", nil)
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
		if _, _, ok := ParseReviewURL(raw, nil); ok {
			t.Errorf("%q no debería resolverse a un review", raw)
		}
	}
}
