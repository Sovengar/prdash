// Package model define los tipos normalizados e inmutables que comparten
// todos los forges: referencias de repositorio, ítems del inbox y avisos.
//
// No toca red, subproceso, TOML ni disco: es la capa de datos pura sobre la
// que se construyen el parseo, la consolidación del inbox y el render.
package model

import "time"

// Section identifica la sección del inbox a la que pertenece un ítem.
type Section string

const (
	// SectionAuthored son los PR/MR creados por el usuario.
	SectionAuthored Section = "authored"
	// SectionReview son los review pedidos o asignados al usuario.
	SectionReview Section = "review"
	// SectionMentions son los ítems donde el usuario aparece mencionado.
	SectionMentions Section = "mentions"
)

// String devuelve el título visible de la sección, compartido por la TUI y el
// modo de impresión para que no diverjan.
func (s Section) String() string {
	switch s {
	case SectionAuthored:
		return "Created by me"
	case SectionReview:
		return "Review / assigned"
	case SectionMentions:
		return "Mentions"
	default:
		return string(s)
	}
}

// ReviewKind distingue, dentro de la sección de review, si el ítem llegó por
// una petición de review o por una asignación.
type ReviewKind string

const (
	// ReviewRequested indica que el usuario tiene un review pedido.
	ReviewRequested ReviewKind = "requested"
	// ReviewAssigned indica que el usuario está asignado al ítem.
	ReviewAssigned ReviewKind = "assigned"
)

// CheckState resume el estado agregado de los checks de CI de un ítem.
type CheckState string

const (
	// ChecksUnknown indica que el forge no reportó checks.
	ChecksUnknown CheckState = ""
	// ChecksPassing indica que todos los checks pasan.
	ChecksPassing CheckState = "passing"
	// ChecksFailing indica que al menos un check falló.
	ChecksFailing CheckState = "failing"
	// ChecksPending indica que hay checks aún en ejecución.
	ChecksPending CheckState = "pending"
)

// Checks resume los checks/CI de un ítem sin depender del forge.
type Checks struct {
	State   CheckState
	Total   int
	Failing int
	Pending int
}

// DiffStat resume el tamaño del cambio de un ítem: cuántas líneas añade y
// borra, y sobre cuántos ficheros, sin depender del forge.
//
// Known separa "el cambio es de 0 líneas" de "no se pudo saber": ambos casos
// traen ceros, pero el segundo significa que la fuente no traía el dato (el
// respaldo REST de GitHub, la API de Todos de GitLab, un forge sin soporte) y
// no que el PR esté vacío. Sin ese bit, un ítem sin datos se pintaría como un
// cambio de tamaño cero, que es una mentira.
type DiffStat struct {
	Additions int
	Deletions int
	Files     int
	Known     bool
}

// Total es el número de líneas tocadas: la magnitud que ordena el trabajo.
func (d DiffStat) Total() int { return d.Additions + d.Deletions }

// RepoRef identifica un repositorio dentro de un forge y host concretos.
type RepoRef struct {
	Forge   string // "github" | "gitlab" | ...
	Host    string // "github.com" | "gitlab.example.com" | ...
	Project string // ruta completa: "owner/repo" (GH) o "grupo/sub/proy" (GL)
	Owner   string // propietario/grupo inmediato
	Name    string // nombre del repositorio
}

// ID es la identidad canónica de un ítem: forge, host, proyecto y número.
// Es la clave con la que el inbox deduplica entre secciones y queries.
type ID struct {
	Forge   string
	Host    string
	Project string
	Number  int
}

// With construye la identidad canónica de un ítem.
func With(forge, host, project string, number int) ID {
	return ID{Forge: forge, Host: host, Project: project, Number: number}
}

// Item es un PR/MR normalizado, listo para pintar, deduplicar y ordenar.
type Item struct {
	Section        Section
	Forge          string
	Host           string
	Ref            RepoRef
	Number         int
	Title          string
	Author         string
	ReviewKind     ReviewKind
	SourceBranch   string
	TargetBranch   string
	URL            string
	State          string // estado crudo del forge: open/merged/closed…
	ReviewDecision string // decisión de review del forge: APPROVED/…
	Checks         Checks
	Diff           DiffStat
	UpdatedAt      time.Time
}

// NewItem construye un ítem normalizando los campos que derivan de la
// referencia (forge y host) para que no puedan divergir.
func NewItem(ref RepoRef, number int) Item {
	return Item{
		Forge:  ref.Forge,
		Host:   ref.Host,
		Ref:    ref,
		Number: number,
	}
}

// ID devuelve la identidad canónica del ítem.
func (it Item) ID() ID {
	return With(it.Forge, it.Host, it.Ref.Project, it.Number)
}

// AuthState describe si un forge está autenticado y operativo.
type AuthState struct {
	Forge  string
	OK     bool
	Reason string
	// Login es el usuario con el que está autenticado el forge. Va vacío si el
	// adapter no sabe deducirlo. Permite reconocer los ítems propios sin
	// depender de en qué sección aparecieron.
	Login string
}

// Warning describe un fallo parcial de un forge. El inbox nunca falla duro:
// acumula warnings y conserva el resto de los ítems.
type Warning struct {
	Forge   string
	Section Section
	Kind    string // "auth" | "network" | "parse" | "timeout" | "unsupported"…
	Msg     string
}
