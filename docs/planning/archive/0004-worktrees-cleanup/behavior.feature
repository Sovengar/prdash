# prdash — comportamiento esperado: limpieza de worktrees de review.
#
# Fuente ÚNICA del comportamiento esperado de esta feature. No es Cucumber (sin
# step definitions ni runner). El executor derivará de aquí tests reales
# (unit/integration) estilo BDD/ATDD.
#
# Gherkin en inglés; las descripciones van en español.
#
# Contexto de diseño: los worktrees de review son del usuario y prdash los
# conserva a propósito — NO hay borrado al cerrar la app (restricción dura en
# cmd/prdash/worktrees.go). Esta feature abre dos caminos de limpieza, y solo
# dos, ambos justificados por un hecho observable:
#
#   A) `prdash worktrees remove --orphans`: borra en lote SOLO los worktrees que
#      el propio `Audit` marca como huérfanos (repo de origen inalcanzable). Es
#      excluyente con las rutas explícitas. La fuente de verdad de "huérfano" es
#      la misma que ya usa `worktrees list`: aquí no se reimplementa.
#
#   B) Auto-borrado al mergear DESDE prdash: cuando una acción de merge lanzada
#      por prdash termina bien, el worktree de ese ítem ya es basura. Pero el
#      checkout puede tener trabajo sin commitear, así que se borra SOLO si está
#      limpio; si está sucio, se CONSERVA y se avisa con
#      "merged, but the worktree has uncommitted changes — kept". Un merge que no
#      salió bien (fallo, conflicto, permiso, no mergeable), o cualquier otra
#      acción (approve, retarget), NO dispara nada.
#
# Invariante que cruza A y B: toda limpieza pasa por los guardas de ownership
# (`worktree.Owned`: label o nombre de ruta con prefijo `prdash-`) y por la
# exigencia de que la ruta viva bajo la raíz gestionada. Un worktree ajeno es
# intocable en todos los caminos.

