# Summary: worktrees-cleanup

## Metadata
- **Completed:** 2026-09-27 20:30
- **Duration:** ~15 minutos (cierre)
- **Plan Number:** 0004
- **PR:** [#21](https://github.com/Sovengar/prdash/pull/21) — `feat/worktrees-cleanup` → `main`

## Scenarios
| Scenario (behavior.feature) | Status |
|-----------------------------|--------|
| `--orphans` borra todos los huérfanos y solo los huérfanos | ✅ Passed |
| `--orphans` borra exactamente lo que "worktrees list" marca como huérfano | ✅ Passed |
| `--orphans` nunca toca un worktree ajeno | ✅ Passed |
| un worktree propio sano nunca se borra con `--orphans` | ✅ Passed |
| `--orphans` con cero huérfanos es el caso feliz, no un error | ✅ Passed |
| `--dry-run` muestra el lote exacto que borraría y no borra nada | ✅ Passed |
| `--dry-run` con cero huérfanos sigue siendo el caso feliz | ✅ Passed |
| `--dry-run` sin `--orphans` es un error de uso | ✅ Passed |
| `--orphans` es excluyente con las rutas explícitas | ✅ Passed |
| un flag desconocido en `remove` es un error de uso | ✅ Passed |
| un token con guion inicial es un flag, nunca una ruta | ✅ Passed |
| `remove` con rutas explícitas sigue comportándose como antes | ✅ Passed |
| un huérfano con un enlace `.git` irresoluble se borra por ruta explícita | ✅ Passed |
| un huérfano con `.git` irresoluble no tumba el lote de `--orphans` | ✅ Passed |
| `remove` sin rutas ni `--orphans` sigue siendo error de uso | ✅ Passed |
| un merge OK desde prdash borra el worktree limpio del ítem | ✅ Passed |
| un merge OK con el worktree sucio lo conserva y lo dice | ✅ Passed |
| un archivo nuevo sin trackear también cuenta como sucio | ✅ Passed |
| ante un estado de git ilegible se conserva el worktree | ✅ Passed |
| un merge OK sin worktree montado no reporta error | ✅ Passed |
| si el worktree ya no está en disco, es un no-op sin error | ✅ Passed |
| el aviso del merge conserva todas las verdades a la vez | ✅ Passed |
| approve no borra ningún worktree | ✅ Passed |
| retarget no borra ningún worktree | ✅ Passed |
| un merge que no sale bien no borra ningún worktree | ✅ Passed |
| cerrar la app no borra ningún worktree | ✅ Passed |
| un PR mergeado directamente en el forge no dispara la limpieza | ✅ Passed |
| ninguna vía de limpieza toca un worktree ajeno o fuera de la raíz | ✅ Passed |
| la guarda de raíz gestionada aplica en las dos provisiones | ✅ Passed |
| una ruta inexistente se rechaza sin tocar nada | ✅ Passed |

30/30 escenarios de `behavior.feature` (fuente única del comportamiento; no es
Cucumber). La verificación real es la suite Go: los escenarios se traducen en
tests unitarios/integration de `cmd/prdash`, `internal/worktree`,
`internal/tui` y `internal/review/executor`.

## Commits
Historia reescrita (History Finalization): 21 commits `merge-base..HEAD`
reducidos a **7 grupos** por unidad de comportamiento. Agrupación determinista —
primarios (`feat`/`fix`/`refactor`/`perf`) agrupados en runs contiguos por
`scope`, secundarios (`docs`/`test`/`chore`/`ci`/`build`/`style`) plegados en el
grupo primario anterior (o en el siguiente si aún no hay ninguno), y un
secundario con `scope` distinto cierra el run. La forma se llevó al punto fijo
(3 pasadas) y se emitió en un solo rebase.

Invariante de árbol: `58876b12cd13cd34c721c29a78c53534699fd462` idéntico antes
(`3fd78e5`) y después (`b5de918`) del rewrite — el código se preserva byte a
byte; solo cambian mensajes y agrupación.

- `feat(worktree): remove a worktree only when it is clean`
  (pliega `docs: add worktrees-cleanup plan (0004)`)
- `feat(worktrees): batch-remove orphans with --orphans and --dry-run`
- `feat(review): remove the active review worktree only when clean`
- `feat(tui): auto-remove a merged item's clean worktree`
  (pliega `docs: document orphan cleanup and merge-time worktree removal`,
  `refactor(tui): clean up the worktree cleanup tests`,
  `test(worktree): cover HerdrNative.RemoveIfClean`)
- `fix(worktrees): remove orphans lacking a gitdir; time each batch`
  (pliega `test(tui)`, `docs(worktrees)` y `docs:` de ese tramo)
- `fix(worktree): apply the managed-root guard in the native removal path`
- `fix(tui): fire the cleanup notice once and update it in place`
  (pliega los `test(worktrees)` y `docs:` finales)

## Files
- Created: `docs/adr/0007-worktree-cleanup-on-merge.md`,
  `docs/planning/0004-feature-worktrees-cleanup/{behavior.feature,context.md,issue.md,plan.md}`,
  `docs/planning/0004-feature-worktrees-cleanup/diagrams/{feature-flow.md,process-flow.md}`
- Modified: `CHANGELOG.md`, `README.md`,
  `cmd/prdash/{main.go,worktrees.go,worktrees_test.go}`,
  `internal/cache/memo.go`, `internal/reporesolver/{reporesolver.go,reporesolver_test.go}`,
  `internal/review/executor/{executor.go,executor_test.go}`,
  `internal/tui/{app.go,app_test.go,merge_test.go,retarget.go,toast.go,toast_test.go,update.go}`,
  `internal/worktree/{audit_test.go,herdr.go,herdr_test.go,worktree.go,worktree_test.go}`

## Tests
- Added: 48 funciones de test (ningún fichero de test nuevo; los que crecen son
  `cmd/prdash/worktrees_test.go` +393, `internal/tui/merge_test.go` +365,
  `internal/worktree/worktree_test.go` +145, `internal/worktree/herdr_test.go` +131)
- System Tests: ✅ Passed — `make test` (build + vet + gofmt + `go test -race ./...`)

## Documentation
- Changelog: ✅ Updated — dos entradas nuevas en `### Added` de `[Unreleased]`
  (`prdash worktrees remove --orphans` y auto-borrado del worktree de un PR
  mergeado solo si está limpio)
- Docs: `README.md` (sección `worktrees remove --orphans` y avisos de limpieza)
- ADR: ✅ Created — `docs/adr/0007-worktree-cleanup-on-merge.md`

## Code Review Issues
- Critical Found: 0
- High Found: 0
- User Decision: Approved to continue

## Next Step
Merge lineal (`gh pr merge 21 --rebase`) sobre `main` y limpieza post-merge
(`fetch --prune`, `branch -D`, destrucción del worktree vía workspace-manager).
