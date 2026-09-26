# Summary: inbox-single-section

## Metadata
- **Completed:** 2026-09-26 22:01
- **Duration:** ~35 minutos (cierre)
- **Plan Number:** 0002
- **PR:** [#17](https://github.com/Sovengar/prdash/pull/17) — `feat/inbox-single-section` → `main`

## Scenarios
| Scenario (behavior.feature) | Status |
|-----------------------------|--------|
| Al abrir, la sección activa es Assigned | ✅ Passed |
| Solo se pinta una sección a la vez | ✅ Passed |
| tab cicla Assigned → Mentioned → Mine → Assigned | ✅ Passed |
| tab cicla también cuando una sección está vacía | ✅ Passed |
| La tecla de ciclo sale de la config, no de un "tab" cableado | ✅ Passed |
| La leyenda del borde superior izquierdo sustituye al título "Inbox" | ✅ Passed |
| La sección activa se resalta en la leyenda | ✅ Passed |
| Los conteos de la leyenda reflejan la sección deduplicada | ✅ Passed |
| El prefijo común de la sección activa se sigue mostrando | ✅ Passed |
| El prefijo mostrado es el de la sección activa | ✅ Passed |
| Una sección sin prefijo común no inventa uno | ✅ Passed |
| El prefijo no rompe el ancho de la tabla | ✅ Passed |
| Cada sección recuerda su cursor y su scroll | ✅ Passed |
| Un refresco conserva la posición de cada sección | ✅ Passed |
| La sección activa vacía se marca como vacía | ✅ Passed |
| El "loading more…" corresponde a la sección activa | ✅ Passed |
| Los avisos de consulta se muestran para la sección activa | ✅ Passed |
| El modo --print no cambia | ✅ Passed |
| En un terminal estrecho la leyenda se trunca sin romper la caja | ✅ Passed |

19/19 escenarios de `behavior.feature` (fuente única del comportamiento; no es
Cucumber). La verificación real es la suite Go: los escenarios se traducen en
tests unitarios/integration de `internal/tui` y `internal/forge/model`.

## Commits
Historia reescrita (History Finalization, agrupación determinista por
tipo/scope con invariante de tree-hash `092e103` idéntico antes/después):

- `feat(model): add short section labels for the inbox`
  (pliega `docs(planning): add 0002 inbox single-section plan` +
  `feat(model): add Section.Legend() for short section labels`)
- `feat(tui): show only the active inbox section`
  (pliega `test(tui)`, `refactor(tui)` y los `docs(adr)` ×2, `docs(changelog)`,
  `docs(readme)` de la rama)

## Files
- Created: `docs/adr/0004-inbox-single-section.md`,
  `docs/planning/0002-inbox-single-section/{issue.md,behavior.feature,plan.md,context.md}`,
  `docs/planning/0002-inbox-single-section/diagrams/{feature-flow.md,process-flow.md}`,
  `internal/tui/section_test.go`
- Modified: `internal/tui/{app.go,list.go,refcol.go,sections.go,styles.go,update.go}`,
  `internal/forge/model/model.go`, `README.md`, `CHANGELOG.md`,
  y sus tests (`app_test.go`, `list_test.go`, `refcol_test.go`, `table_test.go`,
  `comments_test.go`, `mount_test.go`, `auth_reason_test.go`,
  `sim_test.go`, `sim_integration_test.go`, `model_test.go`)

## Tests
- Added: 17 funciones de test (1 fichero nuevo: `internal/tui/section_test.go`,
  353 líneas)
- System Tests: ✅ Passed — `make test` → `go build` + `go vet` + `gofmt` +
  `go test -race ./...`, exit 0 (corrido antes y después de la reescritura de
  historia; árbol byte-idéntico)

## Documentation
- Changelog: ✅ Updated — entrada en `## [Unreleased]` → `### Changed`
  (commit `docs(changelog)`, sin reconciliación necesaria al cierre)
- Docs: `README.md` (sección por defecto y leyenda de conteos),
  `docs/planning/0002-inbox-single-section/*`
- ADR: ✅ Created — `docs/adr/0004-inbox-single-section.md` (Accepted),
  supersede dos cláusulas del ADR 0002 (prefijo en cabecera de sección y
  alcance «todas las secciones» del ancho de ITEM)

## Code Review Issues
- Critical Found: 0
- High Found: 0
- User Decision: Approved to continue

## Next Step
Merge de PR #17 con `gh pr merge --rebase` (historia lineal, sin squash), luego
limpieza de rama/worktree. Post-merge: `make install` lo corre el usuario
(expresamente fuera del alcance de este cierre). Pendiente declarado en
`plan.md`: el prefijo de ruta seleccionable/toggleable es una feature posterior
(aquí solo se dejó la costura).
