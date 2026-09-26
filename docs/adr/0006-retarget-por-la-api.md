# ADR 0006 — la rama destino se cambia por la API, no por el comando de edición

- **Estado**: Accepted
- **Fecha**: 2026-09-27
- **Decisor**: usuario (buble)
- **Alcance**: acción de retarget, `internal/forge/github/`, `internal/forge/gitlab/`,
  `internal/forge/tool/`
- **Depende de**: nada. No cierra ni reabre ningún ADR anterior.
- **Sustituye a**: nada.

## Contexto

prdash habla con los forges por CLI: `gh pr review`, `gh pr merge`, `glab mr
approve`, `glab mr merge`. Para la nueva acción de **cambiar la rama destino** del
PR/MR, cada CLI tiene el comando que documenta:

- GitHub: `gh pr edit <n> -B <rama>`
- GitLab: `glab mr update <n> --target-branch <rama>`

La primera opción es la que toca: es la documentada, es legible y encaja con el
resto del adapter, que ya usa `gh pr …` y `glab mr …` para todo lo demás.

Probada contra un PR real (`Sovengar/prdash#13`), **`gh pr edit` no funciona**:

```
$ gh pr edit 13 -R Sovengar/prdash -B una-rama-que-no-existe
GraphQL: Projects (classic) is being deprecated in favor of the new Projects
experience, see: … (repository.pullRequest.projectCards)
```

Falla **antes de tocar nada**. No es un problema de permisos ni del PR: es la query
con la que `gh` mira si el PR está en un proyecto, y `projectCards` revienta en los
repos donde ese campo da error. El mismo `gh` con la API REST hace la operación:

```
$ gh api -X PATCH repos/Sovengar/prdash/pulls/13 -f base=una-rama-que-no-existe
gh: Validation Failed (HTTP 422)          # y el motivo va en el cuerpo JSON
```

GitLab no está roto en este sentido, pero `glab mr update` es un **comando de
edición**: su razón de ser es abrir título y descripción en el editor. Con un flag
de campo abierto, esa puerta se entreabre. En un subproceso con `stdin` en
`/dev/null` no se cuelga —falla—, y un fallo por un editor que el usuario no ve es el
peor género de avería: no dice nada de qué pasó.

## Alternativas consideradas y descartadas

- **`gh pr edit --base`, cuando `gh` lo arregle.** Es lo que acabaría haciendo
  cualquiera que leyera la documentación. Descartado hoy porque no funciona, y sin
  una señal de que se haya arreglado: si algún día lo hace, el cambio es una línea
  y un test que ya dice qué forma tiene el argv bueno.
- **`glab mr update --target-branch`.** Descartado por lo del editor. Además, glab
  reinterpreta el método por defecto según los flags que le pases, que es el mismo
  género de trampa que ya está documentado en el merge de este repo.
- **GraphQL para GitHub.** La mutación `updatePullRequest` necesita el node ID del
  PR y la consulta que lo trae ya devuelve otra cosa; el PATCH hace lo mismo en una
  petición, sin campos que se puedan retirar.
- **CRUD directo con `net/http` y el token.** Descartado de entrada: el resto del
  adapter habla con las CLIs y autenticarse por su cuenta duplicaría lo que `gh` y
  `glab` ya hacen bien (hosts de enterprise, `GH_TOKEN`, `glab` con varios hosts).

## Decisión

**El retarget va por la API REST de cada forge**, y el resto de acciones sigue por
los comandos de edición de cada CLI:

- GitHub: `gh api -X PATCH repos/<o>/<r>/pulls/<n> -f base=<rama>`
- GitLab: `glab api -X PUT projects/<o%2F<r>/merge_requests/<iid> -f target_branch=<rama>`

El criterio que separa los dos casos no es "la API es mejor", que no es verdad en
general. Es: **un comando de edición cuyo editor puede abrirse no vale para un
subproceso**, y **un comando que hoy falla sin decir por qué tampoco**. Ambos
fallan de forma distinta, pero en los dos casos lo que falla es la manera de
decirlo, no la operación, y la API dice lo mismo en una línea.

El listado de ramas que alimenta el buscador va por la API también, y con la
paginación explícita: `?per_page=100` + `--paginate`, porque un subconjunto de las
ramas dejaría fuera el destino buscado sin avisar. En GitHub el filtrado se hace
con `--jq '.[].name'`; en GitLab **no**, porque `glab api` no tiene `--jq`, y se lee
el NDJSON de `--output ndjson`.

### El motivo del rechazo se lee del cuerpo, no de stderr

Esto salió del mismo trabajo y es la mitad del valor de la decisión. Cuando la
llamada falla con código distinto de cero, las CLIs ponen a stderr **una línea con
el argv entero** y el motivo de verdad va en el cuerpo JSON:

```
stderr: gh api -X PATCH repos/o/r/pulls/1 -f base=x: gh: Validation Failed (HTTP 422)
cuerpo: {"message":"Validation Failed","errors":[{"message":"Proposed base branch 'x' was not found", …}]}
```

`tool.Run` cortaba stderr y se quedaba con lo primero, así que el motivo que veía
el usuario era el comando que falló. Se añade `tool.APIMessage`, que prefiere
`errors[].message` sobre `message` —GitHub escribe el genérico en el primero y el
detalle en el segundo, así que al revés se enseñaría "Validation Failed"— y cae a
`message`, que es como lo manda GitLab.

Y con el motivo a la vista se puede clasificar bien: ese 422 es
`tool.Kind == "validation"`, una clase que **no existía**. Antes caía en `network`,
que `classifyAction` traduce a "forge conflict", que promete un refresco que no
puede arreglar un nombre de rama que no existe. Tampoco es `permission` —eso
registraría el ítem como denegado y le quitaría la acción para siempre— ni
`notfound` —un ítem que desapareció y una rama que no existe se confunden, y solo lo
segundo se arregla escribiendo otro nombre—.

## Consecuencias

**Positivas**

- La acción funciona, que era el objetivo, y funciona por la vía documentada por la
  API en vez de por la que `gh` documenta y no cumple.
- El rechazo dice el motivo: `error: Proposed base branch 'x' was not found`.
- Una clase de fallo nueva y nombrada en vez de un `network` que no significaba
  nada.
- El `422` deja de clasificarse mal **en todas las acciones**, no solo en el
  retarget: `kindForHTTP` es compartido.

**Negativas / costes**

- **Se pierde la validación de la CLI.** `gh pr edit` y `glab mr update` comprueban
  cosas antes de enviar que la API no comprueba (permisos, ramas protegidas,
  estados del MR). En la práctica el forge devuelve un 4xx con el motivo, que es lo
  bastante, pero el mensaje es del servidor y no de la CLI: puede ser más crudo.
- **El proyecto de GitLab hay que urlencodarlo a mano** (`grp%2Fproj`). `glab` lo
  hace solo con el flag `-R`; por API hay que hacerlo, y equivocarse produce un 404
  en un sitio que no existe y no dice por qué.
- **El motivo del forge se enseña tal cual**, en inglés, y sin la traducción que sí
  tienen los motivos canónicos (`ErrMissingHeadSHA`, `unmergeable`,
  `SelfReviewReason`). Se acepta: inventar una traducción para un texto que puede
  ser cualquiera sería peor que enseñarlo.
- Si `gh pr edit` se arregla, esto es deuda técnica que se paga a sabiendas. Está
  documentado aquí y en el README para que no parezca un descuido.
