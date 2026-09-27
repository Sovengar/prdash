# 0004 — Limpieza de worktrees: `--orphans` en lote y auto-borrado al mergear desde prdash (feature)

## Problema

Los worktrees de review son **del usuario**: existen para editar el PR dentro de
ellos, así que prdash los conserva a propósito y **no borra nada de forma
implícita al cerrar la app** (`cmd/prdash/worktrees.go:3-5`). Esa decisión es
correcta y no se toca. Pero deja dos huecos de **higiene**, ambos acotados a
worktrees que ya son **basura con certeza** (huérfanos, o su PR ya no existe):

1. **Limpiar huérfanos de a uno.** Cuando el repo de origen desaparece, el
   worktree queda huérfano y `prdash worktrees list` ya lo reporta
   (`prdash: <ruta> is orphaned: …`, `audit.go:62-65`). Borrarlo hoy exige
   **enumerar rutas a mano**, una por una. Limpiar varios exige un script o
   recordar cada ruta. Falta un gesto: **"borrá todos los huérfanos"**.
2. **El worktree de un PR mergeado desde prdash.** Si el PR se mergea **a través
   de prdash**, su worktree es basura **con certeza**: el review terminó. Hoy se
   queda en disco, ocupando espacio y ensuciando el listado, hasta que alguien
   lo borre a mano. Como el disparador es "el PR ya no existe", borrarlo en ese
   instante **no viola** la regla de conservar al cerrar: no es un borrado
   implícito por salir, es un borrado justificado por un hecho observable.

El riesgo de A es **nulo** por construcción: los huérfanos no pueden volver. El
riesgo de B es **bajo** pero no nulo, y su forma concreta (trabajo sin commitear)
es la decisión abierta de esta issue.

## Alcance (in)

### A — `prdash worktrees remove --orphans`

Nuevo flag en el subcomando `remove` (`cmd/prdash/worktrees.go`). Cuando está
presente, borra **únicamente** las entradas que el propio `Audit` marca como
huérfanas:

- Fuente de verdad: `pr.Audit(ctx)` filtrando `Entry.Orphan` (`audit.go:23-31`,
  `audit.go:62-65`). No se reimplementa la detección: la misma que ya alimenta el
  reporte de `list`.
- Un huérfano se borra por la vía ya existente `pr.Remove(ctx, path)`, que para
  el caso huérfano ya sabe borrar el checkout directamente
  (`worktree.go:105-127`).
- **Solo huérfanos.** Un worktree propio y sano **nunca** se toca con
  `--orphans`.
- Con **cero huérfanos** termina con **exit 0** y un mensaje claro (no es un
  error: es el caso feliz).
- El borrado por **rutas explícitas** (`remove <ruta1> <ruta2> …`) sigue
  funcionando exactamente igual que hoy.

### B — auto-borrado al mergear desde prdash

Cuando una acción de merge lanzada **desde prdash** termina bien, se borra el
worktree de ese ítem. El punto único de observación es `applyAction` en
`internal/tui/update.go:154`, que ya recibe el `forge.Outcome` post-acción:

- Señal: `out.Kind == forge.ActionMerge && out.OK` (`forge.go:271-275`). Cualquier
  otra combinación (approve, retarget, fallo, conflicto, permiso denegado,
  `Unmergeable`) **no dispara** nada.
- Mapeo ítem → worktree: `Executor.ActiveReview(it)` ya existe
  (`executor.go:128`) y está expuesto a la TUI por el puerto `ReviewLookup`
  (`retarget.go:133`, inyectado con `SetReviewLookup`). El `Executor` ya tiene
  `Worktrees worktree.Provisioner` (`executor.go:50`), así que el borrado usa la
  misma vía que el comando CLI.
- Si el ítem **no tiene** worktree montado, no hay nada que borrar y no se
  reporta error: mergea y listo.
- El aviso de la acción (`actionDoneNotice`) sigue saliendo; si además se borra
  el worktree, se informa en el mismo notice.

## No-alcance (out)

- **NO borrar al cerrar la app.** Es la restricción dura (`worktrees.go:3-5`,
  README:493-494). Ningún camino de esta feature toca `quit`/teardown. El único
  borrado implícito admitido es el de un worktree **probadamente muerto**
  (huérfano, o PR mergeado **desde prdash**).
- **NO crear una acción "close".** `forge.go:270-275` solo define
  `ActionApprove`, `ActionMerge` y `ActionRetarget`: prdash **no puede cerrar**
  un PR. "Cerrado desde prdash" es hoy **inalcanzable** (ver Corrección al
  enunciado); B se reduce a "mergeado desde prdash".
