---
feature: 0004-feature-worktrees-cleanup
freshness: 925270001710278918c82eac8443603e4aca9b85
codegraph: ready
generated_by: codebase-researcher
---

# Context: Limpieza de worktrees — `--orphans` en lote y auto-borrado al mergear

**Worktree root (único prefijo de todos los paths de abajo — no asumas que el shell arranca aquí):**
`/home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup`

Todas las rutas de este documento son absolutas. `codegraph sync` corrió OK
(`Already up to date`, `.codegraph/codegraph.db` presente) ⇒ `codegraph: ready`.

## Scope
- In: flag `--orphans` (+ `--dry-run`) en `prdash worktrees remove`; `parseRemoveArgs`;
  `Provisioner.RemoveIfClean` con candado "solo si limpio"; `RemoveReview` en el
  `Executor` + puerto `ReviewRemover`/`SetReviewRemover`; `Resolver.ForgetReview` +
  `cache.Store.DeleteReview`; disparador B en `applyAction` + mensaje `reviewCleanupMsg`
  que compone el aviso; ADR 0007 / README / CHANGELOG.
- Out: borrado al cerrar la app; acción "close" nueva; semántica de `Audit` o de
  `Remove` (rutas explícitas); schema TOML; `list`; `forge`.

## Files to Touch
| Symbol / Area | File | Lines | Why |
|---------------|------|-------|-----|
| `worktreeTimeout` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees.go | 23 | Timeout compartido por `list` y cada `remove`; reusarlo también para el lote `--orphans`. |
| `runWorktrees` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees.go | 26-40 | Despacho `list`/`remove`; `remove` pasa a `parseRemoveArgs(args[1:])` antes de decidir modo. Devuelve `int` (no `os.Exit`): los tests lo conducen en proceso. |
| `removeWorktrees` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees.go | 74-99 | **Ya itera sobre todas las rutas.** Extender con `--orphans`/`--dry-run`; conservar el bloque de rechazo por ruta (86-90, `Owned`+`Exists`) y exit 1. Exit 2 en usos inválidos, 0 en éxito (incluido cero huérfanos). |
| `listWorktrees` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees.go | 42-70 | Modelo de cómo `Audit` se filtra por `Entry.Orphan` (57-67) y del par stdout/stderr esperado. No se cambia; es la fuente de verdad de "huérfano". |
| `main` intercept | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/main.go | 28-34 | Intercepta `worktrees` **antes** de `flag.Parse` y pasa `worktree.Select(..., cfg.WorktreeDir)`. **Sin cambios** de A; B añade `model.SetReviewRemover(ex)` junto a `SetReviewLookup` (línea 64). |
| `worktreeRepoFixture` / `worktreeFixture` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees_test.go | 16-37 | Fixtures reales de git (`testutil.InitRepo`/`CommitFile`/`RunGit`); hoy crean 1 propio (`prdash-pr-1`) + 1 ajeno. Ampliar para dos huérfanos + sano + ajeno. |
| `captureStdout` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/print_test.go | 141-156 | Plantilla exacta para el `captureStderr` que faltan los escenarios de exit 2 (swap de `os.Stderr` + pipe). |
| `Provisioner` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/worktree.go | 43-53 | Añadir `RemoveIfClean(ctx, id) (removed bool, reason string, err error)`. `Remove` (48) se queda **exactamente** como está. |
| `GitDirect.Remove` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/worktree.go | 108-128 | **Sin cambios.** Referencia para `RemoveIfClean`: mismo `removablePath` (109) y mismo `os.RemoveAll`/prune. Una ruta ausente da error hoy ⇒ `RemoveIfClean` comprueba antes. |
| `removablePath` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/worktree.go | 132-143 | Guard único de ownership+raíz; `RemoveIfClean` lo usa **primero**. No se relaja. |
| `git.Run(...)` usage | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/worktree.go | 88, 116, 122, 169 | Patrón de invocación de git desde el paquete; el helper `dirty` usa `g.git.Run(ctx, id, "status", "--porcelain")`. |
| `gitcmd.Runner.Run` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/gitcmd/gitcmd.go | 60-93 | Ejecutor único de git (env no interactivo). Devuelve stdout recortado — `TrimSpace!="" ⇒ sucio`. |
| `Owned` / `LabelPrefix` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/audit.go | 14, 19-21 | Guard de ownership; no se toca. |
| `Entry` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/audit.go | 24-31 | `Orphan bool` + `Reason string`: el filtro de `--orphans` es `e.Orphan`. |
| `Audit` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/audit.go | 36-71 | Fuente única de "huérfano" (`sourceReachable`, 62-65). **No se reimplementa ni se cambia su semántica.** |
| `sourceReachable` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/audit.go | 96-103 | Marca huérfano ante **cualquier** error de `os.Stat` del gitdir (falso-huérfano posible) ⇒ motivo del `--dry-run`. |
| `HerdrNative.Remove` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/herdr.go | 174-188 | Modelo para `HerdrNative.RemoveIfClean`: resolver workspace con `WorktreeList` y delegar en `Remove` nativo, cayendo a `h.scan` si no hay workspace. |
| `HerdrNative.Audit` / `Select` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/herdr.go | 197 / 39-44 | `Audit` delega en `GitDirect`; `Select` elige impl según entorno. Se mantiene la simetría nativa/directa. |
| `Resolver` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor.go | 23-33 | Añadir `ForgetReview(it model.Item) error` al puerto. |
| `Executor` struct + `Worktrees` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor.go | 48-57 | El `Executor` ya tiene el `Provisioner`: `RemoveReview` reusa este campo, sin vía nueva de borrado. |
| `ActiveReview` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor.go | 128-140 | Mapeo ítem→worktree que usa `RemoveReview`. Cuidado: usa el `rec.Worktree` del registro. |
| `resolveRepo` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor.go | 145-156 | Patrón de uso del `Resolver` desde el `Executor` (no se modifica). |
| `RemoveReview` (nuevo) | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor.go | nuevo | `ActiveReview` → ausente ⇒ `(false,"",nil)`; presente ⇒ `Worktrees.RemoveIfClean`; solo si `removed==true` ⇒ `Resolver.ForgetReview`. |
| `fakeProvisioner` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor_test.go | 354-361 | Doble del `Provisioner`; añadir `RemoveIfClean`. Se construye en `TestMountPassesNativeContainerToLayout` (340). Los fakes de `Resolver` del mismo paquete necesitan `ForgetReview`. |
| `Remember` / `RecordReview` / `ActiveReview` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/reporesolver/reporesolver.go | 199-204 / 207-210 / 213-215 | Añadir `ForgetReview` junto a ellas; usa `itemKey(it.ID())` (372-374). |
| `ReviewRecord` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/cache/memo.go | 17-23 | Registro persistido que hay que olvidar. |
| `Store.Review` / `SetReview` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/cache/memo.go | 107-120 | Añadir `DeleteReview(key)` con `delete(s.memo.Reviews, key)` + `s.saveLocked()` (122-127) para persistir. |
| `ActionKind` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/forge/forge.go | 266-276 | Confirma que B es `ActionMerge`+`OK`; no hay acción de cierre. **Sin cambios.** |
| `Outcome` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/forge/forge.go | 379-418 | `Kind`, `OK`, `DeleteMsg`, `Item`, `HasItem`: **todo lo que B necesita ya viaja aquí**. |
| `RunAction` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/forge/forge.go | 466-501 | Embudo del merge; no se toca. |
| `Derive` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/state/state.go | 86-111 | `StateMerged`/`StateClosed`; no se usa para el disparador (que es `out.Kind`+`out.OK`). Sin cambios. |
| `Update` switch + `case actionMsg` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/update.go | 20-130 (89-91) | Añadir `case reviewCleanupMsg:` que componga el aviso. `actionMsg` es el molde de "resultado de trabajo en background". |
| `applyAction` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/update.go | 154-199 | **Punto único de B.** Hooks en la rama `out.OK` (177). Ojo a los `return` tempranos: `DeleteMsg` (182-186) y retarget desfasado (190-194). |
| `actionDoneNotice` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/update.go | 480-496 | Aviso base del merge; B compone sobre él, no lo pisa. |
| `startAction` / `launchAction` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/update.go | 501-510 / 519-530 | Molde del borrado en background: goroutine + `sendEvent(actionMsg{...})`. B usa el mismo patrón con `reviewCleanupMsg`. |
| `Model` fields + `reviewLookup` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/app.go | 190-318 (286-289) | Añadir campo `reviewRemover ReviewRemover`. |
| `Mounter` / `SetMounter` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/app.go | 99-101 / 377-379 | **Molde exacto** de puerto opcional + inyector ("nil lo deshabilita"). `ReviewRemover`/`SetReviewRemover` lo copian. |
| `mountMsg` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/app.go | 77-81 | Molde del mensaje de resultado tipado ⇒ `reviewCleanupMsg`. |
| `SetReviewLookup` call | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/main.go | 64 | Sitio donde añadir `model.SetReviewRemover(ex)` justo después. |
| `ReviewLookup` / `SetReviewLookup` / `staleReviewNotice` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/retarget.go | 133-135 / 149 / 682-691 | Puerto **solo-lectura** hoy; **no** se extiende. `staleReviewNotice` conserva su significado (worktree del usuario). |
| `fakeLookup` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/retarget_test.go | 523-532 | Doble del lookup; los tests de B necesitan además un `ReviewRemover` fake. |
| `WorktreeDir` config | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/config/config.go | 132, 147, 235-236, 318 | Clave/toml/default ya existen. **Cero cambios de schema** (el `DeleteReview` es del cache, no de config). |
| README §worktrees | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/README.md | 490-500 | Corregir `remove <ruta>` → `remove <ruta>…`, documentar `--orphans`/`--dry-run` y cuándo borra B. Línea 493-494 fija la regla de conservación que **no** se contradice. |
| CHANGELOG `[Unreleased]` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/CHANGELOG.md | 8-11 | Añadir entrada en `### Added`. |
| ADR 0007 | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/docs/adr/0007-worktree-cleanup-on-merge.md | 1-106 | Ya redactado en planning; el executor lo mantiene coherente (no reescribe ADRs aceptados). |

