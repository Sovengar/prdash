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

type MergeRules struct {
	Known       bool
	MergeCommit bool
	Rebase      bool
	Squash      bool
}

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
	Project string // full path: "owner/repo" (GH) or "group/sub/name" (GL)
	Owner   string // immediate owner/group
	Name    string
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
	State          string // raw forge state: open/merged/closed…
	IsDraft        bool
	IsFork         bool
	ReviewDecision string // forge review decision: APPROVED/…
	Checks         Checks
	Diff           DiffStat
	HeadSHA        string
	Merge          MergeRules
	Mergeable      Mergeability
	UpdatedAt      time.Time
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

type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

type AuthState struct {
	Forge  string
	OK     bool
	Reason string
	Login  string
}

type Warning struct {
	Forge   string
	Section Section
	Kind    string // "auth" | "network" | "parse" | "timeout" | "unsupported"…
	Msg     string
}