- **NO tocar los guardas de ownership/seguridad.** Se mantienen tal cual:
  `worktree.Owned` (`audit.go:19-21`) y la exigencia de que la ruta viva bajo la
  raíz gestionada `Base` (`removablePath`, `worktree.go:130-143`). Ningún
  worktree ajeno se lista ni se borra.
- **NO borrar con `--force` un worktree sucio como default incuestionado.** El
  `git worktree remove --force` (`worktree.go:116`) destruye cambios sin
  commitear; si eso aplica a B lo decide la **Decisión ABIERTA**, no esta
  sección.
- **NO cambiar la semántica de `remove <ruta>`** ni el contrato de
  `Provisioner.Audit`/`Remove`.
- **NO persistir** ningún estado nuevo ni tocar el schema TOML.
- **NO tocar `list`** más allá de convivir con el nuevo flag.

## Criterios de aceptación

- [ ] `prdash worktrees remove --orphans` borra **todos** los huérfanos que
      `Audit` reporta y **solo** huérfanos.
- [ ] Con `--orphans`, todo worktree propio **sano** (no huérfano) sigue en disco
      y `Audit` lo sigue listando tras el comando.
- [ ] Con `--orphans`, todo worktree **ajeno** (label/ruta que no empieza por
      `prdash-`, o fuera de `Base`) sigue intacto.
- [ ] `--orphans` con **cero huérfanos** termina con **exit 0** y un mensaje
      claro; no imprime error ni código distinto de 0.
- [ ] `prdash worktrees remove <ruta1> <ruta2>` (rutas explícitas) sigue
      funcionando igual que hoy, incluidos los rechazos actuales (ajeno/inexistente
      → no se toca, exit 1).
- [ ] Tras un **merge exitoso desde prdash**, el worktree del ítem desaparece y
      la TUI lo refleja (el ítem ya no tiene review montado).
- [ ] El auto-borrado de B **no dispara** con: `approve`, `retarget`, merge
      **fallido**, merge **bloqueado por permiso**, merge con **conflicto**, ni
      `Unmergeable`.
- [ ] El auto-borrado de B **no dispara** al **cerrar la app** (se puede
      verificar que quitar todos los caminos de teardown no borra nada).
- [ ] Si el ítem mergeado **no tenía** worktree montado, el merge termina igual y
      **sin error**.
- [ ] Los guardas se preservan: un `Remove` sobre un worktree **ajeno** o **fuera
      de `Base`** sigue siendo imposible (los tests de `removeRefusesForeign`
      siguen valiendo).
- [ ] `make test` en verde (build + vet + gofmt + `go test -race`).
- [ ] README (`490-500`) y CHANGELOG sin contradicciones: se documenta
      `remove --orphans`, se corrige `remove <ruta>` → `remove <ruta>…` y se
      explica cuándo B borra y cuándo no.

## Decisión ABIERTA (bloquea el plan) — el borrado de B vs trabajo sin commitear

`GitDirect.Remove` llama a `git worktree remove --force` (`worktree.go:116`): si
el worktree tiene **cambios sin commitear**, borrarlo los **destruye**. La postura
del propio código es que el worktree es **del usuario** y puede alojar trabajo sin
commitear (aviso `staleReviewNotice` en `update.go:187-194` y README:129). Un merge
exitoso desde prdash dice que **el PR terminó**, no que el checkout esté limpio.
Hay que elegir qué hace B cuando el worktree está sucio:

- **A — borrar igual con `--force`.** Simple y determinista: "mergeado desde
  prdash ⇒ worktree fuera". Coste: **puede destruir cambios no commiteados sin
  avisar**; es exactamente el escenario que la regla de conservar al cerrar
  intenta evitar.
