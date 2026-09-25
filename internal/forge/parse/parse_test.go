package parse

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

const ghSearchFixture = `{
  "data": {
    "search": {
      "pageInfo": {"hasNextPage": true, "endCursor": "CURSOR1"},
      "nodes": [
        {
          "number": 42,
          "title": "Add widget",
          "url": "https://github.com/acme/widget/pull/42",
          "state": "OPEN",
          "isDraft": false,
          "reviewDecision": "CHANGES_REQUESTED",
          "updatedAt": "2026-09-24T10:00:00Z",
          "headRefName": "feat/widget",
          "baseRefName": "main",
          "author": {"login": "me"},
          "repository": {"nameWithOwner": "acme/widget", "name": "widget", "owner": {"login": "acme"}},
          "commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "FAILURE", "contexts": {"nodes": [
            {"__typename": "CheckRun", "status": "COMPLETED", "conclusion": "SUCCESS", "state": "SUCCESS"},
            {"__typename": "CheckRun", "status": "COMPLETED", "conclusion": "FAILURE", "state": "FAILURE"}
          ]}}}}]}
        },
        {
          "number": 43,
          "title": "Bump deps",
          "url": "https://github.com/acme/lib/pull/43",
          "state": "OPEN",
          "isDraft": true,
          "reviewDecision": "",
          "updatedAt": "2026-09-22T10:00:00Z",
          "headRefName": "chore/deps",
          "baseRefName": "main",
          "author": {"login": "someone"},
          "repository": {"nameWithOwner": "acme/lib", "name": "lib", "owner": {"login": "acme"}},
          "commits": {"nodes": []}
        }
      ]
    }
  }
}`

func TestParseGHGraphQLSearch(t *testing.T) {
	items, page, err := ParseGHGraphQLSearch(ghSearchFixture)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if !page.More || page.Next != "CURSOR1" {
		t.Errorf("pageInfo = %+v", page)
	}

	first := items[0]
	if first.Number != 42 || first.Ref.Project != "acme/widget" {
		t.Errorf("identidad = %s#%d", first.Ref.Project, first.Number)
	}
	if first.Ref.Owner != "acme" || first.Ref.Name != "widget" {
		t.Errorf("owner/name = %s/%s", first.Ref.Owner, first.Ref.Name)
	}
	if first.Forge != "github" {
		t.Errorf("forge = %q, want github", first.Forge)
	}
	if first.ReviewDecision != "CHANGES_REQUESTED" {
		t.Errorf("reviewDecision = %q", first.ReviewDecision)
	}
	if first.SourceBranch != "feat/widget" || first.TargetBranch != "main" {
		t.Errorf("branches = %s -> %s", first.SourceBranch, first.TargetBranch)
	}
	if first.Checks.State != model.ChecksFailing || first.Checks.Total != 2 || first.Checks.Failing != 1 {
		t.Errorf("checks = %+v", first.Checks)
	}
	if !first.UpdatedAt.Equal(time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("updatedAt = %v", first.UpdatedAt)
	}

	if items[1].Number != 43 || items[1].Ref.Project != "acme/lib" || items[1].State != "OPEN" {
		t.Errorf("segundo ítem inesperado: %+v", items[1])
	}
	if items[1].Checks.State != model.ChecksUnknown {
		t.Errorf("checks del segundo ítem = %+v", items[1].Checks)
	}
}

func TestParseGHGraphQLSearchSinglePR(t *testing.T) {
	raw := `{"data":{"repository":{"pullRequest":{
		"number": 9, "title": "Solo", "url": "u", "state": "OPEN",
		"headRefName": "a", "baseRefName": "b", "author": {"login": "me"},
		"repository": {"nameWithOwner": "o/r", "name": "r", "owner": {"login": "o"}}
	}}}}`
	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(items) != 1 || items[0].Number != 9 {
		t.Fatalf("items = %+v", items)
	}
}

func TestParseGHGraphQLSearchErrors(t *testing.T) {
	_, _, err := ParseGHGraphQLSearch(`{"errors":[{"message":"boom"}]}`)
	if err == nil {
		t.Fatal("se esperaba error de GraphQL")
	}
	assertParseError(t, err)
}

func TestParseGHGraphQLSearchMalformed(t *testing.T) {
	_, _, err := ParseGHGraphQLSearch("not json")
	if err == nil {
		t.Fatal("se esperaba error de parseo")
	}
	assertParseError(t, err)
}

