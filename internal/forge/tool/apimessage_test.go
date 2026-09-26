package tool

import (
	"errors"
	"testing"
)

// ghValidationErr es el error exacto que produce `gh api` cuando la rama base no
// existe. Es un caso real, no inventado: sale así, con el argv entero delante, y
// el motivo de verdad en el cuerpo de la respuesta.
const ghValidationErr = `gh api -X PATCH repos/acme/widget/pulls/7 -f base=main2: ` +
	`gh: Validation Failed (HTTP 422) (exit 1)`

// ghValidationBody es el cuerpo de esa misma respuesta.
const ghValidationBody = `{"message":"Validation Failed","errors":[{"message":` +
	`"Proposed base branch 'main2' was not found","resource":"PullRequest","field":"base","code":"invalid"}],` +
	`"documentation_url":"https://docs.github.com/rest/pulls/pulls#update-a-pull-request","status":"422"}`

// TestKindDe422EsValidationYNoNetwork: un 422 caía en `network` porque el texto de
// stderr no tiene ni "conflict" ni "not found", y `network` acaba en "forge
// conflict", que promete un refresco que no puede arreglar un nombre de rama malo.
func TestKindDe422EsValidationYNoNetwork(t *testing.T) {
	if got := Kind(errors.New(ghValidationErr)); got != "validation" {
		t.Errorf("Kind = %q, want validation (no network: no hay conflicto que refrescar)", got)
	}
}

// TestKindPorCodigoHTTP: el 422 también tiene que salir por la vía del código, que
// es la que se mira antes que el texto. Sin esto, el mismo fallo se clasificaría
// distinto según qué línea de stderr llegue.
func TestKindPorCodigoHTTP(t *testing.T) {
	if got := kindForHTTP(422); got != "validation" {
		t.Errorf("kindForHTTP(422) = %q, want validation", got)
	}
}

// TestAPIMessagePrefiereElDetalle: GitHub pone "Validation Failed" en `message` y el
// campo que no valía en `errors[].message`. Al revés se enseñaría el genérico, que
// es justo el mensaje que no dice nada.
func TestAPIMessagePrefiereElDetalle(t *testing.T) {
	want := "Proposed base branch 'main2' was not found"
	if got := APIMessage(ghValidationBody); got != want {
		t.Errorf("APIMessage = %q, want %q", got, want)
	}
}

// TestAPIMessageCaeAMessage: GitLab no manda `errors`, así que su motivo está en
// `message`. Y si el `message` es un objeto —GitLab lo hace cuando el error es de
// campo— no se enseña: un objeto no es un motivo legible.
func TestAPIMessageCaeAMessage(t *testing.T) {
	if got := APIMessage(`{"message":"404 Not found"}`); got != "404 Not found" {
		t.Errorf("APIMessage = %q, want el message de GitLab", got)
	}
	if got := APIMessage(`{"message":{"base":["is invalid"]}}`); got != "" {
		t.Errorf("APIMessage = %q, want vacío con un message que no es texto", got)
	}
}

// TestAPIMessageNoSeInventa: un cuerpo que no es JSON, o vacío, dan "". Es lo que
// hace que el llamante pueda区分 entre "la API no dijo nada" y "la API dijo algo".
func TestAPIMessageNoSeInventa(t *testing.T) {
	for _, body := range []string{"", "not json", "[]", "{}"} {
		if got := APIMessage(body); got != "" {
			t.Errorf("APIMessage(%q) = %q, want vacío", body, got)
		}
	}
}
