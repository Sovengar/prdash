# ADR 0009 — Comentarios en inglés, y solo cuando explican el porqué

- Estado: aceptada
- Fecha: 2026-10-04

## Contexto

prdash había llegado a 4.896 líneas de comentarios en el código de producción y
14.524 en los tests: más líneas de comentario que de código en varios ficheros, y
`internal/tui/comments.go` tenía 287 líneas de comentario contra 232 de código.

El problema no era el volumen por sí mismo sino **qué** se escribía. La mayoría de
los comentarios explicaban cómo funciona el código, que es lo que el código ya dice
y lo que cambia solo cuando cambia el código. Una observación مثل «este bucle es un
`break` plano» no sale en un test: sale en el git log, y para entonces es ruido.

El resto sí era información real, pero enterrada: POR QUÉ se implementación de una
forma y no de la alternativa evidente. Casos que aparecieron repetidos al leer el
código:

- `shellSafe` no incluye `#` en su lista de caracteres seguros, porque al principio de
  palabra abre un comentario en el shell y una rama `git checkout -b '#123'` desaparecía
  del comando sin error visible.
- `gitcmd.Env` filtra las variables `GIT_*` de localización porque le ganan a
  `cmd.Dir`: con `GIT_DIR` puesto, `git config` escribe en ese repo da igual el
  directorio desde el que se ejecute, que es como se borra la rama equivocada.
- `FitCells` paga el alto a double, porque con celdas 1×2 una imagen 16:9 necesita 3,56
  columnas por línea y no 1,78, y sin el factor los dos commits de una fila se ven como
  una tira de elipses.
- Los guards que «no pueden dispararse» y se quitaron (el hueco interior del popup,
  `perRow <= 0` en `FitCells`, el `if` posterior de `centeredOrigin`), con la cuenta
  escrita para que nadie los vuelva a añadir.

Cada uno de esos es imposible de reconstruir leyendo el código. Es exactamente el
contenido que un comentario debe tener.

## Decisión

Dos reglas.

**Idioma: inglés.** El idioma del código es inglés, y un comentario en un idioma
distinto del resto del código obliga a cambiar de contexto para leerlo. Los ADRs, el
README y la documentación de usuario siguen en español, porque son los que lee el
operador, no quien lee el diff.

**Criterio: un comentario se queda solo si justifica el POR QUÉ.** Concretamente, si
explica una decisión, una restricción o una trampa que el código no puede decir por sí
solo. Se borra todo lo que describe el comportamiento: los doc comments que repiten el
nombre de la función, los comentarios de campo, los de «qué hace este bucle» y los que
explican un mecanismo de Go que quien lo lee ya conoce.

Y lo que se queda se comprime a **una línea**, o dos cuando la decisión tiene dos
caras. La prueba es de aritmética: una explicación que necesita diez líneas suele ser
una explicación de *qué*, no de *por qué*, y si de verdad es un *por qué* cabe en la
frase que dice la decisión.

## Consecuencias

- El código de producción pasa de 4.896 a ~1.000 líneas de comentario, y de 15.140 a
  ~11.000 líneas en total. Los tests, que estaban en 14.524, quedan fuera de este
  cambio; se tratan aparte.
- Un comentario no puede quedar desactualizado si dice una decisión y no un
  comportamiento: las decisiones cambian menos que el código.
- Se pierde lo que un comentario de *qué* añadía: orientación para quien entra en un
  fichero. Se acepta ese coste porque el git log cubre lo mismo y el código está
  partido en funciones con nombres que ya lo dicen.
- La regla se erosiona sola. Un criterio que depende de que alguien lo recuerde se
  deshace en el siguiente commit, así que vive aquí y en `AGENTS.md`, no en la
  costumbre.

## Alternativas descartadas

**Borrarlos todos.** Inviable: media decisión del código es incomprobable sin su
razón, y sin el comentario nadie la vuelve a escribir. Un código sin explicación de sus
trampas se repara dos veces.

**Dejarlos solo en los ficheros «complejos».** Convierte el criterio en "misurable por
fichero", que es subjective y produce el efecto contrario: el fichero que más lo
necesita es el que más comentarios tiene, así que acabaría sobrerrepresentado.

**Una línea por fichero en vez de por decisión.** Más corto y más fácil de leer, pero
obliga a elegir qué decisión merece el comentario, y esa elección se hace mal desde
fuera.