## Contracts
- `Provisioner.RemoveIfClean(ctx context.Context, id string) (removed bool, reason string, err error)` — nuevo en `internal/worktree/worktree.go:43`. `Remove(ctx, id) error` (`:48`) **no** cambia de firma ni de semántica. El candado "solo si limpio" vive **solo** en `RemoveIfClean`: no dentro de `Remove`, no en un booleano.
- `Executor.RemoveReview(ctx context.Context, it model.Item) (removed bool, reason string, err error)` — nuevo en `internal/review/executor/executor.go`. Implementa el puerto TUI; mapea con `ActiveReview` (`:128`) y delega en `Worktrees.RemoveIfClean`. Solo llama `Resolver.ForgetReview` si `removed == true`.
- `Resolver.ForgetReview(it model.Item) error` — nuevo en la interfaz `internal/review/executor/executor.go:23` y en `internal/reporesolver/reporesolver.go` (junto a `ActiveReview`, `:213`).
- `cache.Store.DeleteReview(key string)` — nuevo en `internal/cache/memo.go` (junto a `SetReview`, `:115`); debe llamar `saveLocked()` (`:122`) para persistir.
- Puerto TUI nuevo `ReviewRemover interface { RemoveReview(ctx context.Context, it model.Item) (bool, string, error) }` + `(*Model).SetReviewRemover(ReviewRemover)`; **no** se extiende `ReviewLookup` (contrato solo-lectura, `retarget.go:133`).
- `parseRemoveArgs(args []string) (orphans, dryRun bool, paths []string, err error)` — helper **puro** (no toca `os.Args`); `--orphans`+rutas ⇒ error, `--dry-run` sin `--orphans` ⇒ error, token `-…` desconocido ⇒ error, sin nada ⇒ error. Usos inválidos: stderr + exit 2 + cero borrados. Éxito: stdout + exit 0; fallo de borrado: exit 1.
- `reviewCleanupMsg` — nuevo tipo de mensaje en `internal/tui` que transporta el `base` (aviso del merge capturado al disparar) + `(removed, reason, err)` y compone el aviso final.
- `forge.Outcome` (`Kind`/`OK`/`DeleteMsg`/`Item`/`HasItem`) se **respeta** como contrato de entrada de B: no se añade plumbing al forge.

