# prdash — comportamiento esperado: prefijo de ruta seleccionable y toggleable.
#
# Fuente ÚNICA del comportamiento esperado de esta feature. No es Cucumber (sin
# step definitions ni runner). El executor derivará de aquí tests reales
# (unit/integration) estilo BDD/ATDD.
#
# Gherkin en inglés; las descripciones van en español.
#
# Contexto de diseño: el ADR 0002 partió la ruta de ITEM en un prefijo común (en
# una línea fija, desde el ADR 0004) y un sufijo por celda. El ADR 0004 dejó
# abierta la costura de poder alternarlo. Esta feature la cierra con TRES modos
# ciclados por una tecla, global y sin persistir.
#
# Sobre el hint: la barra de atajos se deriva de `hintOrder` (config), pero la
# etiqueta de `p` tiene que nombrar el modo ACTUAL, que es estado de la TUI. La
# costura elegida es que `Config.Hints()` reciba ese estado: `hintOrder` sigue
# siendo la fuente de lista, orden y etiqueta por defecto, y el rebind de
# `[keybindings]` sigue siendo genérico. Estos escenarios fijan ese contrato.

Feature: Prefijo de ruta seleccionable y toggleable en la columna ITEM

  Background:
    Given la sección activa "Assigned" tiene ítems de los repos "APPCITTI/vsocial/backend/api-gateway#100" y "APPCITTI/vsocial/web-app#101"
    And ejecuto "prdash" dentro de una TUI
    And el modo de prefijo es "common" (el valor por defecto)

  # ──────────────────────────── Ciclo de modos ────────────────────────────

  @inbox @prefijo @keybindings
  Scenario: p cicla common → full → leaf → common
    Given el modo de prefijo es "common"
    When pulso "p"
    Then el modo de prefijo pasa a ser "full"
    When pulso "p"
    Then el modo de prefijo pasa a ser "leaf"
    When pulso "p"
    Then el modo de prefijo vuelve a ser "common"
    And el ciclo se repite indefinidamente en ese orden

  @inbox @prefijo @keybindings
  Scenario: La tecla de ciclo sale de la config, no de un "p" cableado
    Given la config reasigna "prefix-mode" a otra tecla
    When pulso esa tecla
    Then el modo de prefijo cicla igual
    And "p" deja de cambiar el modo de prefijo

  @inbox @prefijo @keybindings
  Scenario: p no dispara ninguna otra acción
    Given hay un ítem seleccionado y la TUI está en su estado normal
    When pulso "p"
    Then no se aprueba, no se mergea, no se monta review, no se refresca y no se sale
    And el merge no se arma

  @inbox @prefijo @navegacion
  Scenario: El modo es global y no lo reinicia el cambio de sección
    Given el modo de prefijo es "leaf"
    When pulso "tab" hasta "Mine" y de vuelta a "Assigned"
    Then el modo de prefijo sigue siendo "leaf"
    And el cursor y el scroll de cada sección se siguen recordando

  @inbox @prefijo @persistencia
  Scenario: El modo no se persiste entre ejecuciones
    Given elijo el modo "full" con "p"
    When cierro la TUI y vuelvo a abrirla
    Then el modo de prefijo es "common"

  # ────────────────────────────── Modo common ──────────────────────────────

  @inbox @prefijo
  Scenario: common muestra el prefijo común en su línea y el sufijo en la celda
    Given el modo de prefijo es "common"
    When se pinta la lista
    Then una línea atenuada al inicio del cuerpo declara el prefijo común "APPCITTI/vsocial/"
    And las celdas ITEM muestran solo el sufijo ("backend/api-gateway#100"), sin el prefijo
    And el prefijo aparece una sola vez en la vista

  @inbox @prefijo
  Scenario: common es el comportamiento de siempre, sin cambios respecto a hoy
    Given el modo de prefijo es "common"
    When se pinta la lista con cualquier conjunto de ítems
    Then la vista es idéntica a la de antes de esta feature

  # ─────────────────────────────── Modo full ───────────────────────────────

  @inbox @prefijo
  Scenario: full no pinta línea de prefijo y pone la referencia completa en la celda
    Given el modo de prefijo es "full"
    And la sección activa tiene los ítems "acme/one#7" y "other/two#8", cuyas referencias caben
    When se pinta la lista
    Then no hay ninguna línea de prefijo en el cuerpo de la lista
    And cada celda ITEM lleva la referencia completa, sin recortar: "acme/one#7" y "other/two#8"

  @inbox @prefijo @recorte
  Scenario: En full, lo que no cabe se recorta por la cola y sobrevive el #número
    Given el modo de prefijo es "full"
    And la sección activa es la de Background, con "APPCITTI/vsocial/backend/api-gateway#100"
    When se pinta la lista
    Then la celda ITEM no cabe entera y se recorta por la izquierda con "…"
    And la celda es "…/vsocial/backend/api-gateway#100": termina en "#100" y conserva la hoja del proyecto y el número

  @inbox @prefijo @layout
  Scenario: full recupera la línea de alto que ocupaba el prefijo
    Given el modo de prefijo es "full"
    And la sección activa tiene prefijo común
    When se pinta la lista
    Then el cuerpo de la lista tiene una línea menos que en modo "common"
    And la caja del inbox no encoge: sigue ocupando el alto reservado

  # ─────────────────────────────── Modo leaf ───────────────────────────────

  @inbox @prefijo
  Scenario: leaf no pinta línea de prefijo y pone solo la hoja del proyecto
    Given el modo de prefijo es "leaf"
    When se pinta la lista
    Then no hay ninguna línea de prefijo en el cuerpo de la lista
    And cada celda ITEM lleva el último segmento del proyecto más "#n" ("api-gateway#100", "web-app#101")

  @inbox @prefijo @layout
  Scenario: leaf da la máxima densidad de la columna ITEM
    Given el modo de prefijo es "leaf"
    When se pinta la lista
    Then la columna ITEM es más estrecha que en "full"
    And sigue reservando al menos el ancho mínimo para no pegarse a la columna vecina

  @inbox @prefijo
  Scenario: leaf no puede desambiguar dos repos con la misma hoja
    Given el modo de prefijo es "leaf"
    And la sección activa tiene los ítems "acme/one#7" y "other/one#8", que no comparten prefijo
    When se pinta la lista
    Then las dos celdas ITEM muestran "one#7" y "one#8"
    And el detalle y --print siguen mostrando la ruta completa de cada uno

  # ────────────────────────── Recalculo del ancho ──────────────────────────

  @inbox @prefijo @layout
  Scenario: El ancho de ITEM se recalcula en cada modo
    Given el modo de prefijo es "common"
    When se pinta la lista
    Then la columna ITEM mide lo que el sufijo más largo + hueco de separación (24 para el fixture de Background)
    When cambio el modo a "full"
    Then la columna ITEM mide lo que la referencia completa + hueco, acotado al tope (34)
    When cambio el modo a "leaf"
    Then la columna ITEM mide lo que la hoja más larga + hueco (16)
    And en los tres modos el ancho se mantiene dentro de [6, 34]

  @inbox @prefijo @layout
  Scenario: El ancho se calcula una vez por render y todas las filas coinciden
    Given el modo de prefijo es "leaf"
    When se pinta la lista
    Then todas las filas.Items empiezan en la misma columna
    And el header de columnas y las filas comparten la misma rejilla
    And la tabla no baila al escribir encima

  @inbox @prefijo @layout
  Scenario: Cambiar de modo no deja el cursor fuera de la ventana
    Given el modo de prefijo es "common"
    And la lista está desplazada y el cursor está en una fila visible
    When pulso "p" para pasar a "full"
    Then el cursor sigue en el mismo ítem y sigue visible en la ventana

  @inbox @prefijo @layout
  Scenario: Un item vacío o un proyecto vacío no rompen el ancho
    Given el modo de prefijo es "leaf"
    And la sección activa incluye un ítem con proyecto vacío
    When se pinta la lista
    Then la celda de ese ítem es solo "#<número>"
    And el resto de la tabla no se descuadra

  # ───────────────────── Degradación sin prefijo común ─────────────────────

  @inbox @prefijo @degradacion
  Scenario: common degrada a full si la sección no tiene prefijo común
    Given el modo de prefijo es "common"
    And la sección activa tiene un solo ítem, o ítems que no comparten directorio
    When se pinta la lista
    Then no se muestra ninguna línea de prefijo
    And cada celda ITEM lleva la ruta completa, recortada por la cola si no cabe
    And el modo "common" se ve exactamente igual que el modo "full"

  @inbox @prefijo @degradacion
  Scenario: La degradación no inventa un prefijo ni repite la ruta
    Given el modo de prefijo es "common"
    And la sección activa tiene los ítems "acme/one#7" y "other/one#8"
    When se pinta la lista
    Then el prefijo común no se pinta por ninguna parte
    And la ruta completa no aparece ni en la línea de prefijo ni duplicada en la celda

  # ────────────────────────── Hint del modo actual ──────────────────────────

  @inbox @prefijo @hints
  Scenario: El hint de p nombra el modo actual, no solo la tecla
    Given el modo de prefijo es "common"
    When se pinta la barra de atajos
    Then muestra "p prefix: common"
    And no muestra la tecla a secas ("p prefix")
    When pulso "p" para pasar a "full"
    Then la barra muestra "p prefix: full"
    When pulso "p" para pasar a "leaf"
    Then la barra muestra "p prefix: leaf"

  @inbox @prefijo @hints @keybindings
  Scenario: El hint del modo sigue al rebind de la tecla
    Given la config reasigna "prefix-mode" a "P"
    And el modo de prefijo es "leaf"
    When se pinta la barra de atajos
    Then muestra "P prefix: leaf"

  @inbox @prefijo @hints
  Scenario: Todo keybind registrado sale en la barra
    Given la config por defecto
    When se pinta la barra de atajos
    Then sale una entrada por cada acción de [keybindings], incluida "prefix-mode"

  @inbox @prefijo @hints
  Scenario: Con el merge armado la Confirmación sustituye a la barra
    Given el modo de prefijo es "leaf"
    And el merge está armado
    When se pinta la barra de atajos
    Then se ve la Confirmación de merge y no la barra de atajos

  # ─────────────────────── No-regresión de otros modos ───────────────────────

  @inbox @prefijo @print
  Scenario: El modo --print no cambia
    When ejecuto "prdash --print"
    Then imprime la ruta completa "proyecto/subgrupo#número" de cada ítem
    And sus tres secciones con sus nombres largos ("Created by me", "Review / assigned", "Mentions")
    And no aplica ningún modo de prefijo ni ninguna línea de prefijo

  @inbox @prefijo
  Scenario: El detalle y la leyenda siguen mostrando la ruta completa
    Given el modo de prefijo es "leaf"
    When se selecciona un ítem y se mira su ficha
    Then el título de la caja de detalle es la referencia completa
    And la leyenda de conteos del borde no cambia de formato
