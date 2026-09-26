package parse

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseGHBranches extrae los nombres de rama de la salida de `gh api ... --jq
// '.[].name'`: un nombre por línea.
//
// Se separan por líneas y no se parsea el JSON porque el jq ya lo hizo en la CLI.
// Que una ref no pueda traer un salto de línea es lo que lo hace seguro: git
// prohíbe el control y el espacio en los nombres de ref, así que una línea es
// una rama y no hay forma de que dos se fundan en una.
func ParseGHBranches(out string) []string {
	return branchLines(out)
}

// ParseGLBranches extrae los nombres de rama del NDJSON que devuelve
// `glab api ... --paginate --output ndjson` sobre el listado de ramas: un objeto
// JSON por línea, cada uno con su `name`.
//
// Va por NDJSON y no por el JSON entero porque glab no tiene `--jq` (gh sí), y con
// `--paginate` la respuesta son varias páginas: un decodificador que leyera un
// único valor JSON se comería la primera página y tiraría el resto, que es
// justo la parte del repositorio que no cabe en una pantalla.
//
// Una línea que no es un objeto con nombre se salta en vez de abortar: el listado
// de ramas es la única lectura de esta acción y perderla entera por una línea
// rara es peor que devolver lo que se pudo leer.
func ParseGLBranches(out string) ([]string, error) {
	var names []string
	var lastErr error
	seen := 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		seen++
		var br struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(line), &br); err != nil {
			lastErr = err
			continue
		}
		if br.Name != "" {
			names = append(names, br.Name)
		}
	}
	// Sin ninguna línea legible no es "un repo sin ramas": es una respuesta que no
	// se entendió, y confundirlas dejaría el buscador vacío sin decir por qué.
	if seen > 0 && len(names) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("could not read the branch list: %w", lastErr)
		}
		return nil, fmt.Errorf("could not read the branch list")
	}
	return names, nil
}

// branchLines parte una salida de un nombre por línea y descarta los huecos.
func branchLines(out string) []string {
	raw := strings.Split(out, "\n")
	names := make([]string, 0, len(raw))
	for _, line := range raw {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}
