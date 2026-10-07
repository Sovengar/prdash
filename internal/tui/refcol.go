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

type refLayout struct {
	cols   []tableColumn
	prefix map[model.Section]string
	mode   prefixMode
}

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
