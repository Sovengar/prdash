// The ITEM column: which part of the project path is visible, and at what width. Long paths do
// not fit a fixed column, and clipping from the head ate the repo name and the "#number".
package tui

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

type prefixMode int

const (
	prefixCommon prefixMode = iota
	prefixFull
	prefixLeaf
)

// next() and String() come out of the same type on purpose: the hint cannot describe a mode the cycle
// does not reach.
func (p prefixMode) String() string {
	switch p {
	case prefixFull:
		return "full"
	case prefixLeaf:
		return "leaf"
	default:
		return "common"
	}
}

func (p prefixMode) next() prefixMode {
	return (p + 1) % 3
}

// Computed once per render and handed to both the header and the rows: if each measured on its
// own, the table would dance as text is written.
type refLayout struct {
	cols   []tableColumn
	prefix map[model.Section]string
	mode   prefixMode
}

// Only the active section is passed in, so no width is spent on suffixes that are off screen. The
// mode is asked for explicitly: every caller has to say what it paints, which changes the width.
func newRefLayout(sections []inbox.Section, mode prefixMode) refLayout {
	l := refLayout{mode: mode, prefix: make(map[model.Section]string, len(sections))}
	longest := 0
	for _, sec := range sections {
		if len(sec.Items) == 0 {
			continue
		}
		prefix := ""
		if mode == prefixCommon {
			prefix = sectionPrefix(sec.Items)
		}
		l.prefix[sec.Kind] = prefix
		for _, it := range sec.Items {
			longest = max(longest, utf8.RuneCountInString(refCellText(it, mode, prefix)))
		}
	}
	l.cols = slices.Clone(tableColumns)
	l.cols[colRefIdx].width = min(max(longest+1, itemWidthMin), itemWidthCap)
	return l
}

func (l refLayout) prefixOf(sec model.Section) string {
	return l.prefix[sec]
}

// Aligned on "/" boundaries and never eating the last segment: the cell always keeps the project name
// and its number.
func sectionPrefix(items []model.Item) string {
	if len(items) < 2 {
		return ""
	}
	segs := make([][]string, 0, len(items))
	for _, it := range items {
		segs = append(segs, strings.Split(it.Ref.Project, "/"))
	}
	minSegs := len(segs[0])
	for _, s := range segs[1:] {
		minSegs = min(minSegs, len(s))
	}
	common := 0
	for i := range minSegs - 1 {
		seg := segs[0][i]
		for _, other := range segs[1:] {
			if other[i] != seg {
				return strings.Join(segs[0][:common], "/")
			}
		}
		common++
	}
	return strings.Join(segs[0][:common], "/")
}

func refCellText(it model.Item, mode prefixMode, prefix string) string {
	if mode == prefixLeaf {
		return refLeaf(it)
	}
	return refSuffix(it, prefix)
}

// Cut at the LAST separator, not the first, because the identifier is at the end. An empty project
// gives "#n": a forge can return an unparsed reference, and the number still identifies the item.
func refLeaf(it model.Item) string {
	project := it.Ref.Project
	if i := strings.LastIndex(project, "/"); i >= 0 {
		project = project[i+1:]
	}
	return project + "#" + strconv.Itoa(it.Number)
}

func refSuffix(it model.Item, prefix string) string {
	label := refLabel(it)
	if prefix == "" {
		return label
	}
	rest, ok := strings.CutPrefix(label, prefix+"/")
	if !ok {
		return label
	}
	return rest
}

func truncateTail(s string, w int) string {
	if w <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return "…" + string(runes[len(runes)-(w-1):])
}
