# prdash

Inbox de PRs/MRs multi-forge + orquestador de review sobre [Herdr](https://herdr.dev).

Responde "¿qué PR/MR me toca?" mezclando GitHub y un GitLab self-managed en un
solo inbox, y al elegir un ítem deja listo el entorno de review (worktree +
layout de 3 panes) para que el loop de comentarios ocurra sin montar nada a
mano.

Estado: **MVP F1 + F2**. F3 (auto-review con gate y allowlist) es un milestone
documentado, sin implementar: ver `docs/planning/archive/0001-mvp/f3-milestone.md`.
Historial de versiones: [`CHANGELOG.md`](CHANGELOG.md).

## Requisitos

- Go 1.26+ para compilar.
- `gh` autenticado en GitHub y `glab` autenticado en el GitLab self-managed.
- Opcional: [Herdr](https://herdr.dev) 0.9.x dentro de la sesión para el layout
  de review. Fuera de Herdr, el inbox (F1) sigue operativo y el montaje de
  review reporta que requiere Herdr.

## Build e instalación

```sh
make build      # compila en ./bin/prdash
make install    # instala en ~/.local/bin/prdash
make config GITLAB_HOST=gitlab.miempresa.com   # crea ~/.config/prdash/config.toml
```

La config vive en `$XDG_CONFIG_HOME/prdash/config.toml`. Un fichero ausente o
malformado degrada a defaults con un aviso; nunca aborta.

### Forges y relative URL root

Cada `[forge.<name>]` acepta `host` y `clone_base`. `clone_base` es el relative
URL root donde la instancia publica el clon/web cuando **no** está en la raíz
del host (p. ej. `clone_base = "git"` para `https://host/git/…`). Vacío = raíz.

En GitLab, `api_base` (base REST) es la fuente habitual de ese prefijo: el
relative URL root se **deriva** quitando el sufijo `api/v4` (`"/git/api/v4/"` →
`git`; `"/api/v4/"` → raíz). `clone_base` es un override explícito y, con
`clone_base = "/"`, fuerza raíz. El default `api_base = "/api/v4/"` asume la
instancia GitLab en la raíz.

```toml
[forge.gitlab]
host = "gitlab.miempresa.com"
# Instancia en subcarpeta: https://gitlab.miempresa.com/git/grupo/proyecto
api_base = "/git/api/v4/"   # deriva clone_base = "git"
# clone_base = "git"        # o explícito (manda sobre api_base)

[forge.github]
host = "github.com"
# GitHub Enterprise en subcarpeta:
# clone_base = "ent"
```

## Uso

```sh
prdash            # TUI del inbox
prdash --print    # inbox en texto plano (incluye la ruta del worktree de los
                  # reviews ya montados), sin abrir la interfaz
prdash worktrees  # lista los worktrees de review propiedad de prdash
```

Teclas por defecto: `j`/`k` mover, `pgup`/`pgdn` página, `home`/`end` extremos,
`tab` sección, `enter` detalle a pantalla completa, `r` refrescar,
`m` montar review (requiere Herdr), `a` approve, `M` merge, `o` abrir en el
navegador, `q` salir. Son configurables en `[keybindings]`.

La pantalla se parte en dos: la lista con scroll arriba y el detalle del ítem
seleccionado en el 40% inferior, que se mueve con el cursor. `enter` lo abre a
pantalla completa por si necesitas más espacio.

`approve` no aplica a los PR/MR propios: ningún forge admite aprobar lo que
escribes tú (GitHub lo rechaza en la API y no hay opción para activarlo). prdash
lo detecta antes de llamar a la CLI, marca esos ítems con `ROLE: own` y explica
el motivo; `merge` sí funciona sobre ellos.

### Comandos de los panes (`[commands]`)

El layout de review abre tres panes y cada uno se puede sustituir por completo
desde `[commands]`:

| Clave | Default | Qué abre |
|---|---|---|
| `tuicr` | `tuicr pr <URL del ítem>` | review de TUICR |
| `hunk` | `hunk diff <rama destino>...HEAD` | diff del PR/MR con Hunk |
| `agent` | `opencode` | agente |

Si defines la clave, su valor se usa **verbatim** como argv completo del pane:
no se le añade la URL del ítem ni el target del diff. Sin clave se usa el
default. Las herramientas de forge (`gh`/`glab`) también se configuran aquí.

La rama destino debe ser una **ref local**: prdash clona en bare
(`git clone --bare`), así que las ramas remotas quedan en `refs/heads/*` y no
existen las refs `origin/*`. En un worktree de review usa `main` (no
`origin/main`).

```toml
[commands]
# Forzar el diff de Hunk contra main con auto-reload:
hunk = "hunk diff main...HEAD --watch"
```

Los panes reciben `PRDASH_BASE` con la rama destino del ítem, además de
`PRDASH_REPO`, `PRDASH_NUMBER`, `PRDASH_WORKTREE`, `PRDASH_BRANCH` y
`PRDASH_URL`.

### Gestión de worktrees (`prdash worktrees`)

Los worktrees de review se identifican por su nombre/label `prdash-…`: prdash
**nunca** lista ni borra worktrees ajenos. Se conservan al cerrar la app (no hay
borrado implícito).

```sh
prdash worktrees                 # lista propia (ruta, rama, estado); marca huérfanos
prdash worktrees list            # idem, explícito
prdash worktrees remove <ruta>   # borra SOLO lo pedido y solo si es de prdash
```

Funciona igual con la provisión nativa de Herdr (dentro de Herdr) y con git
directo (fuera).

## Plugin de Herdr

El plugin es **el mismo binario más subcomandos más un manifiesto**
(`plugin/herdr/herdr-plugin.toml`). Una sola fuente de config, credenciales y
versión.

### Instalar/enlazar en desarrollo

`herdr plugin link` registra el manifiesto sin correr build (a diferencia de
`herdr plugin install`, que clona un repo de GitHub):

```sh
make plugin-link          # herdr plugin link "$(pwd)/plugin/herdr"
# equivalente manual:
herdr plugin link ./plugin/herdr
```

Asegúrate de que `prdash` está en el PATH del servidor de Herdr (`make install`
lo deja en `~/.local/bin/prdash`). Para desenlazar: `make plugin-unlink`
(`herdr plugin unlink prdash`). No se edita `plugins.json` a mano: es derivado
de los manifiestos.

### Subcomandos que consume el plugin

| Subcomando | Uso |
|---|---|
| `prdash herdr inbox` | Abre la TUI del inbox en el pane que declara el manifiesto. |
| `prdash herdr mount [URL]` | Monta el review del PR/MR: la URL recibida por argumento o, si no, `HERDR_PLUGIN_CLICKED_URL` / `clicked_url` del contexto del plugin. Sin URL, monta el **ítem seleccionado en la TUI** del inbox. |
| `prdash herdr link` | Igual que `mount`, para el link handler de Ctrl+click (solo confía en `clicked_url`, nunca en `selected_text`). |


PR de prueba 10/10: nota de humo para practicar el ciclo de resolucion de PRs.
### Keybinding

El manifiesto **no** declara teclas. Para bindear la acción de montar review,
añade este bloque a `~/.config/herdr/config.toml` (prdash no edita ese fichero
por ti) y recarga la config:

```toml
[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "prdash.mount-review"
description = "prdash: montar review del PR/MR"
```

```sh
herdr server reload-config
```

La acción resuelve qué montar en este orden: URL por argumento o `clicked_url`
del contexto (link handler); si no hay, el **ítem seleccionado en la TUI** del
inbox. La TUI persiste la selección en `$XDG_STATE_HOME/prdash/selection.json`
(estado efímero de UI). Sin selección, con el estado corrupto o si está
obsoleto, la acción falla con un aviso claro y no monta nada.

## Desarrollo

```sh
make test    # go build ./... && go vet ./... && gofmt check && go test -race ./...
make fmt     # formatea
make print   # comprueba el pipeline sin TUI
```

Diseño: `docs/planning/archive/0001-mvp/` (plan, comportamiento esperado,
contexto, resumen de cierre);
decisiones permanentes en `docs/adr/`; contrato de integración con Herdr en
`docs/research/herdr-0.9.1-contract.md`.

Hello world.
