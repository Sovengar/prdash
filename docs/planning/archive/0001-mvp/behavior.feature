# prdash — comportamiento esperado del MVP (F1 + F2) y F3 como milestone.
#
# Fuente ÚNICA del comportamiento esperado. Sirve para que el usuario confirme
# que capturamos lo que quiere; NO es Cucumber (sin step definitions ni runner).
# El executor derivará de aquí tests reales (unit/integration) estilo BDD/ATDD.
#
# Gherkin en inglés; las descripciones van en español.

Feature: Inbox multi-forge de PRs/MRs y orquestador de review sobre Herdr

  Background:
    Given la config XDG de prdash define "roots", las forges a consultar y la cadencia de refresco
    And "gh" está autenticado en github.com y "glab" en el GitLab self-managed
    And ejecuto "prdash" dentro de una TUI

  # ─────────────────────────── F1 — Inbox cross-forge ───────────────────────────

  @F1 @inbox
  Scenario: El inbox muestra las tres secciones con datos de ambos forges
    Given hay PRs/MRs creados por mí, con review pedido o asignados, y menciones
    When la TUI termina el primer refresco
    Then veo las secciones "Creados por mí", "Review / asignados" y "Menciones"
    And cada sección muestra ítems de GitHub y del GitLab self-managed
    And cada ítem indica su forge y su host

  @F1 @inbox
  Scenario: "Creados por mí" solo incluye lo que abrí yo
    Given existen PRs abiertos por otras personas
    When consulto la sección "Creados por mí"
    Then solo aparecen ítems cuyo autor soy yo
    And los PRs ajenos no aparecen en esa sección

  @F1 @inbox
  Scenario: "Review / asignados" incluye review pedido y asignaciones
    Given me pidieron review de un PR en GitHub y estoy asignado a un MR en el GitLab self-managed
    When consulto la sección "Review / asignados"
    Then ambos ítems aparecen en esa sección
    And cada ítem indica si es review pedido o asignación

  @F1 @inbox
  Scenario: "Menciones" usa la fuente correcta por forge
    Given me mencionan en un PR de GitHub
    And me mencionan en un MR del GitLab self-managed
    When consulto la sección "Menciones"
    Then la mención de GitHub proviene de la búsqueda de menciones vía GraphQL
    And la mención de GitLab proviene de la API de Todos del GitLab
    And no veo menciones que no me incluyan

  @F1 @inbox
  Scenario: El ítem refleja el estado rico del forge sin salir de la TUI
    Given un PR de GitHub con "reviewDecision" y estado de checks
    And un MR del GitLab self-managed con estado de aprobación
    When el inbox pinta esos ítems
    Then el ítem de GitHub muestra su decisión de review y el estado de sus checks
    And el ítem de GitLab muestra su estado de aprobación
    And no necesito abrir el navegador para verlo

  @F1 @inbox
  Scenario: El mismo ítem no se duplica entre secciones o queries
    Given un PR que cumple más de una condición (p. ej. creado por mí y donde me mencionan)
    When el inbox consolida los resultados
    Then el PR aparece una sola vez, bajo la sección de mayor relevancia
    And la identidad se resuelve por forge, host, proyecto y número

  @F1 @detalle
  Scenario: Abrir el detalle de un ítem
    Given un ítem seleccionado en el inbox
    When pulso la tecla de detalle
    Then veo título, autor, ramas origen/destino, número y URL del ítem
    And veo su estado de review y de checks/aprobación
    And puedo volver al inbox sin perder la selección

  @F1 @refresco
  Scenario: Refresco manual actualiza y sella la hora
    Given el inbox ya mostró datos
    When pulso la tecla de refresco
    Then prdash vuelve a consultar ambos forges
    And el indicador "última actualización" refleja el momento del refresco

  @F1 @refresco
  Scenario: Refresco automático según la cadencia configurada
    Given la cadencia de refresco está configurada (por defecto 60s)
    When transcurre el intervalo sin interacción mía
    Then el inbox se refresca solo
    And el indicador "última actualización" se actualiza

  @F1 @refresco
  Scenario: Un refresco no pisa una acción en curso
    Given estoy ejecutando un approve/merge sobre un ítem
    When el refresco automático dispara
    Then el estado de mi acción en curso no se revierte
    And el resto de ítems sí se actualiza

  @F1 @degradacion
  Scenario: Una forge caída o sin auth no vacía el inbox
    Given glab no tiene token válido para gitlab.com (responde 401)
    When el inbox consulta los forges
    Then la sección de gitlab.com muestra un estado de error explícito
    And los ítems de GitHub y del GitLab self-managed siguen visibles
    And la TUI no aborta

  @F1 @degradacion
  Scenario: Distinguir "sección vacía" de "no se pudo consultar"
    Given una sección sin resultados en una forge que respondió bien
    And otra sección cuya forge devolvió un error
    When pinto el inbox
    Then la sección sin resultados dice que está vacía
    And la sección fallida dice que no se pudo consultar
    And nunca muestro "vacío" cuando en realidad hubo un error

  @F1 @degradacion
  Scenario: Bitbucket está presente pero no operativo
    Given la config habilita el adapter de Bitbucket
    When el inbox intenta consultar Bitbucket
    Then Bitbucket se reporta como no soportado en esta versión
    And prdash no realiza ninguna llamada de red a Bitbucket

  @F1 @acciones
  Scenario: Approve/merge rápido desde el inbox
    Given un ítem revisable en GitHub o en el GitLab self-managed
    When ejecuto approve (o merge cuando aplica) desde el inbox
    Then la acción se realiza vía "gh"/"glab" directo, o se delega en "tuicr" cuando corresponde
    And el ítem refleja el nuevo estado tras refrescar

  @F1 @acciones
  Scenario: El ítem cambió en el forge entre refresco y acción
    Given un ítem que quedó cerrado o mergeado tras el último refresco
    When intento una acción sobre él
    Then prdash muestra un error claro de conflicto/no encontrado
    And refresca el ítem en vez de dejarlo en un estado inconsistente

  # ─────────────────────── F2 — Orquestador de review ───────────────────────

  @F2 @worktree
  Scenario: Worktree desde un repo ya local
    Given un ítem cuyo repo ya está clonado y resuelto en un root configurado
    When activo "montar review" sobre el ítem
    Then prdash crea un worktree de la rama del PR/MR
    And el worktree queda registrado como el review activo de ese ítem

  @F2 @worktree
  Scenario: Repo no clonado se clona en bare y se saca el worktree del clon
    Given un ítem cuyo repo no está clonado en ningún root
    When activo "montar review"
    Then prdash crea un clon bare en "~/.local/share/prdash/repos/<forge>/<host>/<owner>/<repo>" (configurable)
    And crea el worktree de la rama del PR/MR desde ese clon bare
    And el destino de clon y de worktrees es configurable

  @F2 @worktree
  Scenario: PR de fork cuya rama no existe en origin
    Given un ítem de fork donde la rama de origen no es una rama normal de origin
    When monto el review
    Then prdash obtiene el ref del PR ("refs/pull/N/head" en GitHub o "refs/merge-requests/N/head" en GitLab)
    And crea una rama local de trabajo a partir de ese ref
    And crea el worktree sobre esa rama local

  @F2 @worktree
  Scenario: Reusar el worktree existente de un ítem
    Given ya monté el review de un ítem antes
    When lo monto de nuevo
    Then prdash reutiliza el worktree existente
    And no crea un worktree duplicado para el mismo ítem

  @F2 @worktree
  Scenario: Varios PRs del mismo repo no chocan
    Given monto el review de dos ítems distintos del mismo repo
    Then cada ítem tiene su propio worktree, en una ruta distinta
    And ambos worktrees pueden coexistir

  @F2 @worktree @degradacion
  Scenario: Sin permisos de clon o fetch, el fallo es claro y no deja basura
    Given un ítem cuyo repo es privado y no tengo permisos de clon/fetch
    When intento montar el review
    Then prdash muestra un error claro con la causa
    And no deja un clon bare ni un worktree a medias

  @F2 @layout
  Scenario: El layout de review monta los tres panes sobre el worktree
    Given un ítem con worktree creado
    And estoy dentro de Herdr
    When se abre el layout de review
    Then aparece un pane con TUICR sobre ese PR/MR
    And aparece un pane con Hunk mostrando el diff
    And aparece un pane con un agente opencode
    And los tres panes trabajan sobre el worktree del ítem

  @F2 @layout @degradacion
  Scenario: Una herramienta ausente no tumba el layout
    Given el binario de Hunk no está instalado
    When abro el layout de review
    Then el pane de Hunk se omite con un aviso
    And los demás panes siguen operativos

  @F2 @layout
  Scenario: Link handler de URLs de PR
    Given estoy dentro de Herdr con una URL de PR visible
    When hago Ctrl+click sobre la URL
    Then prdash resuelve esa URL a un ítem del inbox
    And monta (o enfoca) el review de ese ítem

  @F2 @loop
  Scenario: El loop de comentarios llega al agente
    Given el layout de review abierto con un comentario pendiente
    When escribo un comentario en TUICR o en Hunk
    Then el agente opencode puede leer ese comentario
    And aplica el cambio pedido en el worktree
    And yo veo el resultado del agente sin cambiar de herramienta

  @F2 @degradacion
  Scenario: Fuera de Herdr, F1 sigue y F2 se informa
    Given ejecuto prdash sin "HERDR_ENV=1"
    When abro la TUI
    Then el inbox (F1) funciona con normalidad
    And la acción de montar review informa que requiere Herdr
    And prdash no se cuelga ni deja procesos huérfanos

  # ─────────────── F3 — Auto-review con gate y allowlist (post-MVP) ───────────────

  @F3
  Scenario: Repo no allowlisted no se auto-aprueba
    Given el modo auto-review activado
    And el repo del ítem no está en la allowlist
    When el agente termina el análisis sin findings críticos
    Then prdash NO aprueba el PR/MR
    And deja constancia del motivo (allowlist)

  @F3
  Scenario: Auto-aprobación con el gate satisfecho
    Given el modo auto-review activado con el gate habilitado
    And el repo del ítem está en la allowlist
    When el agente termina el análisis con 0 findings críticos
    Then prdash aprueba el PR/MR vía el forge correspondiente
    And notifica por Herdr que aprobó

  @F3
  Scenario: Un análisis fallido o parcial nunca aprueba
    Given el modo auto-review activado
    When el análisis del agente falla o queda incompleto
    Then prdash no aprueba el PR/MR
    And reporta que el análisis no fue concluyente

  @F3 @degradacion
  Scenario: Notificación de Herdr sin Herdr
    Given Herdr no está disponible
    When ocurre un evento que debía notificarse
    Then prdash no rompe y registra el evento localmente
