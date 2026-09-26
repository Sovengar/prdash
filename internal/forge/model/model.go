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

// Legend devuelve la etiqueta corta de la sección para la leyenda de conteos del
// borde del inbox. Es distinta de String() a propósito: String() es el nombre
// largo del modo de datos (--print), que no debe cambiar, y la leyenda necesita
// una palabra por sección para caber en la línea del borde.
func (s Section) Legend() string {
	switch s {
	case SectionAuthored:
		return "Mine"
	case SectionReview:
		return "Assigned"
	case SectionMentions:
		return "Mentioned"
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

// MergeRules son las estrategias de integración que el repositorio admite.
//
// No todos los forges las publican en la consulta del ítem, así que Known
// separa "el repositorio no permite squash" de "no lo sabemos": sin ese bit, un
// repositorio con las tresstrategias desactivadas se indistinguible de uno que
// las tiene todas y la TUI acabaría ofreciendo un modo que el forge va a
// rechazar. Un MergeRules desconocido no restringe nada, y quien lo consume tiene
// que tratarlo como tal en vez de suponer que todo está permitido.
type MergeRules struct {
	Known       bool
	MergeCommit bool
	Rebase      bool
	Squash      bool
}

// MergeRulesAll es el conjunto de reglas de un repositorio que admite las tres
// estrategias. Es un default permisivo, no un dato del repositorio, y por eso
// Known=false es lo que lo distingue de unas reglas realmente leídas del forge.
func MergeRulesAll() MergeRules {
	return MergeRules{Known: true, MergeCommit: true, Rebase: true, Squash: true}
}

// Mergeability resume si el forge puede integrar el ítem tal y como está, sin
// que haya que resolver nada antes.
//
// Known separa "el forge dice que no" de "el forge todavía no lo sabe": GitHub
// devuelve UNKNOWN y GitLab UNCHECKED mientras calculan la mergeabilidad en
// segundo plano, y hay caminos (el respaldo REST de GitHub, la API de Todos de
// GitLab) donde el dato no viaja. Un unknown no es un no, así que no
// restringe: quien lo consume tiene que tratarlo como tal en vez de suponer que
// todo se puede mergear.
type Mergeability struct {
	Known      bool
	Conflicted bool
}

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
	Section      Section
	Forge        string
	Host         string
	Ref          RepoRef
	Number       int
	Title        string
	Author       string
	ReviewKind   ReviewKind
	SourceBranch string
	TargetBranch string
	URL          string
	State        string // estado crudo del forge: open/merged/closed…
	// IsDraft dice que el forge marco el ítem como borrador. Es un campo aparte
	// y no un valor de State a propósito: State es el enum del forge (OPEN en
	// GitHub, opened en GitLab) y meter ahí un "draft" derivado obligaba a que
	// cada adapter tradujera y a que la comprobación dependiera de que la
	// traducción fuera exacta. Un PR en borrador llega con State="OPEN" y esta
	// bandera a true, y quien decide qué etiquetas mostrar lo lee aquí.
	IsDraft bool
	// IsFork dice que la rama origen del ítem vive en otro repositorio. En
	// GitHub es `isCrossRepository`; los forges donde la rama siempre es del
	// repo destino lo dejan en false. Importa por una cosa concreta: borrar la
	// rama al mergear un PR de fork no borra nada, así que quien affirme lo
	// contrario en un aviso está mintiendo.
	IsFork         bool
	ReviewDecision string // decisión de review del forge: APPROVED/…
	Checks         Checks
	Diff           DiffStat
	// HeadSHA es el commit al que apunta la rama origen del ítem, tal y como lo
	// tenía el forge en la última lectura. Es lo que permite pinear el merge a un
	// commit concreto (`--match-head-commit` / `--sha`): sin él, un merge puede
	// integrar commits que nadie miró, porque la rama se movió entre el refresco
	// del inbox y la pulsación. Vacío significa "el forge no lo reportó", que no
	// es lo mismo que "no hay": por eso un merge que lo necesita se niega en vez
	// de integrar a ciegas.
	HeadSHA string
	// Merge son las estrategias que el repositorio admite. Un repositorio que no
	// publica el dato llega con Known=false, y eso no restringe nada.
	Merge MergeRules
	// Mergeable es si el forge puede integrar esto ahora mismo. Vive separado de
	// Merge porque son preguntas distintas: MergeRules es lo que el repositorio
	// PERMITE (y no lo publica GitLab), y esto es lo que el forge puede HACER con
	// el ítem tal y como está. Un repositorio puede permitirte las tres
	// estrategias y seguir sin poder integrarte el PR porque las ramas se pisan.
	Mergeable Mergeability
	UpdatedAt time.Time
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

// Comment es una intervención escrita por una persona en la conversación de un
// ítem. No lleva estado de forge ni reactions: la ficha solo necesita saber
// quién escribió qué y cuándo, que es lo que decide si hay que abrir el PR para
// entenderlo.
//
// Body va en crudo, con su markdown. Quitarlo es trabajo de la vista, no del
// parseo: el mismo cuerpo se lee de otra forma según las filas que quedaron.
type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
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
