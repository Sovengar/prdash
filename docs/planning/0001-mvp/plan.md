# 0001 — prdash MVP — Plan

adr_required: true
adr_reason: la provisión del worktree de review tiene alternativas reales descartadas (nativa de Herdr vs git directo) y fija el acoplamiento entre prdash y Herdr.
adr_title: adr-worktree-provisioning
adr_path: docs/adr/0001-worktree-provisioning.md
adr_note: el ADR es permanente (vive en docs/adr/) y sobrevive al archivado de este planning.

## Resultado esperado

Una sola herramienta (TUI Go + plugin de Herdr) que responde "¿qué PR/MR me toca?" mezclando GitHub y un GitLab self-managed en un inbox de 3 secciones, y que al elegir un ítem deja listo el entorno de review (worktree + layout de 3 panes) para que el loop "comento → el agente aplica" ocurra sin montar nada a mano. Fuera de Herdr, el inbox sigue siendo útil; el orquestador degrada con un aviso.

## Alcance de fases

- **F1 — Inbox cross-forge (MVP).** Inbox de 3 secciones (creados por mí / review pedido o asignados / menciones) con datos ricos de GitHub y del GitLab self-managed, detalle de ítem, refresco manual + automático, y acciones approve/merge. Bitbucket solo interfaz.
- **F2 — Orquestador de review (MVP).** Resolver/crear el clon local, provisionar el worktree de la rama del PR (incluidos forks), y montar el layout de 3 panes sobre el worktree dentro de Herdr. El loop de comentarios es responsabilidad de las herramientas/agente, no de prdash.
- **F3 — Auto-review (milestone, no se implementa acá).** El agente revisa y, con gate + allowlist por repo, aprueba y notifica. Se planifica su encaje, no su código.

## Enfoque (alto nivel)

Pipeline F1 como en el hermano gitdash: *config → clientes de forge (subprocess `gh`/`glab`) → parseo puro → merge/dedupe → modelo TUI → render*. Pipeline F2: *ítem → resolutor de repo → fetch del ref de review → provisión de worktree → puerto Herdr (layout) → notificación*. La capa de parseo es pura y testeable con fixtures string; el I/O vive en los adapters y en los puertos. Nada de daemons ni estado de larga vida.

## Módulos y contratos (fronteras, no estructura de ficheros)

**Puro (sin red, sin subprocess, sin TOML, sin disco):**
- `forge/model` — tipos normalizados: referencia de repo, ítem (sección, forge, ref, estado, decisión, checks, URL, marca temporal).
- `forge/parse` — traduce JSON de `gh`/`glab` (GraphQL, REST, Todos) a `model`.
- `inbox` — consolida las 3 secciones, deduplica y decide "¿me toca?".
- `state` — estados derivados con precedencia y score de orden (atención primero), compartido con el modo `--print`.
- `review/plan` — dado (ítem, worktree, entorno) produce el plan de panes (cwd, argv, labels) sin tocar Herdr.

**I/O (adapters y puertos):**
- `config` — TOML XDG; `Load()` nunca falla (defaults + warning). roots, forges/hosts, cadencia, rutas de clon bare/worktrees, argv de herramientas.
- `forge` + `forge/{github,gitlab,bitbucket}` — implementan el contrato `Adapter` (estado/auth, listar authored, review-requested/asignados, menciones, estado de ítem, approve, merge) devolviendo ítems **más warnings tipados; nunca fallan duro**.
- `reporesolver` — único dueño del namespace de rutas: índice remoto→local sobre `roots`, memoria de rutas, clon bare, fetch del ref de review. No llama a la API del forge (resuelve remotos de clones existentes).
- `worktree` — puerto de provisión con dos implementaciones intercambiables (nativa Herdr / git directo); el llamador no sabe cuál corre.
- `herdr` — **único** lugar que lee `HERDR_ENV` y parsea salida de Herdr: disponibilidad, workspace/tab/pane, layout, notificaciones, link handlers.
- `review/executor` — aplica el plan usando los puertos (reporesolver, worktree, herdr).
- `cache` — snapshot del inbox y rutas recordadas; corrupto = silencioso.
- `tui` — modelo bubbletea, tabla/detalle, cadencia y event pump.
- `cmd/prdash` — entrypoint: TUI, `--print`, subcomandos `herdr …` que consume el plugin.

**Contratos clave (roles, sin firmas):** `forge.Adapter` (lista + warnings, nunca error duro), `reporesolver.Resolver` (resolver ruta local, asegurar clon bare, traer ref de review), `worktree.Provisioner` (crear/borrar/listar), `herdr.Port` (disponibilidad, layout, notificar, abrir link). Los no-cruzables: `inbox`/`parse` no tocan red ni disco; los adapters no tocan git/worktree/TUI; `reporesolver` no toca la API del forge; `herdr` no toca git ni TUI; `review` solo habla por puertos.

## Decisiones clave

