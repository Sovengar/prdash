# 0001 — prdash MVP: inbox multi-forge + orquestador de review (F1+F2)

## Problema

Quien revisa PRs/MRs repartidos entre GitHub y GitLab self-managed vive saltando de web a web: cada forge tiene su propia UI, su propio estado y su propia noción de "pendiente para mí". El resultado es que el inbox real está fragmentado en dos pestañas, se pierde contexto entre ellas y cada review exige montar a mano el entorno (worktree de la rama, diff, agente, comentarios). No existe un punto único que responda "¿qué me toca revisar ahora?" ni que prepare el terreno de review automáticamente.

prdash existe para cerrar esas dos brechas en una sola herramienta: un inbox cross-forge que unifica lo que me toca, y un orquestador que, al elegir un PR/MR, deja listo el worktree y el layout de review sobre Herdr para que el loop "comento → el agente aplica" ocurra sin fricción.

## Alcance (in)

**F1 — Inbox cross-forge.**
- TUI Go (bubbletea v2) con tres secciones: *(1)* creados por mí, *(2)* review pedido / asignados a mí, *(3)* menciones.
- Forges funcionales: GitHub (`github.com`, vía `gh`) y GitLab self-managed (`gitlab.example.com`, REST bajo subfolder `/git/api/v4/`, vía `glab`).
- Estado rico de GitHub vía GraphQL (`gh api graphql`: `reviewDecision`, checks); GitLab vía GraphQL (`glab api graphql`: `currentUser` authored / reviewRequested / assigned) + Todos API para menciones.
- Approve/merge rápido desde el inbox con `gh`/`glab` directo; delegan en `tuicr` cuando aplique (tuicr ya sube reviews reales vía gh/glab).
- Interface de adapter de forge documentada, con Bitbucket *solo como interfaz* (compila, sin llamadas reales).

**F2 — Orquestador de review por PR/MR.**
- Al seleccionar un PR/MR: crear worktree de su rama y montar layout en Herdr con tres panes: TUICR sobre el PR, Hunk (diff) y agente opencode.
- Loop de comentarios: el usuario comenta en TUICR/Hunk; el agente los lee y los aplica.
- Plugin de Herdr: binario Go + `herdr-plugin.toml` (pane entrypoint con placement, keybind y link handler para URLs de PR). Target Herdr 0.9.x.
- Degradación limpia fuera de Herdr: sin `HERDR_ENV=1` la app informa y no rompe.

## No-alcance (out)

- Bitbucket funcional (solo la interfaz de adapter).
- gitlab.com funcional (sin token; queda como futuro).
- F3 auto-review/auto-approve: solo diseño y milestone post-MVP, sin implementación.
- Vista de "todos los PRs abiertos" (el scope es mi inbox, no el backlog ajeno).
- Webhooks, daemon servidor o cualquier componente push.
- Gestión de repos locales (prdash no los descubre ni administra más allá del worktree de review).
- Multi-usuario / multitenancy: asume una identidad por forge ya autenticada.

## Criterios de aceptación

- [ ] La TUI arranca y muestra las 3 secciones con datos reales de GitHub y del GitLab self-managed.
- [ ] Cada PR/MR listado refleja el estado del forge (p. ej. `reviewDecision`/checks en GitHub, autoría/review-request en GitLab) sin salir de la TUI.
- [ ] Las menciones provienen de la fuente correspondiente por forge (GraphQL en GitHub, Todos API en GitLab).
- [ ] Seleccionar un PR/MR crea el worktree de su rama.
- [ ] El layout de Herdr abre los 3 panes esperados (TUICR, Hunk, agente opencode).
- [ ] Un comentario escrito en TUICR queda legible por el agente y este lo aplica.
- [ ] Approve/merge rápido funciona desde el inbox en GitHub y en el GitLab self-managed, o delega en `tuicr` cuando aplica.
- [ ] Fuera de Herdr, la app informa la limitación y no rompe (F1 sigue operativo).
- [ ] El adapter de Bitbucket compila y está documentado, sin ninguna llamada real.
- [ ] `go build ./... && go vet ./... && go test ./...` pasa y el binario queda instalable en PATH (`~/.local/bin/prdash`).

## Preguntas abiertas (bloquean la aprobación)

1. **PR → clon local**: para crear el worktree de un PR, prdash necesita la ruta local del repo de ese PR (`owner/repo` o proyecto GL → carpeta). No está definido cómo se resuelve. Propuesta por defecto: `roots` en la config TOML (patrón gitdash) + índice remoto→local construido al escanear esos roots, y si no hay match, pedir la ruta una vez y recordarla.
2. **Cadencia de refresco del inbox**: ¿poll automático o refresh manual? Propuesta por defecto: refresh manual (`r`) + auto cada 60s configurable, con indicador de "última actualización".

## Riesgos / verificaciones pendientes

- **`tuicr pr` submit contra el GitLab self-managed**: verificar que tuicr sube reseñas reales por `glab` en `gitlab.example.com` y no solo en GitHub.
- **`herdr worktree create --branch` con rama remota no local**: confirmar el comportamiento cuando la rama del PR aún no existe en el clon local (fetch implícito, fallo, o necesidad de fetch previo).
- **Permisos de approve/merge en el self-managed**: verificar que el usuario (`<usuario>`) tiene capacidad de approve/merge, o el botón/acción rápida debe deshabilitarse.
- **Placement del pane del plugin Herdr**: validar que `herdr-plugin.toml` coloca los panes como se espera en Herdr 0.9.x y que el link handler resuelve URLs de PR.
- **Ancho/estilos de celdas de tabla**: el render ANSI y el cálculo de ancho pueden romper la tabla del inbox (patrón conocido: `pad()` antes de aplicar estilo).
- **Degradación limpia**: comprobar que la detección de `HERDR_ENV=1` falla suave y no bloquea el arranque de la TUI.