## Pattern to Follow
- **Puerto opcional + inyector con degradación** → `Mounter`/`SetMounter` (`internal/tui/app.go:99-101`, `:377-379`); `ReviewRemover`/`SetReviewRemover` lo copian literalmente ("nil lo deshabilita"). Inyección desde `main.go:64`.
- **Trabajo en background + mensaje tipado** → `launchAction` (`internal/tui/update.go:519-530`) y `mountMsg` (`internal/tui/app.go:77-81`): goroutine con timeout, `sendEvent`, `case ...Msg` en `Update` (89, 103). B usa este mismo esqueleto.
- **Guard + borrado de worktree** → `GitDirect.Remove` (`internal/worktree/worktree.go:108-128`): `removablePath` primero; `RemoveIfClean` replica el guard y decide antes de borrar.
- **Filtro por huérfano** → `listWorktrees` (`cmd/prdash/worktrees.go:55-68`) y `Audit` (`internal/worktree/audit.go:36-71`): `--orphans` filtra `e.Orphan`, sin reimplementar detección.
- **CLI in-process testeable** → `runWorktrees`/`removeWorktrees` devuelven `int`; los tests usan `captureStdout` (`cmd/prdash/print_test.go:141-156`) y fixtures de git reales (`cmd/prdash/worktrees_test.go:16-37`).
- **Tests de TUI por mensajes** → `merge_test.go` (`newMergeFixture` `:38`, `waitOutcome` `:57`, `send(t, m, actionMsg{...})` `:311`) y helpers `newTestModel`/`send`/`press`/`lastToast` (`internal/tui/app_test.go:23,66,76,117`).
- **Tests de worktree con repos reales** → `internal/worktree/worktree_test.go` (`TestCreateListRemove` `:21`) y `audit_test.go` (`TestRemoveOrphanDeletesCheckout` `:105`, `TestRemoveRefusesForeignWorktree` `:129`).

