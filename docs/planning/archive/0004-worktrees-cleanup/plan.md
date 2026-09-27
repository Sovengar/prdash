# 0004 — Limpieza de worktrees: `--orphans` en lote y auto-borrado al mergear — Plan

adr_required: true
adr_reason: B introduce la **primera eliminación implícita** de prdash que no nace de una ruta escrita por el usuario. Sustituye de forma estrecha la postura documentada "al cerrar la app los worktrees se conservan: no hay borrado implícito" (`cmd/prdash/worktrees.go:3-5`, README §Gestión de worktrees) por una excepción justificada en un hecho observable ("el PR se mergeó desde prdash") **más** una política duradera para el caso sucio (borrar solo si limpio; conservar y avisar si hay cambios). Es una decisión con alternativas reales (borrar con `--force` vs solo si limpio vs reportar la ruta) y con riesgo de pérdida de datos, no un refactor.
adr_title: adr-0007-worktree-cleanup-on-merge
adr_path: docs/adr/0007-worktree-cleanup-on-merge.md
adr_note: el ADR 0007 acota la excepción implícita (disparador `ActionMerge` + `OK`, solo desde prdash), fija el candado "solo si limpio" y la separación `Remove` (rutas explícitas, `--force`, sin candado) vs `RemoveIfClean` (B). No reescribe el ADR 0001 (provisión): lo complementa con la política de borrado. Se redacta en el planning y el executor lo mantiene coherente.

## Resultado esperado

Dos caminos de limpieza, y solo dos, para worktrees de review que ya son basura
con certeza:

1. **`prdash worktrees remove --orphans`** borra **en lote** todos los worktrees
   que el propio `Audit` marca como huérfanos, y **solo** esos. `--orphans` es
   **excluyente** con las rutas explícitas. Con cero huérfanos informa y sale con
   **0**. Un `--dry-run` opcional imprime el lote exacto que se borraría y no
   borra nada.
2. **Al mergear desde prdash**: cuando una acción `merge` lanzada por prdash
   termina bien, se borra el worktree de ese ítem **solo si está limpio**. Si hay
   cambios sin commitear (incluidos archivos sin trackear) o **no se puede
   comprobar** el estado, se **conserva** y el aviso lo dice:
   `merged, but the worktree has uncommitted changes — kept`.

Nada más cambia: **cerrar la app no borra nada**, sigue sin haber acción de
cierre, y los guardas de ownership (`worktree.Owned`) y de raíz gestionada
(`removablePath`) siguen siendo intocables para toda vía de limpieza.

## Alcance

- **In**: flag `--orphans` y `--dry-run` en `remove`; candado "solo si limpio" y
  su fail-safe; puerto de borrado del review para la TUI; limpieza del registro
  del review activo al borrar; ADR 0007; README y CHANGELOG.
- **Out**: borrado al cerrar la app (restricción dura); acción "close" nueva;
  cambios en la definición de huérfano de `Audit` y en la semántica de `Remove`
  (rutas explícitas); cambios en los guardas de ownership; estado nuevo en el
  schema TOML (el `DeleteReview` del cache **no** es schema de config); el script
  del orquestador y el artefacto no versionado `.codegraph/` (fuera de prdash).
- **Out (explícito)**: no se borra el worktree de un PR que sale mergeado
  **fuera** de prdash (el refresco que lo ve mergeado **no** dispara limpieza).

## Enfoque (alto nivel)

### A — el flag vive en el subcomando, no en el parser global

`main.go` intercepta `worktrees` **antes** del `flag.Parse` global, así que
`--orphans`/`--dry-run` se parsean **dentro** de `remove`, con un helper puro
`parseRemoveArgs(args)` que no toca `os.Args` y es testeable aislado. El lote sale
de la **misma** fuente de verdad que ya alimenta `list`: `pr.Audit(ctx)` filtrado
por `Entry.Orphan`. `Remove` ya sabe borrar un huérfano (repo de origen
desaparecido → borra el checkout), así que A **no abre una vía de borrado nueva**:
reusa la existente, solo automatiza la selección.

