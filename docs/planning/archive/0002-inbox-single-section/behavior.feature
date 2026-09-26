# prdash — comportamiento esperado: Inbox de una sola sección.
#
# Fuente ÚNICA del comportamiento esperado de esta feature. Sirve para que el
# usuario confirme que capturamos lo que quiere; NO es Cucumber (sin step
# definitions ni runner). El executor derivará de aquí tests reales
# (unit/integration) estilo BDD/ATDD.
#
# Gherkin en inglés; las descripciones van en español.
#
# Contexto del diseño (ADR 0002): el motivo de mostrar una sola sección a la vez
# es conservar y aprovechar el prefijo de ruta común. Con una única sección
# visible todas las filas comparten sección, así que el prefijo se calcula sobre
# la sección activa y se sigue mostrando, aunque desaparezca el header interno
# con título y conteo.

Feature: Inbox de una sola sección con leyenda de conteos y prefijo de ruta común

  Background:
    Given hay ítems en las tres secciones del inbox (Mine = authored, Assigned = review, Mentioned = mentions)
    And las etiquetas de sección son "Mine", "Assigned" y "Mentioned"
    And ejecuto "prdash" dentro de una TUI

  # ─────────────────────── Sección activa y estado inicial ───────────────────────

  @inbox @navegacion
  Scenario: Al abrir, la sección activa es Assigned
    Given abro la TUI
    When se pinta el Inbox por primera vez
    Then la sección activa es "Assigned"
    And solo se ven en la lista los ítems de Assigned
    And los ítems de Mine y de Mentioned no aparecen en la lista

  @inbox @navegacion
  Scenario: Solo se pinta una sección a la vez
    Given la sección activa es "Assigned"
    When miro el Inbox
    Then la lista contiene únicamente ítems de Assigned
    And no hay una cabecera interna de sección con el título y el conteo dentro de la lista

  @inbox @navegacion
  Scenario: tab cicla Assigned → Mentioned → Mine → Assigned
    Given la sección activa es "Assigned"
    When pulso "tab"
    Then la sección activa pasa a ser "Mentioned" y la lista muestra sus ítems
    When pulso "tab"
    Then la sección activa pasa a ser "Mine" y la lista muestra sus ítems
    When pulso "tab"
    Then la sección activa vuelve a ser "Assigned"
    And el ciclo se repite indefinidamente en ese orden

  @inbox @navegacion
  Scenario: tab cicla también cuando una sección está vacía
    Given la sección "Mentioned" no tiene ítems
    And la sección activa es "Assigned"
    When pulso "tab"
    Then la sección activa pasa a ser "Mentioned" igualmente
    And la lista muestra su estado vacío

  @inbox @keybindings
  Scenario: La tecla de ciclo sale de la config, no de un "tab" cableado
    Given la config reasigna "section-next" a otra tecla
    When pulso esa tecla
    Then cambia la sección activa según el ciclo
    And "tab" deja de cambiar de sección

  # ───────────────────────────── Leyenda de conteos ─────────────────────────────

  @inbox @leyenda
  Scenario: La leyenda del borde superior izquierdo sustituye al título "Inbox"
    Given las secciones tienen 9, 4 y 0 ítems respectivamente
    When se pinta el Inbox
    Then el borde superior izquierdo de la caja del Inbox muestra "Mine (9) · Assigned (4) · Mentioned (0)"
    And ya no se muestra el título "Inbox"

  @inbox @leyenda
  Scenario: La sección activa se resalta en la leyenda
    Given la sección activa es "Assigned"
    When se pinta la leyenda
    Then "Assigned" y su conteo se muestran resaltados (color/negrita)
    And "Mine" y "Mentioned" se muestran atenuados

  @inbox @leyenda @refresco
  Scenario: Los conteos de la leyenda reflejan la sección deduplicada
    Given un ítem que aparecería en más de una sección
    When se pinta la leyenda
    Then ese ítem cuenta una sola vez, en la sección de mayor autoridad
    And cada conteo es el número de ítems que esa sección tiene en la lista

  # ────────────────────────── Prefijo de ruta común (ADR 0002) ──────────────────────────

  @inbox @prefijo
  Scenario: El prefijo común de la sección activa se sigue mostrando
    Given la sección activa tiene varios ítems cuyo proyecto comparte "APPCITTI/vsocial/backend"
    When se pinta la lista
    Then el prefijo de ruta común "APPCITTI/vsocial/backend/" aparece una sola vez en la vista
    And las celdas ITEM de las filas muestran solo el sufijo (p. ej. "api-gateway#100"), sin el prefijo

  @inbox @prefijo
  Scenario: El prefijo mostrado es el de la sección activa
    Given "Mine" y "Assigned" tienen prefijos de ruta comunes distintos
    When la sección activa es "Assigned"
    Then la vista muestra el prefijo común de "Assigned"
    And no muestra el de "Mine"
    When pulso "tab" hasta "Mine"
    Then la vista pasa a mostrar el prefijo común de "Mine"

  @inbox @prefijo
  Scenario: Una sección sin prefijo común no inventa uno
    Given la sección activa tiene un solo ítem, o sus ítems no comparten directorio
    When se pinta la lista
    Then no se muestra un prefijo común
    And cada celda ITEM lleva la ruta completa, recortada por la cola si no cabe

  @inbox @prefijo
  Scenario: El prefijo no rompe el ancho de la tabla
    Given la sección activa tiene un prefijo de ruta común
    When se pinta la lista a cualquier ancho de terminal
    Then el prefijo se recorta si no cabe, sin desalinear las columnas
    And las celdas ITEM conservan la hoja del proyecto y el "#número"

  # ───────────────────────────── Posición por sección ─────────────────────────────

  @inbox @cursor
  Scenario: Cada sección recuerda su cursor y su scroll
    Given en "Assigned" muevo el cursor y desplazo la lista
    When pulso "tab" a "Mentioned" y muevo su cursor a otro sitio
    And vuelvo con "tab" a "Assigned"
    Then "Assigned" recupera el cursor y el scroll que tenía
    When vuelvo a "Mentioned"
    Then "Mentioned" recupera su propio cursor y scroll

  @inbox @cursor @refresco
  Scenario: Un refresco conserva la posición de cada sección
    Given tengo una posición en cada sección
    When termina un refresco del inbox
    Then la sección activa conserva su cursor y su scroll, acotados al nuevo contenido

  # ───────────────────────── Estados de la sección activa ─────────────────────────

  @inbox @estado
  Scenario: La sección activa vacía se marca como vacía
    Given la sección activa no tiene ítems y su consulta no falló
    When se pinta la lista
    Then la lista muestra "(empty)"
    And la leyenda muestra 0 para esa sección

  @inbox @paginacion
  Scenario: El "loading more…" corresponde a la sección activa
    Given la sección activa tiene páginas pendientes
    When se pinta la lista
    Then la lista muestra "loading more…"
    And si la sección que paginaba no es la activa, su indicador no se muestra
    And al volver a esa sección, el indicador vuelve a mostrarse

  @inbox @degradacion
  Scenario: Los avisos de consulta se muestran para la sección activa
    Given la consulta de una sección falló o devolvió datos parciales
    And esa sección es la activa
    When se pinta la lista
    Then la lista muestra su aviso "⚠ <forge>: could not be queried (…)"
    And si esa sección no es la activa, su aviso no se pinta (sigue visible su conteo en la leyenda)

  # ─────────────────────────── No-regresión de otros modos ───────────────────────────

  @inbox @print
  Scenario: El modo --print no cambia
    When ejecuto "prdash --print"
    Then imprime las tres secciones con sus nombres largos ("Created by me", "Review / assigned", "Mentions")
    And cada sección lista sus ítems en el orden de siempre

  @inbox @leyenda
  Scenario: En un terminal estrecho la leyenda se trunca sin romper la caja
    Given un ancho de terminal menor que la leyenda completa
    When se pinta el Inbox
    Then la leyenda se recorta por la derecha
    And la línea superior sigue midiendo exactamente el ancho de la caja