const ghAuthoredFixture = `{
  "total_count": 1,
  "items": [
    {
      "number": 7,
      "title": "Fix crash",
      "html_url": "https://github.com/acme/lib/pull/7",
      "state": "open",
      "updated_at": "2026-09-20T08:00:00Z",
      "user": {"login": "me"},
      "repository_url": "https://api.github.com/repos/acme/lib",
      "pull_request": {"head": {"ref": "fix/x"}, "base": {"ref": "main"}}
    }
  ]
}`

func TestParseGHAuthored(t *testing.T) {
	items, err := ParseGHAuthored(ghAuthoredFixture)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	it := items[0]
	if it.Number != 7 || it.Ref.Project != "acme/lib" || it.Ref.Owner != "acme" || it.Ref.Name != "lib" {
		t.Errorf("ref = %+v", it.Ref)
	}
	if it.Author != "me" || it.SourceBranch != "fix/x" || it.TargetBranch != "main" {
		t.Errorf("item = %+v", it)
	}
	if !it.UpdatedAt.Equal(time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("updatedAt = %v", it.UpdatedAt)
	}
}

func TestParseGHChecks(t *testing.T) {
	raw := `[
		{"name":"ci","state":"SUCCESS","bucket":"pass"},
		{"name":"lint","state":"FAILURE","bucket":"fail"},
		{"name":"e2e","state":"PENDING","bucket":"pending"}
	]`
	c, err := ParseGHChecks(raw)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if c.State != model.ChecksFailing || c.Total != 3 || c.Failing != 1 || c.Pending != 1 {
		t.Errorf("checks = %+v", c)
	}
}

const glGraphQLFixture = `{
  "data": {
    "currentUser": {
      "authoredMergeRequests": {"pageInfo": {"hasNextPage": true, "endCursor": "GL_CURSOR"},
        "nodes": [
        {"iid":"5","title":"Add","webUrl":"u5","state":"opened","sourceBranch":"feat","targetBranch":"main","approved":true,"updatedAt":"2026-09-23T09:00:00Z","author":{"username":"me"},"project":{"fullPath":"grp/proj","name":"proj","group":{"fullPath":"grp"}}}
      ]},
      "reviewRequestedMergeRequests": {"nodes": [
        {"iid":"6","title":"Review me","webUrl":"u6","state":"opened","sourceBranch":"f","targetBranch":"main","approved":false,"updatedAt":"2026-09-23T09:00:00Z","author":{"username":"other"},"project":{"fullPath":"grp/proj","name":"proj","group":{"fullPath":"grp"}}}
      ]},
      "assignedMergeRequests": {"nodes": [
        {"iid":"8","title":"Assigned","webUrl":"u8","state":"opened","sourceBranch":"g","targetBranch":"main","approved":false,"updatedAt":"2026-09-23T09:00:00Z","author":{"username":"other"},"project":{"fullPath":"grp/other","name":"other","group":{"fullPath":"grp"}}}
      ]}
    }
  }
}`

func TestParseGLGraphQL(t *testing.T) {
	items, page, err := ParseGLGraphQL(glGraphQLFixture)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	if !page.More || page.Next != "GL_CURSOR" {
		t.Errorf("pageInfo = %+v", page)
	}

	if items[0].Section != model.SectionAuthored || items[0].Number != 5 {
		t.Errorf("authored = %+v", items[0])
	}
	if items[0].ReviewDecision != "APPROVED" {
		t.Errorf("reviewDecision = %q", items[0].ReviewDecision)
	}
	if items[1].Section != model.SectionReview || items[1].ReviewKind != model.ReviewRequested {
		t.Errorf("review requested = %+v", items[1])
	}
	if items[2].Section != model.SectionReview || items[2].ReviewKind != model.ReviewAssigned {
		t.Errorf("assigned = %+v", items[2])
	}
	if items[2].Ref.Owner != "grp" || items[2].Ref.Name != "other" {
		t.Errorf("ref = %+v", items[2].Ref)
	}
}

