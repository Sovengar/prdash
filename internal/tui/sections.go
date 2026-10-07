package tui

import (
	"fmt"
	"strings"
	"time"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/tui/bordered"
)

type view struct {
	text string
	rows []bool
}

type box struct {
	text      string
	paintable bool
}

func (b box) lines() int { return strings.Count(b.text, "\n") + 1 }

type stack struct {
	parts []string
	rows  []bool
}

func (s *stack) add(b box) {
	n := b.lines()
	for i := range n {
		s.rows = append(s.rows, b.paintable && i > 0 && i < n-1)
	}
	s.parts = append(s.parts, b.text)
}

func (s *stack) view() view {
	return view{text: strings.Join(s.parts, "\n"), rows: s.rows}
}

func (m Model) sectionLines(title string, content []string, paintable bool) box {
	if title != "" {
		title = " " + title + " "
	}
	return box{
		text: bordered.RenderWithTitle(
			bordered.Rounded(), borderColor, title, strings.Join(content, "\n"), m.outerWidth(),
		),
		paintable: paintable,
	}
}

func (m Model) layout() layout {
	return computeLayout(m.height, len(m.hintLines()), true)
}

func (m Model) compose(lay layout, boxes ...box) view {
	var s stack
	if lay.showHeader {
		s.add(m.headerSection())
	}
	for _, b := range boxes {
		s.add(b)
	}
	if lay.showKeybinds {
		s.add(m.keybindsSection(lay.hintLines))
	}
	return s.view()
}

func (m Model) headerSection() box {
	parts := make([]string, 0, 2)
	if m.loading {
		parts = append(parts, m.spinner.View()+styleCount.Render(" refreshing…"))
	}
	parts = append(parts, m.forgesStatusLine(time.Now()))
	return m.sectionLines("PRDash", []string{strings.Join(parts, "  ")}, true)
}

func (m Model) listSection(lay layout) box {
	all := m.listLines(m.contentWidth())

	var body []string
	if lay.bodyLines > 0 {
		for _, l := range visibleList(all, m.scroll, lay.bodyLines) {
			body = append(body, l.text)
		}
		for len(body) < lay.bodyLines {
			body = append(body, "")
		}
	} else {
		body = textOf(all)
	}
	return m.sectionLines(m.legend(), body, true)
}

func (m Model) legend() string {
	parts := make([]string, 0, len(m.inbox.Sections))
	for _, sec := range m.inbox.Sections {
		label := fmt.Sprintf("%s (%d)", sec.Kind.Legend(), len(sec.Items))
		if sec.Kind == m.activeSection {
			parts = append(parts, styleLegendActive.Render(label))
			continue
		}
		parts = append(parts, styleDim.Render(label))
	}
	return strings.Join(parts, styleDim.Render(" · "))
}

func textOf(lines []listLine) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.text)
	}
	return out
}

func (m Model) detailSection(it model.Item, ok bool, rows int) box {
	body := m.detailLines(it, ok, rows)
	for len(body) < rows {
		body = append(body, "")
	}
	title := "Detail"
	if ok {
		title = refLabel(it)
	}
	return m.sectionLines(title, body, true)
}

func (m Model) keybindsSection(hintLines int) box {
	lines := m.hintLines()
	lines = lines[:max(0, min(hintLines, len(lines)))]
	return m.sectionLines("Keybinds", lines, false)
}

const hintSep = " · "

func (m Model) hintLines() []string {
	if m.mergeArmed {
		return wrapHint(m.mergeConfirmText(), m.contentWidth(), func(s string) string { return styleWarn.Render(s) })
	}
	return wrapHint(strings.Join(m.cfg.Hints(m.dynamicHints()), hintSep), m.contentWidth(), func(s string) string { return styleHint.Render(s) })
}

func (m Model) dynamicHints() config.HintState {
	return config.HintState{"prefix-mode": m.prefixMode.String()}
}

func wrapHint(text string, width int, paint func(string) string) []string {
	plain := wrapText(text, width)
	plain = plain[:max(0, min(maxHintLines, len(plain)))]
	lines := make([]string, 0, len(plain))
	for _, l := range plain {
		lines = append(lines, paint(l))
	}
	return lines
}

func (m Model) mergeConfirmText() string {
	it, _ := m.selected()
	parts := make([]string, 0, 4)
	for _, mode := range forge.AllowedModes(it.Merge) {
		parts = append(parts, modeKey(mode)+" "+mode.Label())
	}
	if len(parts) == 0 {
		parts = append(parts, "the repository allows no merge strategy")
	}
	head := "merge " + refLabel(it) + "? " + m.deleteLabel()
	if reason := m.mergeBlockReason; reason != "" {
		return head + " with " + reason +
			" · press the mode anyway: " + strings.Join(parts, " · ") + " · esc cancel"
	}
	return head + " press the mode: " + strings.Join(parts, " · ") + " · esc cancel"
}

func (m Model) deleteLabel() string {
	if m.deleteBranch {
		return "delete branch: yes (tab)"
	}
	return "delete branch: no (tab)"
}

func modeKey(mode forge.MergeMode) string {
	switch mode {
	case forge.MergeCommit:
		return "m"
	case forge.Rebase:
		return "r"
	case forge.Squash:
		return "s"
	default:
		return "?"
	}
}

func (m Model) outerWidth() int {
	if m.width <= 0 {
		return defaultOuterWidth
	}
	return m.width
}
