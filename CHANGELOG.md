# Changelog

Todos los cambios notables de prdash se documentan en este fichero.

El formato sigue [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/) y el
versionado sigue [Semantic Versioning](https://semver.org/lang/es/).

## [Unreleased]

## [0.1.0] - 2026-09-25

Primera versión: inbox cross-forge (F1) y orquestador de review (F2). Coincide
con la versión del manifiesto del plugin (`plugin/herdr/herdr-plugin.toml`).

### Added

**F1 — Inbox cross-forge** ([#1](https://github.com/Sovengar/prdash/pull/1)):

- TUI (bubbletea v2) con tres secciones —creados por mí, review pedido/asignados
  y menciones— con dedupe por identidad y precedencia de sección.
- Forges GitHub (vía `gh`, GraphQL con `reviewDecision` y checks) y GitLab
  self-managed (vía `glab`, GraphQL + Todos API, REST bajo `/git/api/v4/`).
- Detalle del ítem (título, autor, ramas, número, URL, review y checks) sin
  salir de la TUI.
- Refresco manual (`r`) y automático (60s configurable) con indicador de última
  actualización por forge, backoff ante rate limit/timeout y guard para no pisar
  una acción en curso.
- Paginación sin tope con carga progresiva, snapshot en `cache` y refresco
  incremental por cursor.
- Acciones approve/merge vía `gh`/`glab` (o delegando en `tuicr`), con manejo de
  conflicto y relectura del ítem.
- Degradación honesta: estado explícito por forge/sección (nunca "vacío" si hubo
  error); Bitbucket presente como adapter no operativo, sin red.
- Modo `prdash --print` (texto plano, mismo orden que la TUI).

**F2 — Orquestador de review** ([#2](https://github.com/Sovengar/prdash/pull/2)):

- Resolución del repo local, clon bare configurable, fetch del ref de review
  (`refs/pull/N/head`, `refs/merge-requests/N/head`, incluidos forks) y rama
  local de trabajo.
- Provisión de worktree con dos implementaciones intercambiables: git directo
  (fuera de Herdr) y nativa de Herdr (dentro), con reuso del worktree existente y
  varios worktrees por repo.
- Layout de 3 panes sobre el worktree (TUICR, Hunk, agente opencode), con
  omisión avisada de las herramientas ausentes.
- Plugin de Herdr (`plugin/herdr/herdr-plugin.toml`): pane del inbox, acción
  `mount-review`, link handler de URLs de PR/MR y subcomandos
  `prdash herdr inbox|mount|link`.
- Atajo `m` para montar el review del ítem seleccionado; `prdash.mount-review`
  sin URL resuelve la selección persistida en
  `$XDG_STATE_HOME/prdash/selection.json`.
- Degradación fuera de Herdr: F1 sigue operativo y el layout se informa como no
  disponible.
- Comando `prdash worktrees [list|remove]`: lista los worktrees con ownership
  `prdash-…`, marca huérfanos y borra solo a petición explícita (nunca los
  ajenos); los worktrees se conservan al cerrar la app.

### Known limitations

- **F3 (auto-review con gate y allowlist) no incluido**: solo diseño/milestone
  documentado en `docs/planning/archive/0001-mvp/f3-milestone.md`.
- Readiness de panes con `herdr pane wait-output` pendiente: los panes se lanzan
  fire-and-forget.
- `herdr worktree create` abre también el workspace del repo fuente; su
  cierre/gestión queda pendiente.
- LOWs residuales no bloqueantes de las reviews adversariales de F1 (PR #1) y
  F2 (PR #2).
- Fuera de alcance: Bitbucket funcional, gitlab.com funcional, vista de "todos
  los abiertos", webhooks/daemon, gestión de repos locales más allá del worktree
  y multi-usuario.
