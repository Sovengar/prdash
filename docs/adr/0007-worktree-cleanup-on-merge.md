# ADR 0007 — limpieza de worktrees: huérfanos en lote y auto-borrado al mergear

- **Estado**: Accepted
- **Fecha**: 2026-09-27
- **Decisor**: usuario (buble)
- **Alcance**: subcomando `prdash worktrees`, `internal/worktree/`,
  `internal/review/executor/`, `internal/tui/`
- **Depende de**: ADR 0001 (provisión del worktree de review).
- **Sustituye a**: la cláusula de conservación total de `cmd/prdash/worktrees.go`
  ("al cerrar la app los worktrees se conservan: no hay borrado implícito"), que
  pasa a ser "no hay borrado por cerrar la app; sí hay borrado por un hecho
  observable" (ver Decisión 2).

## Contexto

Los worktrees de review son **del usuario**: existen para editar el PR dentro de
ellos, así que prdash los conserva a propósito y no borra nada al cerrar la app.
Esa decisión sigue siendo correcta y **no se toca**. Pero deja dos huecos de
higiene, ambos acotados a worktrees que ya son basura con certeza:

1. **Limpiar huérfanos requiere enumerar rutas a mano.** Cuando el repo de origen
   desaparece, el worktree queda huérfano y `worktrees list` ya lo reporta, pero
   borrar varios exige recordar cada ruta.
2. **El worktree de un PR mergeado desde prdash se queda en disco.** Si el PR se
   mergea **a través de prdash**, su worktree ya es basura: el review terminó.

Dos hechos verificados condicionan la decisión:

- **prdash no tiene acción de cierre.** Solo existen `approve`, `merge` y
  `retarget`. "Cerrado desde prdash" es hoy inalcanzable; el único hecho observable
  de "el PR ya no existe" que prdash puede provocar es un merge que sale bien.
- **`git worktree remove --force` destruye los cambios sin commitear.** Un merge
  exitoso dice que el PR terminó, **no** que el checkout esté limpio.

## Decisión

1. **A — `prdash worktrees remove --orphans`** borra en lote **solo** las entradas
   que el propio `Audit` marca como huérfanas (misma fuente de verdad que
   `worktrees list`), y es **excluyente** con las rutas explícitas. Cero huérfanos
   es el caso feliz (exit 0). La definición de huérfano **no** se toca.
2. **B — auto-borrado al mergear desde prdash.** Cuando una acción `merge`
   lanzada por prdash termina bien (`ActionMerge` + `OK`), se borra el worktree
   de ese ítem **solo si está limpio**. Es la primera eliminación implícita que no
   nace de una ruta escrita por el usuario; se justifica en un hecho observable
   (el merge lo hizo prdash) y no en el cierre de la app.
3. **Candado "solo si limpio"**, con fail-safe: sucio (incluidos archivos sin
   trackear) ⇒ **se conserva** y se avisa
   (`merged, but the worktree has uncommitted changes — kept`). Si el estado **no
   se puede comprobar**, también se conserva. Ante la duda, nunca se borra.
4. **Dos modos de borrado, no una bandera.** `Remove` (rutas explícitas, `--force`,
   sin candado) y `RemoveIfClean` (B, con candado) son métodos distintos. El
   candado no se mete dentro de `Remove` ni en un booleano que cambie su
   significado: A y B tienen justificaciones distintas y deben poder leerse por
   separado.
5. **Los guardas no se relajan.** Toda vía de limpieza pasa por `worktree.Owned`
   (label o nombre con prefijo `prdash-`) **y** por vivir bajo la raíz gestionada.
   Un worktree ajeno es intocable por ruta explícita, por `--orphans` y por B.
6. **`--dry-run` para `--orphans`.** `Audit` marca huérfano ante **cualquier**
   error de `os.Stat`, no solo "no existe", así que un gitdir temporalmente
   inaccesible podría marcar sano a huérfano. No se cambia `Audit`: se ofrece
   imprimir el lote exacto, desde el mismo camino de código, sin borrar nada.
7. **Registro del review activo.** Cuando B borra de verdad, olvida el
   `cache.ReviewRecord` de ese ítem; si no, `ActiveReview` mentiría para siempre.
   Es un borrado de registro, no estado nuevo de configuración.

## Alternativas consideradas y descartadas

- **Borrar siempre con `--force` al mergear (sin candado).** Simple y
  determinista, pero destruye cambios no commiteados sin avisar: es exactamente el
  escenario que la regla de conservación intenta evitar. Descartada.
- **Borrar siempre y reportar la ruta para "recuperar".** Mantiene el automatismo,
  pero `--force` borra el checkout: reportar la ruta no recupera lo no commiteado y
  el mensaje promete algo que puede no cumplirse. Descartada por poco honesta.
- **B como confirmación (pulsar para borrar tras el merge).** Elimina el riesgo de
  pérdida, pero reintroduce el paso manual que B existe para quitar y añade estado
  a la TUI. Descartada.
- **`remove` sin argumentos = huérfanos.** Cómodo, pero rompe el contrato de rutas
  explícitas y es peligroso por defecto. Descartada; el flag nombra la intención.

## Consecuencias

**Positivas**
- La limpieza de huérfanos pasa de "una ruta por vez, a mano" a un gesto, con una
  previsualización (`--dry-run`) previa a lo irreversible.
- El worktree de un PR mergeado desde prdash no se queda ensuciando el listado, y
  el caso con trabajo sin commitear se conserva con un aviso explícito.
- La separación `Remove` / `RemoveIfClean` deja cada justificación de borrado en su
  propio método, sin cambiar la semántica que ya usaban las rutas explícitas.

**Negativas / costes**
- Aparece una eliminación implícita. Se acota a un hecho observable (merge por
  prdash) y a un checkout limpio; queda registrada aquí para que no se generalice.
- El caso sucio deja un worktree en disco que el usuario limpia a mano (con aviso).
- La detección de huérfano no distingue "ausente" de "inaccesible": `--dry-run` la
  mitiga, no la elimina.
- En A, el registro persistido del review puede quedar residual (el CLI no tiene el
  store a mano); se documenta y no se ensancha el wiring.

**Verificación (a cerrar con `make test`)**
- `RemoveIfClean` borra limpio, conserva sucio/untracked/ilegible y no es un error
  si la ruta ya no existe; los guardas siguen rechazando ajeno y fuera de la raíz.
- `remove --orphans` borra todos y solo huérfanos; `--dry-run` no borra;
  excluyente con rutas; cero huérfanos ⇒ exit 0.
- Merge OK desde prdash borra el worktree limpio y compone el aviso; no dispara con
  approve/retarget/fallo/conflicto/permiso ni al cerrar la app; no dispara con un PR
  mergeado fuera de prdash.
