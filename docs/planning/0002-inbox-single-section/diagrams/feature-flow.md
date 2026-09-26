# Feature flow — 0002 Inbox de una sola sección

Flujo de comportamiento del Inbox, derivado de `behavior.feature`. Feature-aware:
una sola sección activa, leyenda de conteos y prefijo de ruta común de la activa.

```mermaid
flowchart TD
  Start([Abrir prdash]) --> Def["Sección activa = Assigned (review)<br/>al abrir"]

  subgraph Ciclo["Ciclo de tab (acción section-next)"]
    A["Assigned"] -->|tab| M["Mentioned"]
    M -->|tab| Mi["Mine"]
    Mi -->|tab| A
  end
  Def --> A

  Def --> Legend["Leyenda en el borde SUP. IZQ<br/>Mine (n) · Assigned (n) · Mentioned (n)<br/>activa resaltada · resto atenuado"]
  Legend -. sustituye .-> T["título 'Inbox'"]

  Def --> Body{"Componer el cuerpo<br/>de la sección activa"}
  Body --> Pref{"¿La activa comparte<br/>prefijo de ruta?"}
  Pref -->|sí| PL["Línea fija atenuada:<br/>· APPCITTI/vsocial/backend/"]
  Pref -->|no| FC["Celdas ITEM con ruta completa<br/>(recorte por la cola)"]
  PL --> Content
  FC --> Content

  Content{"Estado de la activa"}
  Content -->|con ítems| Rows["header de columnas + filas<br/>celdas ITEM solo con el sufijo"]
  Content -->|sin ítems y sin avisos| Empty["(empty)"]
  Content -->|con avisos| Warn["⚠ forge: could not be queried (…)"]
  Content -->|paginando| More["loading more…<br/>(solo si pagina la activa)"]

  Cycle["Cambiar de sección"]
  A -. guarda cursor/scroll .-> Mem[("Posición por sección")]
  Mem -. restaura al volver .-> A

  classDef active fill:#1f6feb,stroke:#1f6feb,color:#fff;
  class A active;
```

Notas:
- Solo se pinta la sección activa; los avisos y el `loading more…` son de la
  activa (los de otras secciones no se pintan).
- El prefijo de ruta común se conserva (ADR 0002) y es la costura para el futuro
  prefijo seleccionable/toggleable (feature posterior).
