# AGENTS.md — prdash

Guía para agentes sin contexto previo sobre este proyecto.

## Qué es

Inbox de PRs/MRs multi-forge + orquestador de review sobre
[Herdr](https://herdr.dev). Mezcla GitHub y un GitLab self-managed en un solo
inbox y, al elegir un ítem, deja listo el entorno de review (worktree + layout de
2 tabs) para que el loop de comentarios ocurra sin montar nada a mano.

Estado: **MVP F1 + F2**. F3 (auto-review con gate y allowlist) está documentado
pero **sin implementar** — ver `docs/planning/archive/0001-mvp/f3-milestone.md`.

## Stack

- Go 1.26+ (`go 1.26.3` en `go.mod`), module `prdash`
- `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`
  (import paths `charm.land`, NO `github.com/charmbracelet`)
- `github.com/charmbracelet/x/ansi` — sí se usa, para el manejo de ancho/ANSI
- `go tool` gremlins para mutation testing
- En runtime: `gh` y `glab` autenticados, y Herdr 0.9.x (opcional)

## Comandos

```bash
make build         # compila en ./bin/prdash
make install       # instala en ~/.local/bin/prdash
make test          # gate completo: go build + go vet + gofmt + go test -race
make run           # abre la TUI con go run
make print         # modo --print, sin TUI (para comprobar el pipeline)
make mutate-diff   # mutation testing acotado al diff vs main (advisory)
```

`make test` **falla si hay código sin `gofmt`** (comprueba
`gofmt -l $(git ls-files '*.go')`). Es un gate, no un aviso.

## Paso crucial tras cualquier cambio de código

**Desplegar el binario**: el usuario ejecuta `~/.local/bin/prdash`, no el repo,
y los tests con `go run` no actualizan el instalado.

```bash
make install
```

Sin esto, cualquier verificación que haga el usuario sobre la TUI usa la versión
vieja. Ejecutarlo SIEMPRE al terminar una tarea de código, después de verificar.

**No hace falta cerrar la TUI**: en Linux el binario se reemplaza en disco
mientras el proceso sigue corriendo con su copia en memoria.

## CI

`.github/workflows/mutation.yml` — **mutation testing** con gremlins. Corre en
cada `pull_request` y a mano (`workflow_dispatch`). No en push a `main`.

- **Deliberadamente SIN `paths`**: un workflow con filtro que se vuelve required
  check deja ese check en `pending` para siempre en los PRs que lo skipean, y
  eso deadlockea los PRs. El scope `.go` se aplica DENTRO del job, igual que el
  guard de `make mutate-diff`.
- **Scope**: solo muta si el diff contra la base tiene ficheros `.go`
  (`make mutate-diff MUTATE_BASE=origin/<base>`). `ubuntu-24.04`, timeout 20 min.
- **Gate (bloqueante)**: falla si hay algún mutante **superviviente nuevo** en el
  diff que no esté en `.mutation-allowlist`. Para un mutante que solo se acepte
  si es demostrablemente equivalente, se añade la línea al allowlist con un
  comentario explicando por qué.
- **Degradaciones a conocer**: si no hay `report.json` (timeout o crash de
  gremlins) el gate **pasa** — un crash nunca se lee como fallo. Y si
  `.mutation-allowlist` **no existe**, el gate solo informa y no bloquea
  (no está calibrado). Ojo: gremlins no reporta los mutantes TIMED OUT en
  `report.json` y la eficacia los excluye.
- Sube `report.json` como artefacto (14 días).

`make test` (build + vet + gofmt + `go test -race`) sigue siendo el gate local y
es más rápido: **corrélo antes de abrir el PR**, porque el workflow solo mide
mutación.

## Arquitectura

```
cmd/prdash/            → entry point
internal/config/       → config XDG, defaults, keybindings
internal/forge/        → GitHub / GitLab / Bitbucket (adapters)
internal/inbox/        → items, secciones (Assigned/Mentioned/Mine), refresh
internal/reporesolver/ → resuelve el repo destino de cada ítem
internal/gitcmd/       → envoltura de git (worktrees, refs, merge)
internal/worktree/     → provisioning de worktrees de review
internal/review/       → montaje del review, layout de 2 tabs
internal/herdr/        → control de panes (sin plugin de Herdr)
internal/state/        → persistencia de estado
internal/cache/        → caché (comentarios, etc.)
internal/sim/          → integración con git-sim
internal/tui/          → Bubbletea v2: Inbox, detalle, modales, merge
internal/testutil/     → helpers de test
docs/adr/              → decisiones de arquitectura (ADR numeradas)
```

## Convenciones

- **Archivos**: `snake_case.go`. **Tipos**: `PascalCase`.
- **Comentarios y docs en español** donde aplique (el README y las ADRs lo están).
- **`internal/` exclusivamente** — no hay paquetes exportados.
- **Config nunca aborta**: un config ausente o malformado degrada a defaults con
  un aviso. Es un patrón del proyecto, no una excepción: no introduzcas `os.Exit`
  ni `panic` por config inválido en runtime.
- **Layouts de TUI**: degradación honesta y explícita. Si una caja no cabe, no
  se pinta a medias — todo o nada. La razón está en los comentarios del código.
- **Merge pide dos teclas** y una de ellas es el modo de confirmación.
- **Worktrees de Worktrunk** viven bajo `.worktrees/`.

## Commits

Conventional Commits, observado en el historial:

```
fix(tooling): guard mutate-diff against gremlins empty-diff full-module fallback
feat(retarget): cambiar la rama destino con `e` y un buscador de ramas (#20)
chore(repo): worktrees de Worktrunk bajo .worktrees/
docs(readme): nota de prueba 1/10 (#5)
```

Scope en el idioma del subsistema tocado. El número de PR/MR va entre paréntesis
al final cuando el trabajo viene de una issue.

## Reglas para revisión de código

- **`make test` debe seguir siendo el gate**: nada de romper `gofmt`, `go vet`
  ni la suite con `-race`.
- **No introduzcas `github.com/charmbracelet/bubbletea`**: el import path del
  proyecto es `charm.land/*` v2.
- **No introduzcas dependencies sin pasar por `go mod tidy`** (`make tidy`).
- **No hardcodees forges, hosts ni ramas**: todo eso se deriva de la config XDG
  o se autodetecta. La rama por defecto se deriva, no se escribe a mano.
- **No rompas la degradación**: si una dependencia externa (`gh`, `glab`,
  `git-sim`, Herdr) no está, la acción avisa y no rompe la TUI.
- **No añadas lógica de negocio en el render**: las decisiones van en
  `internal/<subsistema>/`, la TUI pinta.
- Si un cambio altera comportamiento observable, la decisión y sus alternativas
  van en una **ADR nueva** en `docs/adr/` (siguiente número, formato existente).
- `make mutate` / `make mutate-diff` en **local** son *advisory*: reportan y
  nunca bloquean. En **CI** el paso `Gate` sí bloquea (ver la sección CI): un
  mutante sobreviviente nuevo en el diff tumba el PR salvo allowlist.