## Tests
- Existing affected:
  - /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees_test.go — `TestRunWorktreesListsOnlyOwned` `:41`, `TestRunWorktreesRemoveRefusesForeign` `:72`, `TestRunWorktreesRemoveOwned` `:89`, `TestRunWorktreesRemoveOrphan` `:108`, `TestRunWorktreesUsageErrors` `:124` (se conservan; se añaden los de `--orphans`/`--dry-run`).
  - /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor_test.go — `fakeProvisioner` `:354` (añadir `RemoveIfClean`); fakes de `Resolver` del paquete (añadir `ForgetReview`).
  - /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/retarget_test.go — `fakeLookup` `:523-532` (el doble del lookup sigue igual; B necesita un `ReviewRemover` aparte).
  - /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/merge_test.go — `TestMergeNoticeNamesTheMode` `:306` y el patrón `send(..., actionMsg{...})` `:311` son la base de los tests de B.
  - /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/audit_test.go — guardas `TestRemoveRefusesForeignWorktree` `:129`, `TestRemoveRefusesPathOutsideBase` `:148` deben seguir verdes.
- Framework / runner: Go testing. `make test` = `go build ./... && go vet ./... && gofmt -l (git ls-files '*.go') && go test -race ./...` (`Makefile:35-41`).
- Integration infra: **unit only**, sin red. Fixtures con git real vía `internal/testutil/git.go` (`InitRepo` `:28`, `CommitFile` `:40`, `RunGit` `:13`; también `InitBare`, `SetRemote`, `Push`). Adapters falsos: `testutil.FakeAdapter` (`internal/testutil/testutil.go`). Cache TUI aislada por `XDG_CACHE_HOME` en `newTestModel`.
- Suggested targets:
  - `--orphans` (behavior §A) → `cmd/prdash/worktrees_test.go` con `worktreeRepoFixture` ampliado (dos huérfanos quitando el repo, uno sano, uno ajeno) + `captureStderr` nuevo.
  - Excluyente / flag desconocido / sin rutas → exit 2 por stderr (`captureStderr`).
  - `RemoveIfClean` (limpiO/sucio/untracked/status-error/ausente/guardas) → `internal/worktree/worktree_test.go` + `audit_test.go`.
  - `RemoveReview` (activo⇒remove+forget; conservado⇒no forget; sin review⇒`(false,"",nil)`) → `executor_test.go`.
  - B (merge OK borra y compone; sucio conserva; no dispara con approve/retarget/fallo/conflicto/permiso/no-mergeable; sin review ⇒ sin error; `DeleteMsg` coexiste; quit no borra) → `internal/tui/merge_test.go` / `section_test.go`.

## Conventions & Boundaries
- Comentarios en **español**, identificadores en **inglés**; el código es la fuente de verdad (sin referencias a specs/IDs).
- `internal/worktree` **no** puede importar `internal/tui` ni `internal/review/executor` (hoy no lo hace: solo importa `gitcmd`, y `herdr` en `herdr.go`).
- `internal/tui` **ya** importa `internal/config` e `internal/review/executor` (`app.go:17-23`) y `internal/worktree` (`retarget.go:25`) ⇒ el puerto `ReviewRemover` puede hablar de `model.Item` sin ciclo nuevo.
- `internal/forge` **no** debe conocer worktrees: B se engancha en la TUI sobre el `Outcome` existente; cero cambios en `forge`.
- `--print` no cambia (no usa `newRefLayout` ni el modo; guard en `print_test.go:168`).
- TUI no toca red ni disco en el update loop: B corre en goroutine (patrón `launchAction`).
- No se añade estado al schema TOML; `cache.ReviewRecord` es memoria persistida, no config.

