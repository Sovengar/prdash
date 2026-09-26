# prdash

Inbox de PRs/MRs multi-forge + orquestador de review sobre [Herdr](https://herdr.dev).

Responde "¿qué PR/MR me toca?" mezclando GitHub y un GitLab self-managed en un
solo inbox, y al elegir un ítem deja listo el entorno de review (worktree +
layout de 2 tabs) para que el loop de comentarios ocurra sin montar nada a
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
`tab` sección, `r` montar review (el worktree siempre; el layout de 2 tabs
requiere Herdr), `R` refrescar, `a` approve, `m` merge, `o` abrir en el
navegador, `q` salir. Son configurables en `[keybindings]`.

La pantalla se parte en dos: la lista con scroll arriba y el detalle del ítem
seleccionado en el 40% inferior, que se mueve con el cursor. No hay una vista a
pantalla completa: el panel es lo único que hay. La ficha son tres bloques: los
campos cortos en rejilla de dos columnas, el URL en una fila a ancho completo, y
debajo los comentarios en su propia caja.

Los campos van **siempre** en rejilla, no solo cuando no caben en una: en una
sola columna ocupaban 16 de las ~18 líneas que concede el 40% de un terminal
normal, y no cabía ni un comentario. El URL sale de la rejilla porque en media
columna se leen 40 caracteres de una URL de 80, y una URL que no se puede copiar
entera no sirve para nada. No cuesta alto: 12 campos en dos columnas son 6 filas,
las mismas 7 que ocupaban los 13.

### Los últimos comentarios, en su propia caja

Debajo de la ficha se enseñan hasta 5 comentarios de la conversación, en una caja
redondeada con "Comments" en el borde. Se piden al forge **al llegar el cursor al
ítem** y se cachean: moverte arriba y abajo no vuelve a preguntar, y el refresco
del inbox no los tira (una conversación no cambia al ritmo de un ciclo de un
minuto). La única invalidación es una acción sobre el ítem, que sí puede escribir
en la conversación.

Son parte de la ficha, no una vista aparte: no hay tecla que pulsar. Si el forge no
llega a responder, el panel lo dice (`loading…`, `not read: …`, `none`) en vez de
dejar un hueco, porque un hueco no se distingue de "este PR no tiene
conversación".

Lo que se enseña, y por qué:

- **Los últimos, no los primeros.** El final de la conversación es donde está lo
  último que se dijo del PR y el estado actual de la discusión. Van del más antiguo
  de esos al más nuevo, que es como se lee una discusión. Si hay más de los que
  caben, la caja lo dice en el borde de abajo (`5 of 23`) porque es lo que indica
  que conviene abrir el PR.
- **Una caja, no campos más.** La conversación no es un dato del PR sino lo que la
  gente dijo de él, y un borde lo dice sin explicarlo. Sin comentarios —y también
  sin que el forge haya respondido todavía— no hay caja: se queda la línea de
  campo de siempre, porque una caja alrededor de la palabra "none" no separa nada y
  aparecería y desaparecería en cada movimiento del cursor.
- **La caja es todo o nada.** Si no caben sus dos bordes más una fila por
  comentario, no se pinta. Es preferible ver la ficha entera que una caja con un
  solo comentario, porque un recorte de la caja no parece un recorte: parece que el
  PR solo tiene ese.
- **Notas de sistema fuera.** En GitLab, "assigned to @x" o "added 3 commits" no
  son conversación: son el historial de acciones del MR, y mezclado con lo que
  escribió la gente se comería las cinco filas con ruido que ya está en otra parte
  de la ficha.
- **El cuerpo entero, por párrafos.** Las filas se reparten según lo que cada
  comentario necesita: si caben enteros, cada uno toma lo suyo; si no, todos
  reciben una fila —para que los cinco estén— y el sobrante va a quien menos
  tiene. Los párrafos no se pegan entre sí, porque "fix the timeout fix the
  backoff" no dice nada, y lo que no cabe se marca con `…`.
- **Sin boilerplate.** Los bots de GitHub abren con un comentario HTML invisible
  (`<!-- ssf: origin=… -->`) que en un panel de ancho fijo se comería la fila.

Cuando el panel es demasiado pequeño los comentarios son lo primero que se cae,
antes que un campo de la ficha: son lo único que se puede volver a pedir en un
instante, y un campo que se va no vuelve.

### Por qué `approve` no dice nada en la ficha

Aprobar un PR propio no lo admite ningún forge, y el veto no aparece en el detalle:
solo en el aviso, al pulsar la tecla, que es cuando se puede actuar sobre él.

La ficha lo pintaba en todos los renders de todos tus PRs —casi todos los de
"Created by me"— repitiendo lo que el campo `Role` ya dice, y le quitaba dos filas
a los comentarios en justo los ítems donde más se echa de menos. El veto sigue
funcionando igual: `approve` no sale y el motivo se explica entero. Lo que sí
permanece en la ficha es la denegación del forge (`action disabled: …`), que es
pegajosa y su aviso caduca.

### Merge pide dos teclas y una de ellas es el modo

`merge` no se ejecuta a la primera. La primera pulsación solo arma: la caja de
Keybinds se sustituye por la confirmación y no queda nada en curso. La segunda
tecla **es** la elección del modo, y no hay modo por defecto:

| Segunda tecla | Modo |
|---|---|
| `m` | merge commit |
| `r` | rebase |
| `s` | squash |
| `esc` | cancelar |

El motivo es que un merge reescribe historia y no se deshace con un comando, así
que no debe existir ningún camino que lo dispare con una estrategia que no hayas
nombrado. Cualquier otra tecla desarma y hace lo que haría normalmente, para que
un `m` a destiempo no deje la vista esperando. `q` y `ctrl+c` siguen saliendo.

Los avisos nombran el modo tanto al empezar (`merge (rebase) en curso…`) como
al terminar (`merge (squash) ok`), porque sin eso un "merge ok" no dice qué se
hizo.

`approve` no aplica a los PR/MR propios: ningún forge admite aprobar lo que
escribes tú (GitHub lo rechaza en la API y no hay opción para activarlo). prdash
lo detecta antes de llamar a la CLI y marca esos ítems con `ROLE: own`; al pulsar
`a` sale el motivo en el aviso. `merge` sí funciona sobre ellos.

### Layout de review

`r` sobre un ítem monta el worktree y, dentro de su workspace, **dos tabs**:

| Tab | Panes | Para qué |
|---|---|---|
| `Review` | TUICR \| editor | leer la review y editar el código en paralelo |
| `Edit` | Hunk \| agente | el diff contra la rama destino y el agente trabajando |

Los dos panes de cada tab se abren al 50 %, y el primero reutiliza el pane que ya
traía el workspace, así que no queda ninguna pestaña huérfana.

El tab de `Review` se queda aunque falte una herramienta: si no hay binario de
TUICR, del diff o del agente, su pane no se monta y prdash lo avisa en vez de
dejar un hueco mudo. El pane del editor **nunca** se omite, porque su orden
suele ser una función del shell (el típico `vi` que expande a `nvim .`) que no
existe como binario en el `PATH`; si la orden está mal escrita, el error se ve en
el propio pane.

### Comandos de los panes (`[commands]`)

Cada pane se puede sustituir por completo desde `[commands]`:

| Clave | Default | Qué abre |
|---|---|---|
| `tuicr` | `tuicr pr <URL del ítem>` | review de TUICR |
| `hunk` | `hunk diff` | diff del **working tree** con Hunk |
| `agent` | `opencode` | agente |
| `editor` | `vi` | editor en el worktree |

Si defines la clave, su valor se usa **verbatim** como argv completo del pane:
no se le añade la URL del ítem ni el target del diff. Sin clave se usa el
default. Las herramientas de forge (`gh`/`glab`) también se configuran aquí.

Hunk revisa el working tree, no el diff del PR: el pane vive junto al editor, así
que lo que se mira es lo que se está tocando. Para el diff del PR contra la rama
destino, cambia su clave:

```toml
[commands]
hunk = "hunk diff main...HEAD --watch"
```

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

El editor es el único que se ajusta mejor con `[tools].editor`, que es un atajo
para su base sin override:

```toml
[tools]
editor = "nvim ."   # por si no usas el `vi` → `nvim .` de tu shell
```

Funciona igual con la provisión nativa de Herdr (dentro de Herdr) y con git
directo (fuera).

## Dentro de Herdr

prdash **no es un plugin de Herdr**. Habla con la CLI de Herdr por subproceso
(`herdr worktree create`, `herdr tab create`, `herdr pane split`, `herdr pane
run`), así que el worktree, los tabs y los panes los abre él mismo cuando pulsas
`r` sobre un ítem.

Lo único que necesita es **estar dentro de Herdr**: el cliente exige
`HERDR_ENV=1`, que Herdr solo inyecta a sus propios hijos. Lanzado desde una
terminal normal, `r` degrada a git directo y monta el worktree sin panes.

Lánzalo desde cualquier pane:

```sh
prdash
```

PR de prueba 10/10: nota de humo para practicar el ciclo de resolucion de PRs.

Si quieres una tecla para abrirlo, declárala tú en `~/.config/herdr/config.toml`
(prdash no edita ese fichero) y recarga:

```toml
[[keys.command]]
key = "prefix+p"
type = "command"
command = "prdash"
description = "prdash: inbox de PRs/MRs"
```

```sh
herdr server reload-config
```

El atajo es una comodidad, no un requisito: `prdash` a mano en un pane funciona
igual. Lo que sí es un requisito es que el pane esté en el **repo root**, porque
`herdr worktree create` se rechaza desde un workspace de worktree vinculado.

**Lo que no hay:** Ctrl+click sobre una URL de PR/MR para montarla, ni una tecla
de Herdr que monte lo último seleccionado en prdash desde otro pane. Sin plugin no
hay link handlers; para el segundo, salta al pane de prdash y pulsa `r`.

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
