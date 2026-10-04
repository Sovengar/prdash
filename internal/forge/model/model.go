package model

import "time"

type Section string

const (
	SectionAuthored Section = "authored"
	SectionReview   Section = "review"
	SectionMentions Section = "mentions"
)

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

// Not String(): that is the long data-mode name (--print) and must not change, while the border
// legend needs one word per section to fit.
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

type ReviewKind string

const (
	ReviewRequested ReviewKind = "requested"
	ReviewAssigned  ReviewKind = "assigned"
)

type CheckState string

const (
	ChecksUnknown CheckState = ""
	ChecksPassing CheckState = "passing"
	ChecksFailing CheckState = "failing"
	ChecksPending CheckState = "pending"
)

type Checks struct {
	State   CheckState
	Total   int
	Failing int
	Pending int
}

type DiffStat struct {
	Additions int
	Deletions int
	Files     int
	Known     bool
}

func (d DiffStat) Total() int { return d.Additions + d.Deletions }

// Known separates "the repo forbids squash" from "we do not know": without it a repo with all
// three disabled is indistinguishable from one that allows them. Unknown restricts nothing.
type MergeRules struct {
	Known       bool
	MergeCommit bool
	Rebase      bool
	Squash      bool
}

// A permissive default, not data from a repo, which is exactly why Known=false is what
// distinguishes it from rules actually read from the forge.
func MergeRulesAll() MergeRules {
	return MergeRules{Known: true, MergeCommit: true, Rebase: true, Squash: true}
}

type Mergeability struct {
	Known      bool
	Conflicted bool
}

type RepoRef struct {
	Forge   string // "github" | "gitlab" | ...
	Host    string // "github.com" | "gitlab.example.com" | ...
	Project string // ruta completa: "owner/repo" (GH) o "grupo/sub/proy" (GL)
	Owner   string // propietario/grupo inmediato
	Name    string // nombre del repositorio
}

type ID struct {
	Forge   string
	Host    string
	Project string
	Number  int
}

func With(forge, host, project string, number int) ID {
	return ID{Forge: forge, Host: host, Project: project, Number: number}
}

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
	// A field of its own rather than a State value: State is the forge's enum (OPEN in GitHub, opened
	// in GitLab) and a derived "draft" there forced every adapter to translate it exactly.
	IsDraft bool
	// Matters for one concrete thing: deleting the branch when merging a fork PR deletes nothing, so a
	// warning saying otherwise is lying on the items most closely watched.
	IsFork         bool
	ReviewDecision string // decisión de review del forge: APPROVED/…
	Checks         Checks
	Diff           DiffStat
	// Empty means "the forge did not report it", not "there is none": a merge that needs it refuses
	// instead of integrating blind.
	HeadSHA string
	Merge   MergeRules
	// Separate from Merge because they are different questions: what the repo ALLOWS versus what
	// the forge can DO with the item as it stands.
	Mergeable Mergeability
	UpdatedAt time.Time
}

func NewItem(ref RepoRef, number int) Item {
	return Item{
		Forge:  ref.Forge,
		Host:   ref.Host,
		Ref:    ref,
		Number: number,
	}
}

func (it Item) ID() ID {
	return With(it.Forge, it.Host, it.Ref.Project, it.Number)
}

// Body stays raw with its markdown: the same body reads differently depending on the rows that fit.
type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

type AuthState struct {
	Forge  string
	OK     bool
	Reason string
	// Empty when the adapter cannot infer it. Lets our own items be recognised without depending on
	// which section they arrived in.
	Login string
}

type Warning struct {
	Forge   string
	Section Section
	Kind    string // "auth" | "network" | "parse" | "timeout" | "unsupported"…
	Msg     string
}
