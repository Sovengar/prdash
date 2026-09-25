# F3 — Auto-review con gate y allowlist (milestone, NO implementado)

- **Estado**: milestone post-MVP. Diseño aprobado en `behavior.feature`; **no se
  implementa** en la feature 0001.
- **Padre**: `docs/planning/archive/0001-mvp/plan.md` (fase F3) y `behavior.feature`
  (bloque `@F3`, que es el comportamiento esperado y sirve de criterio futuro).
- **Decisión**: la release actual termina al abrir los panes de review con su
  cwd, env y argv correctos. Aprobar o comentar es responsabilidad del usuario
  y de las herramientas/agente del loop, no de prdash.

## Objetivo

Que prdash pueda, opcionalmente, dejar que un agente complete el bucle de
review y **apruebe** un PR/MR sin intervención, pero solo cuando se cumplan
todas las condiciones de seguridad. La aprobación automática es una capacidad
de alto riesgo: el diseño la trata como opt-in, auditable y conservadora ante
cualquier duda.

## Flujo previsto (alto nivel)

1. El usuario habilita el modo auto-review en la config y declara qué repos son
   elegibles.
2. El ítem del inbox se monta como review normal (F2): worktree + panes.
3. El agente (opencode) analiza el diff en el worktree y produce un resultado
   con un veredicto y sus findings.
4. El **gate** decide si el resultado es concluyente y sin hallazgos críticos.
5. Solo si el gate pasa **y** el repo está en la **allowlist**, prdash ejecuta la
   aprobación vía el forge correspondiente (`gh`/`glab`).
6. prdash notifica el resultado por Herdr; sin Herdr, registra el evento
   localmente y sigue.

## Reglas duras (no negociables)

- **Allowlist por repo.** Un repo que no esté en la allowlist **nunca** se
  auto-aprueba, aunque el análisis sea limpio. Se deja constancia del motivo.
- **Gate explícito.** La auto-aprobación requiere `autoreview.enabled` y el gate
  habilitado, además de 0 findings críticos.
- **Dry-run por defecto.** Con el modo activado pero sin el gate, prdash
  registra qué *habría* aprobado y no ejecuta la acción. Ninguna ruta de código
  aprueba por defecto.
- **Nunca aprobar con análisis fallido o parcial.** Un análisis que falla, se
  interrumpe o queda incompleto se reporta como no concluyente y jamás se
  traduce en aprobación.
- **Degradación limpia.** Sin Herdr disponible, un evento que debía notificarse
  no rompe el proceso: se registra localmente.
- **Reversibilidad y trazas.** Toda decisión (aprobada, omitida por allowlist,
  omitida por análisis no concluyente) debe quedar en un registro consultable.
  La aprobación no borra evidencia: el resultado del análisis se conserva.

## Configuración reservada

Estas claves ya se parsean en `internal/config` (sin consumidor todavía); F3 las
usaría tal cual:

| Clave | Tipo | Default | Uso |
|---|---|---|---|
| `autoreview.enabled` | bool | `false` | Activa el modo. Por sí solo no aprueba: hace falta el gate. |
| `autoreview.allowlist` | `[]string` | `[]` | Repos (`host/owner/repo`) elegibles para auto-aprobar. |

## Criterios de aceptación futuros

Los escenarios del bloque `@F3` de `behavior.feature` son la fuente de verdad
para cuando se implemente. Se resumen aquí para trazabilidad:

1. Repo no allowlisted no se auto-aprueba y se deja constancia del motivo.
2. Con el gate satisfecho y el repo allowlisted, el análisis con 0 findings
   críticos aprueba vía el forge y notifica por Herdr.
3. Un análisis fallido o parcial nunca aprueba y reporta que no fue concluyente.
4. Sin Herdr, la notificación degrada sin romper el proceso y se registra.

## Fuera de alcance de este milestone documentado

- Implementación de código, tests o adapters de forge para la aprobación.
- Políticas de gate más ricas (pesos por severidad, excepciones por autor,
  ventanas de tiempo). Se decidirán al implementar, sobre el gate mínimo.
- Ejecución del agente o gestión de sus credenciales: prdash no es dueño del
  loop de comentarios (`plan.md`, decisión clave 3).