## Integration Points (non-obvious)
- **`removeWorktrees` ya recorre todas las rutas** (`cmd/prdash/worktrees.go:84`): el único hueco real de A es el flag. Un token `--orphans` hoy se trata como ruta y se rechaza (`:86`) — de ahí la necesidad de parsear antes.
- **`applyAction` tiene `return` tempranos** en `DeleteMsg` (`update.go:182-186`) y en retarget desfasado (`:190-194`): un `setNotice` de B colocado después quedaría saltado o pisaría esos hechos. B debe arrancar el borrado **antes/independiente** de esos returns y volver con `reviewCleanupMsg` que **compone** sobre el aviso base capturado al disparar.
- **"Merge OK pero la rama no se borró" (`out.DeleteMsg`) debe convivir** con "worktree removed/kept": son dos verdades distintas del mismo merge (escenario `el aviso del merge conserva todas las verdades a la vez`).
- **`Audit` marca huérfano ante cualquier error de `os.Stat`** (`audit.go:62-65`, `sourceReachable:96-103`), no solo "no existe" (falso-huérfano por gitdir inaccesible). Por eso `--dry-run` es obligatorio en el plan aunque `behavior.feature` no lo liste como escenario: mitiga lo irreversible sin tocar `Audit`.
- **`main.go` intercepta `worktrees` antes de `flag.Parse`** (`main.go:28`): el parseo de `--orphans`/`--dry-run` debe ser local a `remove` (`parseRemoveArgs`), nunca flags globales.
- **`mainRepoOf`/`linkedGitDir`** (`worktree.go:201-208` / `audit.go:75-92`): `Remove` resuelve el repo desde el propio worktree (no de memoria); un huérfano no tiene repo, y `Remove` ya lo maneja (`worktree.go:119-127`). `RemoveIfClean` no debe asumir repo presente.
- **`worktree_test.go`/`audit_test.go` crean repos git reales** con `testutil`: `RemoveIfClean` "limpio" puede crearse un worktree real y borrarlo; "sucio" escribir un fichero sin commitear; "untracked" un fichero nuevo (que `git diff --quiet` **no** detecta ⇒ usar `status --porcelain`).
- **`removeWorktrees` no llama `os.Exit`**: los tests lo invocan en proceso (`runWorktrees` devuelve `int`). Meter un `os.Exit` mataría la suite.
- **`HerdrNative.Remove` delega en `h.scan.Remove`** cuando no encuentra workspace (`herdr.go:187`): `RemoveIfClean` nativo debe preguntar el estado por `h.scan` (git real) y borrar por el nativo, sin saltarse el candado.

## Risks / Assumptions
- **`--dry-run` ya está cubierto por `behavior.feature`** (tres escenarios: muestra el lote exacto sin borrar, cero huérfanos ⇒ exit 0, y `--dry-run` sin `--orphans` ⇒ error de uso) además de por `plan.md` (decisión 8). No hay hueco de alcance: el executor lo deriva de los escenarios.
- **Fakes acoplados**: añadir un método a `Provisioner` y otro a `Resolver` obliga a tocar `fakeProvisioner` (`executor_test.go:354`) y los fakes de `Resolver`; trabajo mecánico, no riesgo. Cualquier `Provisioner` de test en `internal/worktree/herdr_test.go` (`fakeRunner` `:19` es del puerto Herdr, no del `Provisioner`) no se ve afectado.
- **Ventana TOCTOU** entre `status --porcelain` y `Remove`: el candado no es atómico a nivel git. Aceptado (fail-safe conserva ante la duda).
- **Registro residual en A**: `--orphans` (CLI) no tiene el `Store` a mano y deja el `ReviewRecord` que hubiera; se documenta, no se ensancha el CLI. Solo B olvida el registro (y solo si `removed==true`).
- **Aviso compuesto**: si el resultado de la limpieza llega después de otro aviso, debe recomponer sobre el **base del merge** (capturado al disparar), no sobre el aviso vigente.
- **`staleReviewNotice` vs B**: tras borrar de verdad, `ForgetReview` limpia el registro para que `ActiveReview` deje de mentir; sin él, el aviso de "base desfasada" seguiría apuntando a un checkout inexistente.
- **Freshness**: context generado contra `925270001710278918c82eac8443603e4aca9b85`; si el executor detecta que los símbolos/líneas ya no cuadran, debe emitir `CONTEXT_STALE` en vez de re-descubrir.
