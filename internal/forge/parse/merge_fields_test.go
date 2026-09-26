package parse

import (
	"testing"

	"prdash/internal/forge/model"
)

// TestGHNodeCarriesThePinAndTheRules: sin estos dos datos en el parseo, el pin
// nunca se puede satisfacer y el filtro de modos se queda vacío. Es el eslabón
// entre la query y el adapter, y el más fácil de romper en silencio porque el
// resto sigue funcionando con un HeadSHA vacío.
func TestGHNodeCarriesThePinAndTheRules(t *testing.T) {
	raw := `{"data":{"search":{"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"number":7,"title":"T","url":"https://github.com/acme/widget/7",
		"state":"OPEN","isDraft":false,"reviewDecision":"APPROVED",
		"headRefOid":"deadbeefcafe","headRefName":"feat/x","baseRefName":"main",
		"additions":1,"deletions":0,"changedFiles":1,
		"author":{"login":"me"},
		"repository":{"nameWithOwner":"acme/widget","name":"widget",
			"owner":{"login":"acme"},
			"mergeCommitAllowed":true,"rebaseMergeAllowed":true,"squashMergeAllowed":false}}]}}}`

	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatalf("ParseGHGraphQLSearch: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].HeadSHA != "deadbeefcafe" {
		t.Errorf("HeadSHA = %q, want deadbeefcafe", items[0].HeadSHA)
	}
	if !items[0].Merge.Known {
		t.Error("Merge.Known = false, want true: los tres flags venían en la respuesta")
	}
	if !items[0].Merge.Rebase || !items[0].Merge.MergeCommit {
		t.Errorf("Merge = %+v, want rebase y merge commit permitidos", items[0].Merge)
	}
	// El caso que importa: squash desactivado en el repositorio tiene que leerse
	// como desactivado, no como "no lo sé".
	if items[0].Merge.Squash {
		t.Errorf("Merge.Squash = true, want false: el repositorio lo tiene desactivado")
	}
}

// TestGHNodeWithoutRulesStaysUnknown: la respuesta REST de respaldo no trae los
// flags. Marcar Known con los tres en false haría que el menú no ofreciera
// ningún modo, que es peor que ofrecer los tres: el usuario se quedaría sin
// salida y no sabría por qué.
func TestGHNodeWithoutRulesStaysUnknown(t *testing.T) {
	raw := `{"data":{"search":{"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"number":7,"title":"T","url":"u","state":"OPEN","isDraft":false,
		"reviewDecision":"","headRefName":"f","baseRefName":"main",
		"additions":1,"deletions":0,"changedFiles":1,
		"author":{"login":"me"},
		"repository":{"nameWithOwner":"acme/widget","name":"widget","owner":{"login":"acme"}}}]}}}`

	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatalf("ParseGHGraphQLSearch: %v", err)
	}
	if items[0].Merge.Known {
		t.Errorf("Merge = %+v, want Known=false", items[0].Merge)
	}
	if got := len(allowed(items[0].Merge)); got != 3 {
		t.Errorf("con reglas desconocidas se ofrecen %d modos, want 3", got)
	}
}

// TestGHNodeWithPartialRulesStaysUnknown: los tres flags tienen que venir. Con
// uno solo, asumir que los ausentes valen false dejaría fuera el único modo que
// el repositorio quizá sí permite.
func TestGHNodeWithPartialRulesStaysUnknown(t *testing.T) {
	raw := `{"data":{"search":{"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"number":7,"title":"T","url":"u","state":"OPEN","isDraft":false,
		"reviewDecision":"","headRefName":"f","baseRefName":"main",
		"additions":1,"deletions":0,"changedFiles":1,
		"author":{"login":"me"},
		"repository":{"nameWithOwner":"acme/widget","name":"widget",
			"owner":{"login":"acme"},"rebaseMergeAllowed":true}}]}}}`

	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatalf("ParseGHGraphQLSearch: %v", err)
	}
	if items[0].Merge.Known {
		t.Errorf("Merge = %+v, want Known=false con reglas parciales", items[0].Merge)
	}
}

// TestGLNodeCarriesThePin: `diffHeadSha` es lo que GitLab usa para `--sha`. Va
// como puntero porque el schema lo declara nullable; un MR recién abierto puede
// devolverlo a null y eso NO puede confundirse con un SHA vacío que se pueda
// usar.
func TestGLNodeCarriesThePin(t *testing.T) {
	raw := `{"data":{"currentUser":{"authoredMergeRequests":{
		"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"iid":7,"title":"T","webUrl":"u","state":"opened",
		"sourceBranch":"f","targetBranch":"main","approved":true,
		"diffHeadSha":"beef1234","squash":false,
		"diffStats":[{"additions":1,"deletions":0}],
		"author":{"username":"me"},
		"project":{"fullPath":"grp/proj","name":"proj","group":{"fullPath":"grp"}}}]}}}}`

	items, _, err := ParseGLGraphQL(raw)
	if err != nil {
		t.Fatalf("ParseGLGraphQL: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].HeadSHA != "beef1234" {
		t.Errorf("HeadSHA = %q, want beef1234", items[0].HeadSHA)
	}
	// GitLab no expone las estrategias por GraphQL, así que llegan sin conocer.
	// No restringe, y eso es lo correcto: no saber no es no permitir.
	if items[0].Merge.Known {
		t.Errorf("Merge = %+v, want Known=false en GitLab", items[0].Merge)
	}
}

// TestGLNodeWithNullPinStaysUnpinned: `diffHeadSha: null` es un MR cuyo diff no
// está calculado. Traducirlo a "" es lo correcto —el merge que lo necesita se
// niega—, pero lo que no puede pasar es inventarse un SHA.
func TestGLNodeWithNullPinStaysUnpinned(t *testing.T) {
	raw := `{"data":{"currentUser":{"authoredMergeRequests":{
		"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"iid":7,"title":"T","webUrl":"u","state":"opened",
		"sourceBranch":"f","targetBranch":"main","approved":true,
		"diffHeadSha":null,"squash":false,
		"diffStats":[{"additions":1,"deletions":0}],
		"author":{"username":"me"},
		"project":{"fullPath":"grp/proj","name":"proj","group":{"fullPath":"grp"}}}]}}}}`

	items, _, err := ParseGLGraphQL(raw)
	if err != nil {
		t.Fatalf("ParseGLGraphQL: %v", err)
	}
	if items[0].HeadSHA != "" {
		t.Errorf("HeadSHA = %q, want vacío: un diff null no es un SHA", items[0].HeadSHA)
	}
}

// allowed cuenta los modos que las reglas dejan pasar, replicando lo que hace
// forge.AllowedModes sin importar ese paquete (que importa a state, y state
// importa a model: el test vive en parse y no necesita esa cadena).
func allowed(r model.MergeRules) []bool {
	if !r.Known {
		return []bool{true, true, true}
	}
	return []bool{r.MergeCommit, r.Rebase, r.Squash}
}
