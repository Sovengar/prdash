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
make lint          # go vet + gofmt + golangci-lint v2.13.2 (pineado, vía go run)
make check         # build + lint + test: el gate local equivalente a ci.yml
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

Dos workflows, el mismo esqueleto que dbx/gitdash/tsk/vroom: sin `paths`, permisos
mínimos, `concurrency` con cancelación fuera de `main`, `ubuntu-24.04` y actions
pineadas por SHA.

`.github/workflows/ci.yml` — **Build / Lint / Test**. Corre en cada
`pull_request`, en push a `main` y a mano (`workflow_dispatch`).

- **Build**: `go build ./...` + `go vet ./...` (5 min).
- **Lint**: `make lint` = `vet` + `fmt-check` (gofmt) + golangci-lint **v2.13.2**
  pineado en el Makefile, ejecutado con `go run` (sin binario global). No hay
  `.golangci.yml`, así que aplica el set por defecto (errcheck, govet,
  ineffassign, staticcheck, unused). El `Makefile` entra en la clave de caché de
  módulos para que bumpear la versión no reutilice módulos viejos.
- **Test**: `go test -race -count=1 -covermode=atomic -coverprofile=coverage.out
  ./...` más un resumen de cobertura por paquete en el step summary. La suite es
  autocontenida (fixtures git en `t.TempDir()` vía `internal/testutil`, adapters
  falsos), así que no necesita `gh`/`glab`/`herdr`/`git-sim` en el PATH.
- `-count=1` desactiva la caché de tests: con la caché de build restaurada, un
  `ok` cacheado nunca puede hacer pasar un fallo.

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
  `.mutation-allowlist` **no existe**, el job `Mutation` entero se **SKIPEA**
  (visible como not-a-pass) y no bloquea: sin allowlist el gate no está
  calibrado y no mide nada. Ojo: gremlins no reporta los mutantes TIMED OUT en
  `report.json` y la eficacia los excluye.
- Sube `report.json` como artefacto (14 días).
- Consecuencia de lo anterior: un job skippeado es un check skippeado, así que
  `Mutation` **no** debe marcarse como required status check (misma clase de
  deadlock que los `paths`).

`make check` (build + lint + test) es el gate local equivalente a `ci.yml`, y
`make test` (build + vet + gofmt + `go test -race`) el más rápido: **corrélo
antes de abrir el PR**, porque `mutation.yml` solo mide mutación.

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
- **Comentarios en inglés, y solo si explican el POR QUÉ.** El README y las ADRs
  siguen en español: son los que lee el operador, no quien lee el diff. Un comentario
  que explica *cómo* funciona el código se borra —el código ya lo dice y cambia más
  rápido—. Uno que justifica una decisión, una restricción o una trampa se queda, y
  se comprime a una línea. Ver `docs/adr/0009-comentarios-en-ingles.md`.
- **`internal/` exclusivamente** — no hay paquetes exportados.
- **Config nunca aborta**: un config ausente o malformado degrada a defaults con
  un aviso. Es un patrón del proyecto, no una excepción: no introduzcas `os.Exit`
  ni `panic` por config inválido en runtime.
- **Layouts de TUI**: degradación honesta y explícita. Si una caja no cabe, no
  se pinta a medias — todo o nada. El suelo que lo hace posible va escrito junto a la
  aritmética, porque un guard que no puede dispararse esconde la cuenta.
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
