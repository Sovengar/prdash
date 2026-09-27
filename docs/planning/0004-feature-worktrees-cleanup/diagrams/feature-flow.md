# Flujo de comportamiento — 0004 limpieza de worktrees

Derivado de `../behavior.feature`. No es Cucumber: es una vista del
comportamiento esperado para revisión humana.

## A — `prdash worktrees remove --orphans`

```mermaid
flowchart TD
  A0["prdash worktrees remove ..."] --> A1{"uso válido?<br/>--orphans con rutas<br/>--dry-run sin --orphans<br/>flag desconocido"}
  A1 -- "no" --> A2["stderr + exit 2<br/>NO borra nada"]
  A1 -- "sí" --> A3["Audit() → filtrar Entry.Orphan<br/>(misma fuente que 'worktrees list')"]
  A3 --> A4{"¿cuántos huérfanos?"}
  A4 -- "0" --> A5["stdout: 'no orphaned worktrees'<br/>exit 0 (caso feliz)"]
  A4 -- "N" --> A6{"¿--dry-run?"}
  A6 -- "sí" --> A7["imprime el lote exacto<br/>NO borra · exit 0"]
  A6 -- "no" --> A8["Remove() por huérfano<br/>guardas: Owned + bajo raíz gestionada"]
  A8 --> A9["exit 0<br/>(1 si falla algún borrado)"]

  A8 -. "nunca toca" .-> AX1["worktree propio sano"]
  A8 -. "nunca toca" .-> AX2["worktree ajeno"]
```

## B — auto-borrado al mergear **desde prdash**

```mermaid
flowchart TD
  B0["acción merge lanzada por prdash"] --> B1{"out.Kind == ActionMerge<br/>&& out.OK"}
  B1 -- "no: approve, retarget,<br/>fallo, conflicto, permiso,<br/>no-mergeable" --> B2["no toca ningún worktree"]
  B1 -- "sí" --> B3["ActiveReview(item)<br/>mapea ítem → worktree"]
  B3 --> B4{"¿hay review montado<br/>y existe la ruta?"}
  B4 -- "no" --> B5["no-op sin error<br/>el merge sigue ok"]
  B4 -- "sí" --> B6{"git status --porcelain<br/>¿árbol limpio?"}
  B6 -- "sucio (incluye untracked)" --> BKEEP["CONSERVA<br/>'merged, but the worktree<br/>has uncommitted changes — kept'"]
  B6 -- "estado ilegible" --> BKEEP2["CONSERVA (fail-safe)<br/>'could not read the state'"]
  B6 -- "limpio" --> B7["RemoveIfClean → borra<br/>ForgetReview (olvida el registro)"]
  B7 --> B8["aviso: 'merge ok · worktree removed'"]
  BKEEP --> BNOTICE["aviso compuesto:<br/>merge + rama (DeleteMsg) + worktree<br/>sin perder ningún hecho"]
  BKEEP2 --> BNOTICE
  B8 --> BNOTICE

  BQ["cerrar la app (q / ctrl+c)"] --> BQ1["NO borra ningún worktree"]
  BF["PR mergeado FUERA de prdash"] --> BF1["el refresco lo ve<br/>pero NO dispara limpieza"]
```

## Guardas comunes a las dos vías

```mermaid
flowchart LR
  G0["cualquier intento de borrado<br/>(ruta explícita · --orphans · B)"] --> G1{"Owned?<br/>label o nombre 'prdash-…'"}
  G1 -- "no" --> G2["rechazado"]
  G1 -- "sí" --> G3{"¿bajo la raíz gestionada?"}
  G3 -- "no" --> G2
  G3 -- "sí" --> G4["puede borrarse"]
```