**Mitigación del falso huérfano (sin tocar `Audit`)**: `Audit` marca huérfano ante
**cualquier** error de `os.Stat` sobre el gitdir, no solo "no existe"; un gitdir
temporalmente inaccesible (montaje de red, permisos, otro usuario) marcaría sano a
huérfano. No se cambia esa semántica. El valle es un **`--dry-run` opcional** que
imprime el lote **desde el mismo camino de código** y no borra nada, de modo que la
operación irreversible se puede ver antes de ejecutarla. La clase de fallo queda
documentada en el ADR/README.

### B — el disparador ya existe; falta el puerto y el candado

El merge de prdash pasa por un embudo único: `applyAction(out forge.Outcome, cycle)`
(`internal/tui/update.go`), llamado desde `case actionMsg:`. El `Outcome` **ya**
trae todo lo necesario: `Kind == forge.ActionMerge` y `OK` (más el `Item` releído
post-acción). **B no necesita plumbing nuevo en el forge**: engancha ese embudo.
Lo que falta es:

- una **capacidad de borrado** que la TUI no tiene hoy (`ReviewLookup` es solo
  lectura): un puerto opcional **`ReviewRemover`** implementado por el `Executor`,
  que ya tiene el `Provisioner` inyectado;
- un **candado "solo si limpio"** en el nivel que conoce git (el provisioner),
  porque `Remove` es `--force` a propósito para las rutas explícitas de A y **no**
  debe cambiar de semántica;
- limpiar el **registro del review activo** cuando de verdad se borra, para que
  `ActiveReview` deje de reportar un review montado.

El borrado de B corre **en segundo plano** (subproceso de git; el handler de
`Update` no puede bloquear) y su resultado vuelve como su propio mensaje, que
compone el aviso final sin pisar los hechos que ya traía el merge.

## Decisiones clave

1. **Dos modos de borrado, dos métodos, un solo juego de guardas.**
   `Provisioner` gana `RemoveIfClean(ctx, id) (removed bool, reason string, err error)`.
   - `Remove` (rutas explícitas, A) se queda **exactamente** como está: borra
     aunque haya cambios; un huérfano ni siquiera tiene repo para preguntar.
   - `RemoveIfClean` (B) aplica el candado: primero `removablePath` (los mismos
     guardas), luego decide. **No** se mete el candado dentro de `Remove` ni una
     bandera booleana que cambie su significado.
   - **Dónde NO va**: no va en `executor` (su contrato es orquestar sin conocer
     git) ni en la TUI. El hecho de git vive en el paquete `worktree`.
2. **El candado pregunta por el árbol, no por el índice.**
   `git status --porcelain` (vía `internal/gitcmd`), no `git diff --quiet`: un
   archivo **nuevo sin trackear** es trabajo sin commitear y `diff` lo ignora.
   `strings.TrimSpace(salida) != ""` ⇒ sucio ⇒ conservar.
3. **Fail-safe en B: la duda conserva.** Si la comprobación de estado **falla**
   (index.lock, EACCES, gitdir que se movió entre `stat` y `status`), el resultado
   es `(removed=false, reason="could not read the worktree status")` y **no** un
   borrado. `err` se reserva para lo que sí es un fallo de infraestructura
   reportable; "no se pudo comprobar" es un motivo de conservación, no un error de
   la acción.
4. **B es idempotente y no inventa errores.** Si el ítem no tiene review montado,
   o la ruta del registro ya no existe en disco, el resultado es
   `(false, "", nil)`: el merge termina igual y **no** se reporta un error de
   limpieza. `Remove` no es idempotente hoy (una ruta ausente da error), así que
   `RemoveIfClean` comprueba antes de intentar.