- **B (recomendada) — borrar solo si está limpio; si hay cambios, avisar y
  conservar.** B respeta la regla de conservar ante la duda y solo automatiza el
  caso sin pérdida. Coste: hay que **detectar el estado sucio** (p. ej.
  `git status --porcelain`/`git diff --quiet`) antes de borrar, y el worktree sucio
  queda en disco con un aviso ("merged, but the worktree has uncommitted changes —
  kept"), lo que el usuario tiene que limpiar a mano.
- **C — borrar igual, pero reportar la ruta para poder recuperar.** Mantiene la
  automatización y da una pista. Coste: `--force` borra el **checkout**; reportar
  la ruta **no recupera** lo no commiteado (el mensaje promete una recuperación que
  puede no existir), así que es la peor de las tres en confianza.

**Recomendación: B.** Es la única que cumple el invariante documentado ("el
worktree es del usuario") sin renunciar al automatismo en el caso frecuente (PR
mergeado, checkout limpio). El tradeoff es un caso residual sucio que queda en
disco con aviso; es preferible un worktree de más que trabajo perdido.

**Pregunta para el usuario**: cuando mergeás un PR desde prdash y su worktree tiene
**cambios sin commitear**, ¿qué hacemos con el worktree? **(A)** borrarlo igual
aunque se pierdan esos cambios, **(B)** borrarlo solo si está limpio y, si hay
cambios, avisar y conservarlo (recomendada), o **(C)** borrarlo igual y mostrar la
ruta para que intentes recuperar lo que quieras.

## Corrección al enunciado (no bloquea, pero no se documenta falso)

1. **A no arregla "acepta una sola ruta".** `removeWorktrees(pr, args[1:])` **ya
   itera sobre todas las rutas** (`worktrees.go:74-98`): `prdash worktrees remove
   <ruta1> <ruta2> …` **funciona hoy**. Lo único que miente es el README
   (`499: remove <ruta>`, singular). Lo genuinamente ausente es el flag
   `--orphans`: tal cual, un token `--orphans` se trataría como una ruta y se
   rechazaría (`worktrees.go:85-90`). Y la cobertura ya existe: `TestRunWorktreesRemoveOwned`,
   `TestRunWorktreesRemoveOrphan` y `TestRunWorktreesRemoveRefusesForeign`
   (`cmd/prdash/worktrees_test.go`).
2. **B no es "cerrado o mergeado desde prdash": es solo "mergeado".** prdash **no
   tiene acción de cierre** (`forge.go:270-275`: `ActionApprove`, `ActionMerge`,
   `ActionRetarget`), así que "cerrado desde prdash" es **inalcanzable** hoy.
   `state.Derive` sí mapea a `StateMerged`/`StateClosed` (`state.go:86-91`), pero
   `StateClosed` describe un PR cerrado **en el forge**, no una acción de prdash.
   La señal observable de B es `out.Kind == forge.ActionMerge && out.OK`. Añadir un
   close action queda **fuera de alcance**.

## Riesgos / verificaciones pendientes

- **Pérdida de trabajo sin commitear (B).** Es el riesgo central; lo resuelve la
  Decisión ABIERTA. Cualquiera de las opciones debe quedar **documentada** en el
  README junto al aviso de conservación.
- **Falso positivo de "mergeado"**: el notice de B se apoya en `out.OK` +
  `ActionMerge`. Verificar que un merge que **devuelve OK pero deja la rama sin
  borrar** (`out.DeleteMsg`, `update.go:182-186`) no se confunda: borrar el
  worktree sigue siendo correcto, pero el aviso debe decir las dos cosas.
- **`out.Item` re-leído**: `applyAction` recibe el ítem post-acción
  (`update.go:156-164`); hay que mapear el worktree con el mismo `Item` que
  `ActiveReview` espera para no borrar el worktree equivocado.
- **Puerto `ReviewLookup` es solo-lectura hoy** (`retarget.go:133`): exponer el
  borrado a la TUI requiere añadir un método (o un puerto nuevo) sobre el
  `Executor`, que ya tiene `Worktrees` (`executor.go:50`). No se inventa una vía
  nueva de borrado: se reusa `Provisioner.Remove`.
- **Huérfano vs sano en `--orphans`**: la detección depende de
  `sourceReachable` (`audit.go:94-103`). Un gitdir temporalmente inaccesible
  marcaría huérfano un worktree sano. `--orphans` borraría ese checkout. Verificar
  que la definición de huérfano (gitdir inexistente tras `os.Stat`) es lo bastante
  estable, o acotar el riesgo en el plan (p. ej. `--dry-run`/confirmación), sin
  cambiar la semántica de `Audit`.
- **`HerdrNative`**: su `Audit` delega en el escaneo `GitDirect`, así que un
  huérfano se detecta igual dentro y fuera de Herdr; confirmar que `Remove` se
  comporta igual en ambos entornos.
- **Exclusividad de flags (micro-decisión, no bloquea)**: si `--orphans` convive con
  rutas explícitas o es excluyente. Propuesta: `--orphans` es **excluyente** (o
  ignora rutas) para que "solo huérfanos" sea una garantía legible; se fija en el
  plan.
- **Tests a añadir**: `--orphans` borra todos y solo huérfanos; cero huérfanos →
  exit 0 con mensaje; `--orphans` no toca sanos ni ajenos; B dispara con
  merge OK y no con approve/retarget/fallo/conflicto; B no dispara en teardown;
  B con worktree sucio según la decisión elegida.
