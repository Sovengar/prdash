# Flujo de comportamiento — prdash MVP

Derivado de `behavior.feature`. Recorrido del usuario: inbox (F1), orquestador de review (F2),
degradación y gate de F3.

```mermaid
flowchart TD
    A["Arranca prdash"] --> B{"¿HERDR_ENV=1?"}
    B -->|No| C["F1 inbox operativo<br/>F2 muestra aviso: requiere Herdr"]
    B -->|Sí| D["F1 inbox + F2 disponible"]

    C --> E["Inbox: Creados por mí · Review/asignados · Menciones"]
    D --> E
    E --> E1["Refresco manual r / automático 60s<br/>indicador última actualización por forge"]
    E1 --> E2{"¿Alguna forge falla?"}
    E2 -->|Sí| E3["Estado de error explícito por forge<br/>el resto sigue visible"]
    E2 -->|No| E4["Secciones vacías vs con datos"]

    E3 --> G
    E4 --> G
    G{"Selección de ítem"} -->|detalle| H["Detalle: título, autor, ramas, nº, URL, estado"]
    G -->|approve / merge| I["Acción vía gh/glab o delegada en tuicr"]
    G -->|montar review| J{"¿Repo local resuelto?"}

    I --> I1{"¿Ítem cambió en el forge?"}
    I1 -->|Sí| I2["Error claro + refresco del ítem"]
    I1 -->|No| I3["Estado actualizado"]

    J -->|No| K["Clon bare en XDG data<br/>repos/forge/host/owner/repo"]
    J -->|Sí| L["Fetch del ref de review<br/>refs/pull/N/head · refs/merge-requests/N/head"]
    K --> L
    L --> M["Crear rama local de trabajo + provisionar worktree"]
    M --> N["Layout Herdr: TUICR + Hunk + agente opencode"]
    N --> O["Loop: comento en TUICR/Hunk → el agente los lee y aplica"]
    O --> P["Al cerrar: worktree se conserva<br/>limpieza por comando explícito"]

    P --> Q{"F3 activado y repo en allowlist?"}
    Q -->|No| R["No auto-aprueba · deja constancia del motivo"]
    Q -->|Sí| S{"¿0 findings críticos y análisis concluyente?"}
    S -->|No| R
    S -->|Sí| T["Aprueba vía forge + notifica por Herdr"]

    M -.->|sin permisos de clon/fetch| X["Error claro, sin dejar clon ni worktree a medias"]
    N -.->|herramienta ausente| Y["Pane omitido con aviso, el layout sigue vivo"]
```

**Notas de comportamiento**
- "Vacío" y "error" nunca se confunden (falla explícita por forge/sección).
- Sin tope por sección: el inbox pagina hasta agotar, con carga progresiva.
- Fuera de Herdr, F1 no se degrada; solo F2 se informa.