1. **Provisión del worktree: nativa de Herdr dentro de Herdr, `git worktree add` fuera.** prdash hace siempre el fetch y la resolución del ref (incl. refs de PR de forks) y crea la rama local; luego delega la provisión en el nativo de Herdr (liga el worktree a un workspace, con IDs estables que el layout necesita como contenedor) y cae a git directo fuera de Herdr. Hacer nosotros el fetch elimina el riesgo de "--branch con rama remota inexistente": el nativo recibe siempre una rama local ya existente.
2. **Plugin = mismo binario + subcomandos + manifiesto.** El manifiesto de Herdr declara el pane del inbox, una acción para montar el review del ítem seleccionado y un link handler de URLs de PR que reenvía a un subcomando. Una sola fuente de config/credenciales/versión; manifiesto y código no divergen.
3. **El loop comentario→agente no es de prdash.** La responsabilidad termina al abrir panes con cwd, env y argv correctos: sin daemon, sin poller, sin API de comentarios. Evita acoplarse a TUICR/Hunk y a la API de review de cada forge.
4. **Bitbucket es un adapter registrado, no un stub muerto.** Comparte la suite de conformidad con las implementaciones reales y debe responder "no soportado" explícito, sin red.
5. **Detección de entorno en un solo sitio.** Solo `herdr` lee `HERDR_ENV`; el resto recibe el entorno inyectado, así la TUI se testea en modo "fuera de Herdr".
6. **Config única XDG.** Adapters y TUI no leen entorno/TOML; el plugin usa el mismo TOML.

## Sin tope por sección: paginación y carga progresiva (área crítica)

Se decide **no capar** las secciones: se pagina hasta agotar cada una.

- **GitHub** vía GraphQL con `pageInfo/endCursor` hasta `hasNextPage=false`. **GitLab** vía iteración de páginas en GraphQL/REST (o `glab api --paginate`).
- **Carga progresiva:** el primer render muestra la primera página de cada sección (rápido) y las páginas restantes llegan en segundo plano sin bloquear la UI ni el refresco.
- **Impacto en rate limits/refresco:** con refresco automático cada 60s y paginación ilimitada, el coste por ciclo puede dispararse. Mitigación obligatoria: reutilizar el snapshot en `cache` y refrescar incremental (primera página + comparación por cursor), backoff y respeto de las cabeceras de rate limit (GitHub ~5000 pts/h; el 401 de gitlab.com se trata como forge no disponible), y pausar el auto-refresco mientras hay una acción en curso o se está paginando.
- **UI:** indicador "última actualización" por forge (una forge lenta no debe mentir sobre el resto) y contador de "cargando más…" cuando queda paginación pendiente.

## Riesgos

- **Rate limit / coste por `--paginate`** — mitigado con snapshot + incremental + backoff (ver arriba).
- **Auth del GitLab self-managed con subfolder `/git/api/v4/`** — encapsulado tras el adapter con base URL configurable; test de solo-lectura contra el host real.
- **Drift de la CLI de Herdr (0.9.x)** — parseo aislado por comando, versión mínima en el manifiesto y fallback (git + splits) confinado a `herdr`.
- **Drift de salida/exit codes de `gh`/`glab`** — parseo puro con fixtures string.
- **Worktrees huérfanos / basura** — ownership en el nombre, listado y comando explícito de limpieza (los worktrees se conservan al cerrar).
- **Degradación silenciosa** (confundir error con vacío) — estado explícito por forge/sección; nunca "vacío" si hubo error.
- **TUICR/Hunk cambian flags** — son argv configurables en el plan, no APIs; binario ausente = pane omitido con aviso.
- **F3 auto-approve** — gate + allowlist por delante; nunca aprobar con análisis fallido o parcial; dry-run por defecto.

## Verificaciones pendientes

- `tuicr pr` submit (review real) contra el GitLab self-managed, no solo GitHub.
- `herdr worktree create` desde un **clon bare** con rama local ya creada y `--path` destino (equivalente a la incógnita de "--branch con rama remota no local", que el diseño elimina al hacer el fetch antes).
- Permisos de approve/merge del usuario en el self-managed (`<usuario>`); si faltan, la acción debe deshabilitarse con motivo.
- `placement` real del pane del plugin y resolución del link handler en Herdr 0.9.x.
- Comportamiento de `git clone --bare` + `fetch` de refs de PR en el host self-managed (refs de merge-requests y permisos de fetch).

## Orden de trabajo (grueso)

1. Esqueleto: config + model + parse + adapter GitHub + adapter GL + inbox + TUI de solo-lectura (F1 núcleo).
2. F1 completo: detalle, refresco/paginación/carga progresiva, acciones approve/merge, degradación por forge, adapter Bitbucket + conformidad.
3. F2 núcleo: resolutor de repo + clon bare + fetch de refs + provisión de worktree.
4. F2 layout: puerto Herdr, plan de panes, plugin/manifiesto, link handler, degradación fuera de Herdr.
5. Cierre: limpieza de worktrees, `--print`, y dejar F3 documentado como milestone.

## Fuera de alcance (recordatorio)

Bitbucket funcional, gitlab.com funcional, F3 implementado, vista de "todos los abiertos", webhooks/daemon, gestión de repos locales más allá del worktree, multi-usuario.
