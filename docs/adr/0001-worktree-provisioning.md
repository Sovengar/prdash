# ADR 0001 — Provisión del worktree de review

- **Estado**: Accepted
- **Fecha**: 2026-09-24
- **Decisor**: usuario (buble)
- **Alcance**: prdash MVP, fase F2 (ver `docs/planning/0001-mvp/`)
- **Patrón de nombres**: `docs/adr/NNNN-slug.md`

## Contexto

prdash (F2) debe, al elegir un PR/MR, obtener su código y montar un layout de review en Herdr sobre un worktree de la rama del PR/MR. Restricciones de partida:

- Herdr 0.9.x expone `herdr worktree create` (con `--cwd`, `--branch`, `--base`, `--path`, `--label`, `--no-focus`) y **liga el worktree a un workspace** de Herdr.
- El repo del PR puede no estar clonado; se decidió clonarlo en **bare** bajo el directorio de datos XDG del proyecto.
- Las ramas de PRs de forks **no existen necesariamente** como branch de origin, así que hay que traer el ref de review (`refs/pull/N/head` en GitHub, `refs/merge-requests/N/head` en GitLab).
- Fuera de Herdr la aplicación debe degradar de forma limpia.
- Se necesitan **varios worktrees coexistentes** (uno por PR), también del mismo repo.

## Decisión

1. **prdash hace siempre** la resolución del repo local (roots + índice remoto→local + memoria de rutas; clon bare si falta), el **fetch del ref de review** y la **creación de una rama local de trabajo**. Nunca delega el fetch ni la resolución del ref.
2. **Dentro de Herdr** (`HERDR_ENV=1`), la provisión del worktree se delega en el nativo: `herdr worktree create --cwd <repo> --branch <rama-local> --path <destino> --label <prdash-…> --no-focus`.
3. **Fuera de Herdr**, la provisión cae a `git worktree add` directo y el layout se reporta como no disponible (F1 sigue operativo).

## Alternativas consideradas y descartadas

- **Solo `git worktree add`.** Obliga a crear el workspace/tab/pane de Herdr a mano y pierde el vínculo worktree↔workspace (restore de sesión, IDs estables, cierre por grupo), que el layout usa como contenedor.
- **Delegar todo a `herdr worktree create`, incluido el fetch.** El nativo recibiría una rama remota que aún no existe localmente (riesgo abierto del plan) y no cubre el caso de forks ni el clon bare previo.
- **Clonar siempre desde cero con `git clone` normal.** No permite múltiples worktrees desde un mismo clon; el clon bare es el punto de partida correcto para N worktrees.

## Consecuencias

**Positivas**
- El riesgo "`--branch` con rama remota no local" desaparece por diseño: el nativo recibe siempre una **rama local ya existente**.
- Un único dueño del namespace de rutas (`reporesolver`) y worktrees múltiples coexistentes sobre un clon bare.
- El acoplamiento a Herdr queda confinado a un solo puerto: solo ese módulo conoce el nativo y aloja el fallback.

**Negativas / costes**
- prdash debe implementar y mantener su propio fetch y resolución de refs por forge (dos rutas de ref distintas).
- Dependencia de la CLI de Herdr 0.9.x (drift): se mitiga con parseo aislado por comando, versión mínima declarada en el manifiesto y fallback localizado.

**Verificación (cerrada)**
- **OK** `herdr worktree create` desde un **clon bare** con rama local ya creada y `--path` destino, contra `herdr 0.9.1-preview.2026-09-21-0ff0f27e2226`. Evidencia y hallazgos asociados (semántica de `--label`) en `docs/research/herdr-0.9.1-contract.md` §Verificación local.
