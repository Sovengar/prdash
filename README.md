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

PR de prueba 1/10: nota de humo para practicar el ciclo de resolucion de PRs.

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
`tab` cambia de sección (Assigned → Mentioned → Mine), `p` modo de prefijo (ver
[Prefijo de ruta](#prefijo-de-ruta-tres-modos-con-p)), `r` montar review (el
worktree siempre; el layout de 2 tabs requiere Herdr), `R` refrescar, `a`
approve, `m` merge, `e` cambiar la rama destino, `v` simular, `o` abrir en el
navegador, `q` salir. Son configurables en `[keybindings]`. Con el merge armado,
`m`/`r`/`s` eligen estrategia, `tab` conmuta si la rama se borra y cualquier otra
tecla cancela (ver [Merge](#merge-pide-dos-teclas-y-una-de-ellas-es-el-modo)).

El Inbox pinta **una sola sección a la vez**: al abrir muestra **Assigned**, y el
borde superior lleva la leyenda de conteos `Mine (n) · Assigned (n) · Mentioned
(n)`, con la activa resaltada. Cada sección recuerda su cursor y su scroll.

### Prefijo de ruta: tres modos con `p`

La columna ITEM no siempre muestra la ruta igual. `p` cicla **tres modos**, y el
nombre del modo sale en la barra de atajos (`p prefix: full`) para no tener que
contar pulsaciones:

| Modo | La línea de prefijo | La celda ITEM |
|---|---|---|
| `common` (al abrir) | con el prefijo común de la sección activa | solo el sufijo: `api-gateway#100` |
| `full` | no se pinta | la ruta entera, recortada por la cola si no cabe: `…kend/vsocial-api-actuacions#1016` |
| `leaf` | no se pinta | solo la hoja: `vsocial-api-actuacions#1016` |

En `full` y `leaf` la lista recupera la línea que ocupaba el prefijo, y la columna
ITEM se ajusta a lo que ahora muestra. Ojo con `full`: la referencia de un
subgrupo largo no cabe en el tope de la columna, así que **se recorta por la cola**
y se ve `…kend/api-gateway#1016`, no la ruta entera. Y como ITEM llega al tope,
es el modo que peor aguanta un terminal estrecho: por debajo de ~50 columnas la
columna desaparece y la tabla se queda solo con FORGE. Si la sección no tiene
prefijo común (un solo ítem, o nada en común), `common` se ve igual que `full`: no
inventa un prefijo ni repite la ruta.

Tres avisos honestos: el modo **no se persiste** (al reabrir vuelve a `common`),
aunque la tecla `p` sí es configurable por `[keybindings]` —y si tenías otra acción
asignada a `p`, ahora la pierdes: la barra y el ciclo son de `prefix-mode`. Y
`leaf` **no desambigua**: dos repos de grupos distintos con la misma hoja se ven
iguales (`acme/one#7` y `other/one#8` → `one#7` y `one#8`). Para leer la ruta
completa está la ficha del ítem y `--print`. La decisión y sus alternativas están
en [ADR 0005](docs/adr/0005-selectable-prefix-mode.md).

### Simulación (`v`)

`v` abre un popup que renderiza con [git-sim](https://github.com/initialcommit/git-sim)
cómo quedaría el historial al integrar el PR, y lo enseña **encima** del inbox: la
vista de fondo se sigue viendo salvo donde tapa la caja. `enter` renderiza, `esc`
cierra y `o` abre la imagen en el visor del sistema.

No es un gate: git-sim dibuja, no ejecuta, y su veredicto solo existe dentro de la
imagen. Es un visualizador. Para saber si un merge **choca**, `v` no sirve.

Requisitos y límites:

- Necesita `git-sim` en el PATH (con `manim`, `cv2` y Python). Si no está, la
  acción avisa y no hace nada.
- Necesita el review montado (`r`): los refs solo existen en local después del
  `fetch`. Sin él, avisa.
- Todo el render ocurre en un **clon temporal** de esos refs, con la rama base
  activa, y se borra al terminar. No toca el worktree del review ni el clon del
  usuario: no deja refs, worktrees, ramas ni cambios sin commitear.
- Solo se ofrece `merge`. git-sim 0.3.5 no sabe dibujar un `rebase`: si la rama
  del PR ya está basada en la base —el caso normal— responde con un mensaje
  invertido, y si divergen revienta con un `IndexError`. Cuando el proyecto lo
  arregle, la lista de estrategias de `internal/tui/sim.go` es lo único que hay
  que tocar.
- La caja se dimensiona a lo que la imagen necesita manteniendo su proporción, y se
  queda con el 75% del alto de la terminal dejando fondo alrededor.
- **Dentro de Herdr con `terminal.kitty_graphics` activo** (y un terminal exterior
  que lo soporte, como kitty), la imagen se publica en la **capa de gráficos del
  pane** y la pinta el terminal a resolución nativa. Es lo que quita el aspecto de
  mosaico: los half-blocks están quantizados a la rejilla de celdas, así que una
  imagen de 1920 px en 84 columnas salía con cada píxel convertido en un bloque de
  23×23 celdas.
- **Sin Herdr, o con la capa apagada o sin respuesta**, la imagen se pinta con
  half-blocks en truecolor (dos píxeles por celda). Se ve pixelada —es el techo de
  una rejilla de caracteres— pero es la degradación honesta. `o` la abre en el
  visor en cualquier caso.
- Las imágenes se conservan en `$XDG_CACHE_HOME/prdash/sim` (las 20 últimas).

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
  de esos al más nuevo, que es como se lee una discusión. El borde de abajo de la caja
  lleva el recuento (`3 of 3`, `5 of 23`): el total es lo que indica que conviene
  abrir el PR, y el número de los que se ven también cuenta, porque el tamaño de la
  conversación es parte del estado del PR.
- **Una caja, no campos más.** La conversación no es un dato del PR sino lo que la
  gente dijo de él, y un borde lo dice sin explicarlo. Va sangrada una columna a cada
  lado y con el mismo gris de borde que el resto de las cajas: el sangrado es lo que
  dice que está anidada en el panel. Sin comentarios —y también
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
| `r` | rebase |
| `m` | merge commit |
| `s` | squash |
| `tab` | conmutar el borrado de la rama |
| `esc` | cancelar |

El motivo es que un merge reescribe historia y no se deshace con un comando, así
que no debe existir ningún camino que lo dispare con una estrategia que no hayas
nombrado. Cualquier otra tecla **cancela y se consume**: antes se re-despachaba
como si nada, y eso convertía un merge mal armado en approve (`m` y luego `a`
aprobaba el PR) o en un merge commit inmediato (`m` y luego `m`). Cancelar sin
re-despachar cumple lo mismo —la vista deja de esperar igual— pero sin el efecto
secundario. `q` y `ctrl+c` siguen saliendo.

**Solo se ofrecen los modos que el repositorio admite.** GitHub publica
`mergeCommitAllowed` / `rebaseMergeAllowed` / `squashMergeAllowed` en la misma
consulta del ítem, así que la lista es exacta y no cuesta una llamada: un repo
con el squash desactivado no muestra `s`. Cuando las reglas no se conocen —GitLab
no las expone por GraphQL, `Project.mergeMethod` no existe en su schema— se
ofrecen las tres, porque no saber no es lo mismo que no permitir, y un filtro
inventado dejaría al usuario sin salida legítima.

Los avisos nombran el modo tanto al empezar (`merge (rebase) en curso…`) como al
terminar (`merge (squash) ok · branch deleted`), porque sin eso un "merge ok" no
dice qué se hizo.

### El merge también nombra si borra la rama

La segunda de las dos cosas que el merge decide es si la rama origen se borra al
integrar. Vive en la misma Confirmación y se cambia con la misma tecla que ya
tenía otro destino: `tab`. La caja lo enseña siempre que el merge está armado —
`delete branch: yes (tab)`— y fuera de ahí no aparece, porque no es una decisión
que se pueda tomar en otro sitio.

**El default es borrar.** Es lo que hacen los forges por su cuenta y lo que
espera quien limpia detrás de un PR merged; pedir un gesto extra para evitarlo es
pedir confirmaciones de las que la gente se cansa. `tab` lo apaga para el resto
de la sesión, y `tab` otra vez lo devuelve. El valor es de sesión, no de ítem:
borrar la housekeeping no depende del PR que tengas delante.

El aviso final dice qué pasó con la rama, y hay tres finales distintos porque
son tres situaciones distintas:

| Final | Aviso |
|---|---|
| Merge y borrado | `merge (squash) ok · branch deleted` |
| Merge hecho, borrado rechazado por el forge | `merge (squash) ok · branch not deleted: <motivo>` |
| PR de fork | `merge (squash) ok · branch not deleted: the branch lives in a fork` |

El segundo es el que obliga a mirar dos veces. El borrado va en el **mismo
comando** que el merge (`gh pr merge --delete-branch`,
`glab mr merge --remove-source-branch`), así que si el forge lo rechaza la CLI
sale con error aunque la integración ya esté hecha: sin push, con la rama
protegida, o contra un repo con merge queue —que rechaza `-d` antes de
mergear—. Reportarlo como "merge falló" haría que el usuario buscara un cambio
de estado del forge que no ocurrió. La relectura que el merge ya hacía es la que
distingue los casos: si el ítem vuelve mergeado, el merge salió y lo que falló
fue el borrado.

El tercero es un no-op del forge, no un fallo: un PR de fork no tiene rama que
borrar en el repo destino, y `gh` lo da por hecho y sale con éxito. Sin decirlo,
"branch deleted" sería mentira.

Lo que **no** toca la llamada es el repositorio local: con `--repo` (GitHub) y
`-R` (GitLab), la CLI solo borra la rama remota. Los clones bare y los worktrees
que gestiona prdash quedan intactos.

### Cambiar la rama destino con `e`

`e` abre un popup con **las ramas del repositorio**, que se piden al forge al
abrir, y deja escribirlas para filtrarlas. Se elige una con `↑`/`↓` (con el filtro
vacío, `j`/`k` también) y `enter`; la segunda `enter` es la confirmación.

```
╭ retarget acme/widget#7───────────────────────────────────────╮
│from main  ·  5 branches                                      │
│▸ main                                               · current│
│  develop                                                     │
│  feat/una-rama-deliberadamente-larguisima-que-no-cabe        │
│  fix/hunk-pane-argv                                          │
│  release/2.0                                                 │
│                                                              │
│filter (type to search)                                       │
│↑↓ move · enter choose · esc close                            │
╰──────────────────────────────────────────────────────────────╯
```

Tres decisiones, y por qué:

- **Las ramas salen del forge, no de un campo de texto.** Cambiar la base a una
  rama que se le parece pero no es (`main` por `main-2`, o `release/2.0` por
  `release/2.0-rc1`) lo acepta el forge sin quejarse y no se ve hasta que el PR
  apunta a la rama equivocada. Una errata que ni el compilador ni el forge señalan
  es justo la que un buscador hace imposible. También por eso el listado se pide
  al forge y no al clon local: el clon solo tiene las refs bajadas.
- **Hay confirmación.** Mover la base rehace el diff, la mergeabilidad y el CI, y
  lo que se hubiera aprobado antes pasa a compararse contra otra cosa. La
  confirmación dice las dos ramas —`main → release/2.0`— para que un dedo no
  reoriente el PR por una fila de más. `esc` vuelve a la lista en vez de cerrar:
  señalar la fila equivocada es el error más probable y no debería costar las tres
  pulsaciones.
- **`j`/`k` navigan solo con el filtro vacío.** En cuanto hay texto escrito son dos
  letras más del filtro, porque escribir un nombre de rama con `j` tiene que ser
  posible. `ctrl+u` borra el filtro y les devuelve su segundo oficio.

Detalles que conviene saber:

- El listado se cachea **5 minutos por repositorio**, así que abrir y cerrar el
  popup no cuesta una llamada cada vez. Pasado el TTL se vuelve a preguntar, para
  que una rama recién creada aparezca.
- Elegir la rama que el ítem **ya tiene** no hace nada: se avisa y no se llama al
  forge, que contestaría "sin cambios".
- Los avisos nombran las dos ramas: `retarget (main → release/2.0) ok`. Sin ellas
  un "retarget ok" no dice nada de lo que pasó con el PR.
- **El review ya montado no se toca.** Si había un worktree, el aviso lo dice —
  `· the mounted review still has the old base…`— porque sigue con la base
  anterior. No se rebasea ni se rehace: el worktree es del usuario y puede tener
  cambios sin commitear.
- Solo se aplica a ítems **abiertos**: un PR mergeado o cerrado no es algo cuya
  base se pueda cambiar, y no se gasta la llamada.
- El forge es el que manda sobre los permisos: si no eres mantenedor del
  repositorio, su respuesta se enseña tal cual y la acción queda deshabilitada
  para ese ítem.

Detalle de implementación que no se ve en la UI: **no se usa `gh pr edit --base`**
(aunque es lo que documenta `gh`), porque hoy falla antes de tocar nada con
`GraphQL: Projects (classic) is being deprecated…` —la query con la que `gh` mira
si el PR está en un proyecto—. Se usa `gh api -X PATCH …/pulls/N -f base=`. En
GitLab se usa `glab api -X PUT …/merge_requests/N -f target_branch=` y no
`glab mr update --target-branch`, porque ese es un comando de edición y su razón
de ser es abrir el editor.

### Lo que el merge comprueba antes de salir

Un merge no es un comando: es la acción de la que este outflow no tiene vuelta
atrás. Por eso hay tres cosas que se miran y que antes no se miraban.

**El CI y los cambios pedidos se anuncian antes de la confirmación, no se
prohíben.** El modelo de estado ya distinguía un ítem con checks en rojo de uno con
cambios pedidos de uno sano —la ficha los enseña— pero el gate no los miraba, así
que `merge` salía igual en los tres casos. Ahora:

| Estado | Qué pasa |
|---|---|
| Borrador, ya mergeado, ya cerrado | **No arma.** Es una propiedad del forge: GitHub rechaza el merge de un PR en borrador, y ofrecerlo solo gasta una llamada para recibir un error. |
| **Las ramas se pisan** | **Arma, y avisa** nombrando la rama: `merge acme/widget#6 with the branch conflicts with main · press the mode anyway…`. Es un rebase, y un rebase lo hace el usuario. |
| CI en rojo | Arma, y la confirmación dice `merge acme/widget#7 with CI is failing (2 of 5) · press the mode anyway…` |
| CI todavía corriendo | Arma, y avisa: mergear mientras el CI corre es la carrera que el pin del head no cierra, porque el CI puede pasar *después* del merge. |
| Cambios pedidos | Arma, y avisa. |

Prohibirlos del todo convertiría la herramienta en un muro —un check inestable
dejaría el PR sin poder mergear nunca—, y no hacer nada los haría invisibles. Un
gate que avisa siempre entrena a ignorar el aviso, así que en un ítem sano no sale
ninguno.

El borrador se mira como lo que es —una propiedad del forge— y no como un estado
del ítem, y por eso frena igual con la review aprobada o sin ella: `State` ordena
por atención al operador y un borrador aprobado sale como `approved`. La ficha lo
dice en su propia fila `Draft` en vez de esconderlo dentro de `State`.

El conflicto de ramas también se avisa antes, y por el mismo motivo: sale en la
caja (`the branch conflicts with main`) y no como un rechazo de la CLI después de
gastar la llamada. Los dos datos viajan en la consulta que ya se hacía del ítem
(`mergeable` en GitHub, `detailedMergeStatus` en GitLab), así que no cuestan
llamada, y donde el forge todavía no lo sabe —GitHub devuelve `UNKNOWN` mientras
lo calcula— no se dice nada: un aviso sin dato es un aviso falso.

Cuando el rechazo llega igualmente —porque el PR se empujó entre el refresco y la
pulsación, o porque el dato no venía—, el aviso dice qué hacer y no promete un
refresco que no arregla un rebase:

| Situación | Aviso |
|---|---|
| El ítem cambió mientras lo mirabas (cerrado, mergeado) | `forge conflict: …` — un refresco lo resuelve |
| Las ramas se pisan | `merge refused: the forge will not merge it as it is: rebase the branch onto the target and push` |

Que sean dos mensajes y no uno es el punto: en el vocabulario de prdash un
"conflicto" se arregla solo, y mezclarlo con el rechazo por ramas obligaba a
prometer un refresco que no servía de nada.
Elegir el modo **es** la confirmación: para eso hay que nombrar una estrategia, y
quien la nombra después de leer que el CI está rojo ha decidido. No hace falta una
tercera tecla.

**El merge va pineado al commit que se leyó.** `--match-head-commit` en GitHub,
`--sha` en GitLab. Sin eso la forja integra el HEAD del momento, y entre el
refresco del inbox (60 s) y la pulsación la rama puede haber avanzado: se
integrarían commits que nadie revisó. Es el peor resultado posible de una acción
irreversible, y por eso cuando el forge no reporta el commit —un `diffHeadSha`
null en GitLab, o una respuesta que no lo trae— el merge **se niega** en vez de
salir sin pin.

**El aviso de auth lleva el motivo del adapter.** `bitbucket` responde
`not implemented in this version`, y antes eso se pintaba como
`not authenticated`: dos cosas que piden acciones opuestas, porque un token
inválido se arregla y una feature sin implementar no.

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
**nunca** lista ni borra worktrees ajenos. Cerrar la app no borra nada: no hay
borrado implícito al salir.

```sh
prdash worktrees                     # lista propia (ruta, rama, estado); marca huérfanos
prdash worktrees list                # idem, explícito
prdash worktrees remove <ruta>…      # borra SOLO lo pedido y solo si es de prdash
prdash worktrees remove --orphans    # borra en lote solo los huérfanos que marca `list`
prdash worktrees remove --orphans --dry-run   # imprime el lote exacto y no borra
```

`--orphans` reusa la misma fuente de verdad que `list` (el propio `Audit`), así
que borra **solo** lo que `list` marca como `orphaned`; con cero huérfanos
informa y sale con 0. `--dry-run` imprime el lote exacto por el mismo camino de
código, sin borrar nada, para poder ver la operación irreversible antes de
ejecutarla. `--orphans` es **excluyente** con las rutas explícitas: mezclar los
dos modos es un error de uso. Un huérfano cuyo `.git` ni siquiera declare un
gitdir (enlace corrupto o truncado) también se limpia, por ruta o en lote: no hay
repo que resolver y solo queda borrar su checkout.

La única limpieza implícita es al **mergear desde prdash**: cuando un `merge`
lanzado desde la app termina bien, se borra el worktree de ese ítem **solo si
está limpio**. Si tiene cambios sin commitear (incluidos archivos nuevos sin
trackear) se **conserva** y el aviso lo dice (`merged, but the worktree has
uncommitted changes — kept`). Si su estado de git no se puede comprobar, también
se **conserva**, con otro aviso (`worktree kept: could not read the worktree
status`). Ante la duda, nunca se borra. Ningún otro camino —approve, retarget, un
merge que no sale bien, el refresco que ve un PR mergeado fuera de prdash, cerrar
la app— borra nada.

El editor es el único que se ajusta mejor con `[tools].editor`, que es un atajo
para su base sin override:

```toml
[tools]
editor = "nvim ."   # por si no usas el `vi` → `nvim .` de tu shell
```

Funciona igual con la provisión nativa de Herdr (dentro de Herdr) y con git
directo (fuera): los guardas de ownership y de raíz gestionada se aplican en
ambas vías, así que una ruta propia fuera de la raíz se rechaza sin tocar nada.

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

`make check` es el equivalente local del gate de CI (jobs Build/Lint/Test):

```sh
make check   # build + lint + test (nunca instala)
make test    # go build ./... && go vet ./... && gofmt check && go test -race -count=1 ./...
make lint    # go vet + gofmt + golangci-lint v2.13.2 (pineado, vía go run)
make fmt     # formatea
make print   # comprueba el pipeline sin TUI
```

CI: `.github/workflows/ci.yml` corre en cada PR, en push a `main` y a mano
(Build, Lint y Test con resumen de cobertura);
`.github/workflows/mutation.yml` corre en cada PR y a mano (mutation testing
con gremlins, gate bloqueante sobre los mutantes Supervivientes del diff).

Diseño: `docs/planning/archive/0001-mvp/` (plan, comportamiento esperado,
contexto, resumen de cierre);
decisiones permanentes en `docs/adr/`; contrato de integración con Herdr en
`docs/research/herdr-0.9.1-contract.md`.