func TestGHChecksJSON(t *testing.T) {
	c, err := ParseGHChecks(`[{"name":"ci","state":"SUCCESS","bucket":"pass"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if c.State != model.ChecksPassing || c.Total != 1 {
		t.Fatalf("checks = %+v", c)
	}
}

// TestGLReviewDecisionFromApproved cubre C5: en CE solo hay `approved`; un MR
// no aprobado se reporta como desconocido (no se inventa "review required").
func TestGLReviewDecisionFromApproved(t *testing.T) {
	raw := func(approved bool) string {
		return `{"data":{"currentUser":{"authoredMergeRequests":{"nodes":[
			{"iid":"1","title":"t","state":"opened","approved":` + boolStr(approved) + `,"project":{"fullPath":"g/p","name":"p"}}
		]}}}}`
	}
	items, _, err := ParseGLGraphQL(raw(true))
	if err != nil {
		t.Fatal(err)
	}
	if items[0].ReviewDecision != "APPROVED" {
		t.Errorf("reviewDecision = %q, want APPROVED", items[0].ReviewDecision)
	}

	items, _, _ = ParseGLGraphQL(raw(false))
	if items[0].ReviewDecision != "" {
		t.Errorf("reviewDecision = %q, want vacío (desconocido)", items[0].ReviewDecision)
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestParseGLMRList(t *testing.T) {
	raw := `[
		{"iid":12,"title":"MR","web_url":"u","state":"opened","source_branch":"a","target_branch":"main","updated_at":"2026-09-21T07:00:00Z","author":{"username":"me"},"references":{"full":"grp/sub/proj!12","short":"!12"}}
	]`
	items, err := ParseGLMRList(raw)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	it := items[0]
	if it.Ref.Project != "grp/sub/proj" || it.Ref.Owner != "grp/sub" || it.Ref.Name != "proj" || it.Number != 12 {
		t.Errorf("ref = %+v", it.Ref)
	}
}

func TestParseGLTodos(t *testing.T) {
	raw := `[
		{"id":1,"action_name":"mentioned","target_type":"MergeRequest","updated_at":"2026-09-24T06:00:00Z","target":{"iid":3,"title":"Mention","web_url":"u","state":"opened","source_branch":"a","target_branch":"main","author":{"username":"other"},"references":{"full":"grp/proj!3"}}},
		{"id":2,"action_name":"assigned","target_type":"MergeRequest","target":{"iid":4,"title":"Assigned","references":{"full":"grp/proj!4"}}},
		{"id":3,"action_name":"mentioned","target_type":"Issue","target":{"iid":9,"title":"An issue","references":{"full":"grp/proj#9"}}}
	]`
	items, total, err := ParseGLTodos(raw)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1 (solo menciones de MR)", len(items))
	}
	if total != 3 {
		t.Errorf("total = %d, want 3 (todos de la página)", total)
	}
	it := items[0]
	if it.Section != model.SectionMentions || it.Number != 3 || it.Ref.Project != "grp/proj" {
		t.Errorf("item = %+v", it)
	}
	if it.Title != "Mention" || it.Author != "other" {
		t.Errorf("item = %+v", it)
	}
}

// TestParseNeverPanics comprueba que ninguna función de parseo entra en panic
// con entradas basura: siempre devuelven items más un error tipado.
func TestParseNeverPanics(t *testing.T) {
	inputs := []string{"", "null", "[]", "{}", "{", "\x00", "12345"}
	for _, in := range inputs {
		if _, _, err := ParseGHGraphQLSearch(in); err != nil {
			assertParseError(t, err)
		}
		if _, err := ParseGHAuthored(in); err != nil {
			assertParseError(t, err)
		}
		if _, _, err := ParseGLGraphQL(in); err != nil {
			assertParseError(t, err)
		}
		if _, err := ParseGLMRList(in); err != nil {
			assertParseError(t, err)
		}
		if _, _, err := ParseGLTodos(in); err != nil {
			assertParseError(t, err)
		}
		if _, err := ParseGHChecks(in); err != nil {
			assertParseError(t, err)
		}
	}
}

func assertParseError(t *testing.T, err error) {
	t.Helper()
	var pe *Error
	if !errors.As(err, &pe) {
		t.Fatalf("error = %T (%v), want *parse.Error", err, err)
	}
}

// TestParseGHChecksBuckets cubre C9: un check cancelado no debe pintarse como
// correcto.
func TestParseGHChecksBuckets(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want model.CheckState
	}{
		{"cancel falla", `[{"name":"a","state":"CANCELLED","bucket":"cancel"}]`, model.ChecksFailing},
		{"skipping no falla", `[{"name":"a","state":"SKIPPED","bucket":"skipping"}]`, model.ChecksPassing},
		{"pending", `[{"name":"a","state":"PENDING","bucket":"pending"}]`, model.ChecksPending},
		{"pass", `[{"name":"a","state":"SUCCESS","bucket":"pass"}]`, model.ChecksPassing},
		{"sin bucket usa estado", `[{"name":"a","state":"FAILURE"}]`, model.ChecksFailing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseGHChecks(c.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != c.want {
				t.Fatalf("state = %s, want %s (%+v)", got.State, c.want, got)
			}
		})
	}
}

// TestParseGHGraphQLSearchUnionFragments cubre C1: la respuesta real mezcla
// StatusContext (state) y CheckRun (status/conclusion), y puede traer nodos que
// no son PR (se descartan).
func TestParseGHGraphQLSearchUnionFragments(t *testing.T) {
	raw := `{"data":{"search":{"nodes":[
		{"__typename":"PullRequest","number":5,"title":"t","url":"u","state":"OPEN",
		 "headRefName":"a","baseRefName":"b","author":{"login":"me"},
		 "repository":{"nameWithOwner":"o/r","name":"r","owner":{"login":"o"}},
		 "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[
			{"__typename":"CheckRun","status":"COMPLETED","conclusion":"FAILURE"},
			{"__typename":"StatusContext","state":"SUCCESS","context":"ci/legacy"}
		 ]}}}}]}},
		{"__typename":"Repository","number":0}
	]}}}`
	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1 (el nodo no-PR se descarta)", len(items))
	}
	c := items[0].Checks
	if c.State != model.ChecksFailing || c.Total != 2 || c.Failing != 1 {
		t.Fatalf("checks = %+v", c)
	}
}

// glReviewRequestedRealFixture reproduce la forma real de
// reviewRequestedMergeRequests (redactada): `iid` llega como string (ID!),
// `pageInfo.endCursor` string y `hasNextPage` false.
const glReviewRequestedRealFixture = `{
  "data": {
    "currentUser": {
      "reviewRequestedMergeRequests": {
        "pageInfo": {"hasNextPage": false, "endCursor": "eyJjcmVhdGVkX2F0IjoiMjAyNi0wNy0xNyJ9"},
        "nodes": [
          {
            "iid": "1012",
            "title": "MR de ejemplo",
            "webUrl": "https://gitlab.example.com/g/p/-/merge_requests/1012",
            "state": "opened",
            "sourceBranch": "fix/x",
            "targetBranch": "deploy/Y",
            "approved": false,
            "updatedAt": "2026-09-24T14:36:26Z",
            "author": {"username": "someone"},
            "project": {"fullPath": "grp/sub/proj", "name": "proj", "group": {"fullPath": "grp/sub"}}
          }
        ]
      }
    }
  }
}`

// TestParseGLGraphQLStringIID cubre el tipo real de `iid` (ID! serializado como
// string): sin esto las tres listas de GitLab fallan al parsear.
func TestParseGLGraphQLStringIID(t *testing.T) {
	items, page, err := ParseGLGraphQL(glReviewRequestedRealFixture)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Number != 1012 {
		t.Errorf("Number = %d, want 1012", items[0].Number)
	}
	if items[0].Section != model.SectionReview || items[0].ReviewKind != model.ReviewRequested {
		t.Errorf("sección/kind = %v/%v", items[0].Section, items[0].ReviewKind)
	}
	if items[0].Ref.Project != "grp/sub/proj" || items[0].Ref.Owner != "grp/sub" {
		t.Errorf("ref = %+v", items[0].Ref)
	}
	if page.More || page.Next == "" {
		t.Errorf("pageInfo = %+v (More=false con endCursor string)", page)
	}
}

// TestGLDiffStatsSumsPerFileEntries cubre la trampa de GitLab: `diffStats` no es
// un agregado sino una entrada POR FICHERO cambiado, así que hay que sumarla.
// Estos números son los reales de APPCTTI/vsocial/backend/vsocial-api-actuacions
// !1015, y su endpoint /diffs confirma exactamente los mismos dos ficheros.
func TestGLDiffStatsSumsPerFileEntries(t *testing.T) {
	raw := `{"data":{"project":{"mergeRequest":{"iid":"1015","diffStats":[
		{"additions":40,"deletions":1},{"additions":2,"deletions":0}
	]}}}}`
	items, _, err := ParseGLGraphQL(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	d := items[0].Diff
	if !d.Known {
		t.Fatalf("diffstat presente debería ser conocido: %+v", d)
	}
	if d.Additions != 42 || d.Deletions != 1 || d.Files != 2 {
		t.Errorf("diffstat = %+v, want +42 -1 en 2 ficheros (sumado, no la primera entrada)", d)
	}
	if d.Total() != 43 {
		t.Errorf("Total = %d, want 43", d.Total())
	}
}

// TestGLDiffStatsEmptyVsAbsent separa las dos cosas que un slice vacío puede
// significar. Presente-pero-vacío es un MR que no toca nada (0/0, conocido);
// ausente es que la query no lo pidió, como en la API de Todos (desconocido).
// Confundirlos pintaría un "cambia 0 líneas" donde no se sabe nada.
func TestGLDiffStatsEmptyVsAbsent(t *testing.T) {
	withEmpty := `{"data":{"project":{"mergeRequest":{"iid":"7","diffStats":[]}}}}`
	items, _, err := ParseGLGraphQL(withEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if d := items[0].Diff; !d.Known || d.Total() != 0 || d.Files != 0 {
		t.Errorf("diffStats vacío = %+v, want conocido y 0/0", d)
	}

	without := `{"data":{"project":{"mergeRequest":{"iid":"7"}}}}`
	items, _, err = ParseGLGraphQL(without)
	if err != nil {
		t.Fatal(err)
	}
	if d := items[0].Diff; d.Known {
		t.Errorf("diffStats ausente = %+v, want desconocido", d)
	}
}

// TestGLDiffStatsToleratesStringCounts: la instancia serializa los contadores
// como número, pero flexInt los acepta igual que el iid, sin tumbar el parseo.
func TestGLDiffStatsToleratesStringCounts(t *testing.T) {
	raw := `{"data":{"project":{"mergeRequest":{"iid":"7","diffStats":[{"additions":"3","deletions":"4"}]}}}}`
	items, _, err := ParseGLGraphQL(raw)
	if err != nil {
		t.Fatal(err)
	}
	if d := items[0].Diff; !d.Known || d.Additions != 3 || d.Deletions != 4 {
		t.Errorf("diffstat = %+v, want +3 -4", d)
	}
}

// TestGHDiffStatFromScalarFields: GitHub ya devuelve el diffstat agregado en
// tres escalares, así que no hay nada que sumar.
func TestGHDiffStatFromScalarFields(t *testing.T) {
	raw := `{"data":{"repository":{"pullRequest":{"number":15,"additions":381,"deletions":36,"changedFiles":11}}}}`
	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if d := items[0].Diff; !d.Known || d.Additions != 381 || d.Deletions != 36 || d.Files != 11 {
		t.Errorf("diffstat = %+v, want +381 -36 en 11 ficheros", d)
	}
}

// TestGHDiffStatAbsentIsUnknown: `additions` es `Int!`, así que si el campo no
// viene es que la respuesta no lo trajo (respaldo REST u otra forma de salida).
// Reported=false, no un cambio de cero líneas.
func TestGHDiffStatAbsentIsUnknown(t *testing.T) {
	raw := `{"data":{"repository":{"pullRequest":{"number":15,"title":"t"}}}}`
	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatal(err)
	}
	if d := items[0].Diff; d.Known {
		t.Errorf("diffstat ausente = %+v, want desconocido", d)
	}
}

// TestGHDiffStatZeroIsKnown: un PR sin cambios da 0/0 y eso SÍ es un dato
// conocido, no un fallo de recogida.
func TestGHDiffStatZeroIsKnown(t *testing.T) {
	raw := `{"data":{"repository":{"pullRequest":{"number":15,"additions":0,"deletions":0,"changedFiles":0}}}}`
	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatal(err)
	}
	if d := items[0].Diff; !d.Known || d.Total() != 0 {
		t.Errorf("diffstat 0/0 = %+v, want conocido y total 0", d)
	}
}

// TestGLRESTAndTodosHaveNoDiffStat: ni el endpoint REST de merge requests ni la
// API de Todos traen el diffstat (gitlab-org/gitlab#464260), así que ambos
// parseos lo dejan desconocido en vez de报告显示 un cero.
func TestGLRESTAndTodosHaveNoDiffStat(t *testing.T) {
	mrs, err := ParseGLMRList(`[{"iid":5,"title":"t","references":{"full":"g/p!5"}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if d := mrs[0].Diff; d.Known {
		t.Errorf("REST mr list diffstat = %+v, want desconocido", d)
	}

	todos, _, err := ParseGLTodos(`[{"id":1,"action_name":"mentioned","target_type":"MergeRequest",
		"target":{"iid":9,"title":"t","references":{"full":"g/p!9"}}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 1 {
		t.Fatalf("todos = %d, want 1", len(todos))
	}
	if d := todos[0].Diff; d.Known {
		t.Errorf("todos diffstat = %+v, want desconocido", d)
	}
}

// TestFlexInt tolera número, string, nulo y valores no numéricos.
func TestFlexInt(t *testing.T) {
	cases := map[string]int{
		`{"n":12}`:    12,
		`{"n":"12"}`:  12,
		`{"n":null}`:  0,
		`{"n":""}`:    0,
		`{"n":"abc"}`: 0,
		`{}`:          0,
	}
	for raw, want := range cases {
		var v struct {
			N flexInt `json:"n"`
		}
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Fatalf("Unmarshal(%s): %v", raw, err)
		}
		if int(v.N) != want {
			t.Errorf("flexInt(%s) = %d, want %d", raw, int(v.N), want)
		}
	}
}