Feature: Limpieza de worktrees de review en prdash

  Background:
    Given una raíz gestionada de worktrees donde prdash provisiona los suyos
    And hay worktrees propios de prdash (nombre "prdash-…") y worktrees ajenos conviviendo bajo esa raíz
    And un worktree es "huérfano" cuando el repo de origen al que apunta su enlace ya no es accesible

  # ═══════════════════ A — limpieza en lote de huérfanos ═══════════════════

  @cli @worktrees @orphans
  Scenario: --orphans borra todos los huérfanos y solo los huérfanos
    Given la raíz tiene dos worktrees propios huérfanos y un worktree propio sano
    When ejecuto "prdash worktrees remove --orphans"
    Then los dos huérfanos desaparecen del disco
    And el worktree propio sano sigue en disco
    And la salida nombra cada worktree que se borró
    And el comando termina con código de salida 0

  @cli @worktrees @orphans
  Scenario: --orphans borra exactamente lo que "worktrees list" marca como huérfano
    Given "prdash worktrees list" reporta N worktrees con estado "orphaned"
    When ejecuto "prdash worktrees remove --orphans"
    Then borra exactamente esos N
    And no borra ninguno de los reportados con estado "ok"

  @cli @worktrees @orphans @seguridad
  Scenario: --orphans nunca toca un worktree ajeno
    Given la raíz tiene un worktree ajeno junto a un huérfano propio
    When ejecuto "prdash worktrees remove --orphans"
    Then el worktree ajeno sigue intacto en disco
    And el huérfano propio desaparece

  @cli @worktrees @orphans @seguridad
  Scenario: un worktree propio sano nunca se borra con --orphans
    Given un worktree propio cuyo repo de origen sigue siendo accesible
    When ejecuto "prdash worktrees remove --orphans"
    Then ese worktree sigue en disco
    And sigue listándose con estado "ok"

  @cli @worktrees @orphans
  Scenario: --orphans con cero huérfanos es el caso feliz, no un error
    Given la raíz no tiene ningún worktree huérfano
    When ejecuto "prdash worktrees remove --orphans"
    Then no borra nada
    And informa de que no hay huérfanos
    And el comando termina con código de salida 0
    And no escribe nada en stderr

  @cli @worktrees @orphans @dry-run
  Scenario: --dry-run muestra el lote exacto que borraría y no borra nada
    Given la raíz tiene dos worktrees propios huérfanos, un worktree propio sano y un worktree ajeno
    When ejecuto "prdash worktrees remove --orphans --dry-run"
    Then imprime las rutas de exactamente los dos huérfanos (el mismo lote que borraría sin --dry-run)
    And no borra ningún worktree
    And el worktree propio sano y el ajeno siguen intactos
    And el comando termina con código de salida 0

  @cli @worktrees @orphans @dry-run
  Scenario: --dry-run con cero huérfanos sigue siendo el caso feliz
    Given la raíz no tiene ningún worktree huérfano
    When ejecuto "prdash worktrees remove --orphans --dry-run"
    Then no imprime ningún huérfano
    And no borra nada
    And el comando termina con código de salida 0
    And no escribe nada en stderr

  @cli @worktrees @orphans @dry-run @uso
  Scenario: --dry-run sin --orphans es un error de uso
    Given un huérfano propio y un worktree propio sano
    When ejecuto "prdash worktrees remove --dry-run"
    Then explica por stderr que --dry-run requiere --orphans
    And el comando termina con código de salida 2
    And no borra nada, ni el huérfano ni ningún worktree

  @cli @worktrees @orphans @uso
  Scenario: --orphans es excluyente con las rutas explícitas
    Given un huérfano propio y un worktree propio sano
    When ejecuto "prdash worktrees remove --orphans <ruta>"
    Then no borra nada, ni el huérfano ni la ruta nombrada
    And explica por stderr que los dos modos no se mezclan
    And el comando termina con código de salida 2

  @cli @worktrees @orphans @uso
  Scenario: un flag desconocido en remove es un error de uso
    When ejecuto "prdash worktrees remove --bogus"
    Then no borra nada
    And explica por stderr que el flag no se reconoce
    And el comando termina con código de salida 2

  @cli @worktrees @uso
  Scenario: un token con guion inicial es un flag, nunca una ruta
    Given un worktree propio sano
    When ejecuto "prdash worktrees remove --orphan"
    Then no borra nada
    And explica por stderr que no reconoce el flag
    And el comando termina con código de salida 2

  @cli @worktrees
  Scenario: remove con rutas explícitas sigue comportándose como antes
    Given un worktree propio sano y un worktree ajeno
    When ejecuto "prdash worktrees remove <ruta-del-propio> <ruta-del-propio-2>"
    Then borra las rutas propias nombradas
    And el worktree ajeno sigue intacto
    And un rechazo (ruta ajena o inexistente) no toca nada y sale con 1

  @cli @worktrees @limpieza
  Scenario: un huérfano con un enlace .git irresoluble se borra por ruta explícita
    Given un worktree propio cuyo fichero .git no declara un gitdir y cuyo repo de origen ya no existe
    When ejecuto "prdash worktrees remove <su-ruta>"
    Then lo borra igualmente: no hay repo que resolver y solo queda borrar su checkout
    And el comando termina con código de salida 0

  @cli @worktrees @orphans @limpieza
  Scenario: un huérfano con .git irresoluble no tumba el lote de --orphans
    Given la raíz tiene un huérfano con el .git irresoluble y otro huérfano con el .git bien formado
    When ejecuto "prdash worktrees remove --orphans"
    Then borra los dos huérfanos
    And el comando termina con código de salida 0

  @cli @worktrees @uso
  Scenario: remove sin rutas ni --orphans sigue siendo error de uso
    When ejecuto "prdash worktrees remove"
    Then explica por stderr que falta al menos una ruta
    And el comando termina con código de salida 2
    And no borra nada

  # ═══════════════ B — auto-borrado del worktree al mergear desde prdash ═══════════════

  @tui @merge @limpieza
  Scenario: un merge OK desde prdash borra el worktree limpio del ítem
    Given un ítem con su worktree de review montado y sin cambios sin commitear
    When mergeo ese ítem desde prdash y el merge termina bien
    Then el worktree de ese ítem desaparece del disco
    And el aviso dice a la vez que el merge salió bien y que se borró el worktree
    And ese ítem deja de tener review montado

  @tui @merge @limpieza
  Scenario: un merge OK con el worktree sucio lo conserva y lo dice
    Given un ítem con su worktree de review montado y con cambios sin commitear
    When mergeo ese ítem desde prdash y el merge termina bien
    Then el worktree sigue en disco
    And el aviso dice "merged, but the worktree has uncommitted changes — kept"
    And ese ítem sigue con su review montado

  @tui @merge @limpieza
  Scenario: un archivo nuevo sin trackear también cuenta como sucio
    Given un ítem con su worktree montado cuyo único cambio es un archivo no trackeado
    When mergeo ese ítem desde prdash y el merge termina bien
    Then el worktree se conserva (un archivo nuevo es trabajo sin commitear)
    And el aviso lo dice

  @tui @merge @limpieza
  Scenario: ante un estado de git ilegible se conserva el worktree
    Given un ítem con su worktree montado cuyo estado de git no se puede leer
    When mergeo ese ítem desde prdash y el merge termina bien
    Then el worktree se conserva (ante la duda, no se borra)
    And el aviso dice que no se pudo comprobar su estado

  @tui @merge @limpieza
  Scenario: un merge OK sin worktree montado no reporta error
    Given un ítem sin ningún worktree de review montado
    When mergeo ese ítem desde prdash y el merge termina bien
    Then el merge termina igual
    And no se reporta ningún error de limpieza

  @tui @merge @limpieza
  Scenario: si el worktree ya no está en disco, es un no-op sin error
    Given un ítem cuyo review montado apunta a una ruta que ya no existe
    When mergeo ese ítem desde prdash y el merge termina bien
    Then el merge termina igual
    And no se reporta ningún error

  @tui @merge @limpieza
  Scenario: el aviso del merge conserva todas las verdades a la vez
    Given un ítem con un worktree sucio cuyo merge sale bien pero la rama no se borró
    When mergeo ese ítem desde prdash
    Then el aviso dice a la vez: merge ok, rama no borrada, y worktree conservado
    And no se pierde ningún hecho por mostrar solo el último

  @tui @merge @limpieza @negativo
  Scenario: approve no borra ningún worktree
    Given un ítem con su worktree de review montado
    When apruebo ese ítem desde prdash
    Then el worktree sigue en disco

  @tui @retarget @limpieza @negativo
  Scenario: retarget no borra ningún worktree
    Given un ítem con su worktree de review montado
    When cambio su rama destino desde prdash
    Then el worktree sigue en disco

  @tui @merge @limpieza @negativo
  Scenario: un merge que no sale bien no borra ningún worktree
    Given un ítem con su worktree de review montado
    When intento mergearlo y el merge falla, se bloquea, da conflicto o no es mergeable
    Then el worktree sigue en disco
    And no aparece ningún aviso de borrado

  @tui @quit @limpieza @negativo
  Scenario: cerrar la app no borra ningún worktree
    Given hay worktrees de review montados
    When cierro la TUI con "q" o "ctrl+c"
    Then todos los worktrees siguen en disco
    And no se ejecuta ningún borrado al salir

  @tui @merge @limpieza @negativo
  Scenario: un PR mergeado directamente en el forge no dispara la limpieza
    Given un ítem cuyo PR fue mergeado fuera de prdash
    When prdash refresca el inbox y lo ve mergeado
    Then su worktree NO se borra de forma implícita
    And sigue disponible para limpiarlo a mano o con --orphans si quedara huérfano

  # ═══════════════════ Guardas comunes a las dos vías ═══════════════════

  @cli @tui @seguridad
  Scenario: ninguna vía de limpieza toca un worktree ajeno o fuera de la raíz
    Given un worktree ajeno y un worktree propio situado fuera de la raíz gestionada
    When se intenta borrar cualquiera de los dos por ruta explícita, por --orphans o por B
    Then ninguno de los dos se borra
    And solo puede borrarse lo propio que además vive dentro de la raíz gestionada

  @cli @worktrees @seguridad
  Scenario: la guarda de raíz gestionada aplica en las dos provisiones
    Given un worktree propio situado fuera de la raíz gestionada
    When intento borrarlo por ruta explícita, con git directo o con la provisión nativa de Herdr
    Then no se borra
    And el rechazo es un error y no toca nada, ni con el borrado nativo

  @cli @tui @seguridad
  Scenario: una ruta inexistente se rechaza sin tocar nada
    When pido borrar una ruta que no existe, por cualquier vía
    Then no se borra nada
    And no se crea ni se modifica ningún otro worktree