5. **Puerto `ReviewRemover` nuevo, no extender `ReviewLookup`.**
   `ReviewLookup` está documentado como **solo lectura y degradable** ("sin él…
   lo único que se pierde es ese aviso", `internal/tui/retarget.go`). Una capacidad
   que **borra** no puede heredar ese contrato: necesita ausencia explícita ("sin
   remover no hay auto-borrado, y el merge sigue igual"). Firma:
   `RemoveReview(ctx, it) (removed bool, reason string, err error)`, implementada
   por `*Executor`: mapea ítem→worktree con `ActiveReview`, llama a
   `Worktrees.RemoveIfClean`, y **solo si borró** olvida el registro.
   Se inyecta con `SetReviewRemover(ex)` junto al `SetReviewLookup(ex)` existente
   en `main.go`. Sin remover inyectado, B se degrada a "no auto-borra".
6. **El registro del review activo se limpia al borrar de verdad.**
   `cache.ReviewRecord` es persistente; dejarlo apuntando a un checkout que ya no
   existe haría que `ActiveReview` mintiera para siempre (avisos de "base
   desfasada", `--print`, remontajes). Se añade `Resolver.ForgetReview(it)` +
   `cache.Store.DeleteReview(key)`, llamados **solo** cuando `removed==true`. Es un
   borrado de un registro, **no** estado nuevo de config (nada toca el schema TOML).
   Residual aceptado: el camino A (`--orphans`) no tiene el store a mano y deja el
   registro que hubiera; se documenta, no se ensancha el wiring del CLI.
7. **Aviso compuesto, nunca sobrescrito.** `applyAction` ya hace `return` temprano
   en el caso `DeleteMsg` (rama no borrada) y en el caso de retarget desfasado.
   B no puede ser un `setNotice` aparte que pise esos hechos: el borrado arranca
   como `tea.Cmd` en segundo plano y su resultado llega como mensaje propio
   (`reviewCleanupMsg`) que **compone** el aviso base del merge con el desenlace
   de la limpieza. Reglas:
   - borrado → `… ok · worktree removed` (nivel OK);
   - conservado por sucio/ilegible → `… ok · worktree kept: <motivo>` (nivel warn);
     el texto de sucio es literalmente `merged, but the worktree has uncommitted
     changes — kept`;
   - fallo de infraestructura → `… ok · could not remove the worktree: <err>` (warn,
     **no** error: el merge sí salió).
   Con `DeleteMsg` presente, los tres hechos conviven en el mismo aviso.
8. **`--orphans` excluyente y `--dry-run` solo con `--orphans`.**
   `parseRemoveArgs(args) (orphans, dryRun bool, paths []string, err error)`:
   `--orphans` + rutas ⇒ error de uso; `--dry-run` sin `--orphans` ⇒ error de uso;
   token que empieza por `-` y no se reconoce ⇒ error de uso; sin `--orphans` ni
   rutas ⇒ error de uso (comportamiento actual). Todo error de uso: mensaje por
   **stderr**, **exit 2**, y **cero** borrados. Éxito (incluido "cero huérfanos")
   por **stdout**, **exit 0**. Fallo de borrado: **exit 1**.
9. **Sin `os.Exit` dentro de `runWorktrees`/`removeWorktrees`.** Devuelven `int` y
   `main.go` sale; los tests los conducen en proceso. Meter un exit mataría la
   suite. Se conserva esa costura.
10. **ADR 0007, sin editar ADRs aceptados.** El 0007 acota la excepción implícita,
    declara qué decisión de conservación sustituye y la política del caso sucio.
    Encadena con el 0001 (provisión) sin reescribirlo. Se redacta en el planning.

## Ficheros afectados (alto nivel)

**Producción**

- `internal/worktree/worktree.go`: `RemoveIfClean` en la interfaz `Provisioner` y en
  `GitDirect`; helper `dirty` (`status --porcelain`); `Remove` **sin cambios**.
- `internal/worktree/herdr.go`: `HerdrNative.RemoveIfClean` (mismo candado sobre
  `h.scan` y delegando en su `Remove` nativo, para no dejar el workspace huérfano).
- `internal/review/executor/executor.go`: `RemoveReview` en el `Executor`;
  `ForgetReview` en la interfaz `Resolver`.
- `internal/reporesolver/reporesolver.go` + `internal/cache/memo.go`:
  `ForgetReview` / `DeleteReview` (borrado del registro persistido).
- `internal/tui/retarget.go` (o `review_remover.go`): puerto `ReviewRemover` +
  `SetReviewRemover`; campo en el `Model` (`app.go`).
- `internal/tui/app.go`: campo `reviewRemover`; tipo `reviewCleanupMsg`.
- `internal/tui/update.go`: en `applyAction`, si `merge` + `OK` lanzar el borrado en
  segundo plano; nuevo `case reviewCleanupMsg` que compone el aviso.
- `cmd/prdash/worktrees.go`: `parseRemoveArgs`; `removeWorktrees` con `--orphans`
  (y `--dry-run`) además de las rutas explícitas.
- `cmd/prdash/main.go`: `model.SetReviewRemover(ex)`.
- `docs/adr/0007-worktree-cleanup-on-merge.md`: nuevo (redactado en el planning).
- `README.md`: `remove <ruta>` → `remove <ruta>…`, documentar `--orphans`,
  `--dry-run`, y **cuándo** borra B (merge desde prdash, solo si limpio, con aviso)
  sin contradecir la regla de conservación al cerrar.
- `CHANGELOG.md`: `## [Unreleased] → ### Added`.

**Tests**

- `internal/worktree/worktree_test.go`: `RemoveIfClean` limpio borra; sucio conserva
  (con la razón); **untracked** conserva; `status` en error conserva (fail-safe);
  ruta ausente ⇒ no-op sin error; guardas (ajeno / fuera de `Base`) siguen
  rechazando.
- `internal/review/executor/executor_test.go`: `RemoveReview` con review activo →
  `RemoveIfClean` + `ForgetReview`; conservado ⇒ **no** olvida; sin review ⇒
  `(false,"",nil)`; actualizar `fakeProvisioner`/fakes de `Resolver`.
- `internal/tui/section_test.go` / `app_test.go`: merge OK borra (remover fake
  invocado con el ítem correcto); sucio conserva y avisa con el texto exacto; no
  dispara con approve/retarget/fallo/conflicto/permiso/no-mergeable; sin review
  ⇒ sin error; aviso compuesto con `DeleteMsg`; **cerrar la app no invoca el
  remover**.
- `cmd/prdash/worktrees_test.go`: `--orphans` borra todos y solo huérfanos; sanos y
  ajenos intactos; cero huérfanos ⇒ exit 0 y stdout sin stderr; **`--dry-run`
  imprime el lote y no borra**; `--orphans <ruta>` y `--dry-run` sin `--orphans` y
  flag desconocido ⇒ exit 2 sin tocar nada; rutas explícitas siguen igual (se
  conservan los tests actuales). Puede requerir un helper `captureStderr`.

## Orden de trabajo (TDD, grueso)

1. **Tests primero, por bloques**: `worktree` (`RemoveIfClean`) → `cmd/prdash`
   (`parseRemoveArgs` + `removeWorktrees`) → `executor` (`RemoveReview`) → `tui`
   (disparador y aviso).
2. **Wiring**: interfaz `Provisioner` (+`HerdrNative`, +fakes); `Resolver.ForgetReview`
   (+cache, +fakes); `Executor.RemoveReview`; puerto `ReviewRemover` + `SetReviewRemover`
   + `main.go`; `applyAction` + `reviewCleanupMsg`.
3. **Docs**: ADR 0007, README, CHANGELOG.
4. `make test` y smoke manual.

## Verificaciones

- `make test` (build + vet + gofmt + `go test -race ./...`) **en verde**. Es la única
  garantía: el repo no tiene CI.
- Smoke CLI: `worktrees list` marca huérfanos; `remove --orphans --dry-run` imprime
  el lote y no borra; `remove --orphans` borra solo esos; un sano y un ajeno siguen;
  `remove --orphans <ruta>` y `--bogus` salen con 2.
- Smoke TUI: montar un review limpio y mergear → worktree fuera y aviso "removed";
  montar con un archivo sin commitear y mergear → worktree conservado y aviso
  "merged, but the worktree has uncommitted changes — kept"; `q` no borra nada.
- Coherencia de ADRs: el 0007 declara qué cláusula sustituye; 0001 intacto.

## Riesgos

- **Falso huérfano** (mitigado, no eliminado): la definición de `Audit` no distingue
  "gitdir ausente" de "inaccesible". `--dry-run` da el lote antes de borrar y
  documenta la clase; cambiar `Audit` queda fuera de alcance.
- **Ventana TOCTOU** entre `status` y `remove` dentro del mismo goroutine: el
  candado no es atómico a nivel de git. Aceptable: reduce el borrado a "limpio en el
  momento de comprobar" y el fail-safe conserva ante la duda.
- **Registro residual en A**: `--orphans` no limpia `cache.ReviewRecord`; se
  documenta y no se ensancha el CLI.
- **Aviso compuesto**: si el resultado de la limpieza llega después de otro aviso,
  debe re-componer sobre el **base del merge**, no pisar el aviso vigente; lo fija
  el mensaje propio con el `base` capturado al disparar.
- **Fakes acoplados**: añadir un método a `Provisioner` y otro a `Resolver` obliga a
  actualizar los fakes de `executor`/`tui`; es trabajo mecánico, no riesgo.
