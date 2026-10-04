# ADR 0008 — el dispatch de acción desconocida se corta antes de releer el ítem

- **Estado**: Accepted
- **Fecha**: 2026-10-03
- **Decisor**: usuario (buble)
- **Alcance**: `internal/forge/`, `internal/tui/`, `internal/review/executor/`
- **Depende de**: ADR 0006 (retarget por la API), que es lo que añadió el tercer
  `ActionKind` y dejó el `default` del dispatch como el único sitio donde una
  acción podía colarse sin ejecutarse.

## Contexto

`forge.RunAction` despacha las acciones rápidas del inbox con un `switch` de dos
ramas —`approve` y `merge`— y un `default` que devolvía `nil`. Eso era correcto
mientras las acciones rápidas fueran exactamente dos.

El problema es qué significa `nil` más abajo. `classifyAction` —que traduce los
warnings de una acción a `(ok, conflicto, permiso, motivo)`— **trata una lista
vacía como "no hubo ningún warning"**, que es su forma de decir que la acción
salió bien. Así que el `default` no era una contención: era un **informe de
éxito de algo que no se hizo**.

El síntoma en producción era imposible de ver por dos razones que se refuerzan:

1. La TUI filtra por `canActionOn`, que solo deja pasar `approve` y `merge`. La
   acción desconocida no llegaba a `RunAction`.
2. `retarget` —el tercer `ActionKind`, el del ADR 0006— tiene su propio camino
   y nunca pasa por `RunAction`.

Es decir: el bug estaba detrás de dos guards que funcionan, más uno que se
consumió al añadir la tercera acción. No hay ningún test que lo revelara porque
no hay ningún camino que lo alcance en verde.

## Alternativas consideradas

### A. Dejarlo, porque los guards de arriba lo tapan

Es lo que estaba. Se descarta por una razón concreta: **una contención que
informa de éxito no contiene.** El día que un `ActionKind` nuevo se enrute por
`RunAction` sin añadir su rama, el usuario ve "hecho" en la cabecera sobre una
acción que no ocurrió, y no hay nada en la salida que lo contradiga. El coste del
arreglo es de un `if`; el coste del fallo es un approve fantasma.

### B. Que el `default` devuelva un warning `unsupported`

Descartada. Sería correcto en cuanto al resultado (`classifyAction` lo mapea a
`perm`) y más pequeño de escribir, pero tiene dos defectos:

- **Cuesta un viaje al forge.** `runOn` empieza releyendo el estado del ítem, así
  que despachar una acción imposible hace una llamada de ida y vuelta a GitHub
  para descubrir que no iba a hacer nada.
- **Clasifica mal.** `unsupported` significa "el forge no tiene esta
  operación". Aquí no falta ninguna operación: falta el despacho. Que la TUI
  registre el ítem como denegado para siempre sería **permanente y equivocado**
  —un Kind nuevo reparado al reiniciar prdash dejaría todos esos ítem con la
  acción deshabilitada.

### C. Cortar en la entrada de `RunAction` — elegida

El corte va antes de `runOn`, y el `Outcome` se construye ahí. Elimina el
`default` del `exec` en vez de darle una respuesta nueva.

## Decisión

`RunAction` valida el `ActionKind` antes de hacer nada. Si no es `approve` ni
`merge`, devuelve un `Outcome` con `OK: false`, **sin banderas** y con un motivo
canónico que nombra la acción.

El `default` del `exec` se borra. No se deja como segunda red: dos sitios que
dicen lo mismo divergen, y un `default` inalcanzable es código que se lee como
si protegiera algo.

El corte va antes de `runOn` y no dentro del `exec` por dos razones que se pagan en
cada llamada: `runOn` relee el ítem del forge, y la respuesta no viene de un
forge —pasarla por `classifyAction`, que clasifica respuestas de forge, sería
meterla en una categoría que no le corresponde.

### Por qué sin banderas

`Outcome` tiene tres banderas y ninguna describe esto:

- `Conflict` es "el ítem cambió en el forge, refresca". El ítem no ha cambiado.
- `Perm` es "esta acción está deshabilitada para este ítem". Es permanente y
  equivocado, y es la opción B.
- `Unmergeable` es específico de merge.

Es un fallo de **quién llamó**, no del ítem ni de la sesión. Añadir una cuarta
bandera sería estado nuevo en el `Outcome` que solo este camino pondría, y su
único consumidor sería el mensaje —que ya está. Con `OK: false` y un motivo, la
TUI enseña lo que corresponde: no se hizo nada, y esto es por qué.

### El `default` que se borra

El `switch` de dos ramas con guarda arriba es más honesto que un `switch` con
`default` que nunca se alcanza: el `default` documentaba una tolerancia que el
código ya no tiene.

## Consecuencias

- `RunAction` con un `ActionKind` desconocido es observable: `OK: false` y un
  motivo que nombra la acción. No hay ninguna forma de que informe de éxito sin
  haber hecho nada.
- Un `ActionKind` nuevo se ve en el mensaje en cuanto se enruta por aquí, en vez
  de aparecer como "hecho".
- El `exec` de `runOn` ya no tiene `default`. Si alguien lo enruta sin pasar por
  la guarda de la entrada, el tipo le impide dispatchar: `kind` solo tiene dos
  valores válidos en esa posición.
- El coste del camino bueno es un `if` que compara dos cadenas.

## Alternativa para el futuro

Si algún día `RunAction` tiene que despachar más de dos acciones —un
`request-changes`, un `dismiss`—, la guarda pasa a ser una lista de `ActionKind`
admitidos y el `switch` vuelve a ser un `switch`. La regla —cortar antes de
releer, y no informar de éxito de nada— no cambia.