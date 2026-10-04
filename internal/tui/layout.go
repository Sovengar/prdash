// The view's height budget: which boxes are visible and how much height is left for each body. Single
// source shared by the render, the scroll and the tests, because the parts must add up to the terminal
// height: the view fills it exactly and any mismatch pushes the hints or the header off screen.
package tui

const (
	detailShare   = 2 // 2/5 = 40% de la altura
	minListRows   = 3
	minDetailRows = 6
	// The bar wraps to the inner width, so it is several lines in a narrow terminal, but never more than
	// this cap: the layout clips hints instead of recomputing them.
	maxHintLines = 3
)

type layout struct {
	showHeader   bool
	showKeybinds bool
	hintLines    int
	bodyLines    int
	detailLines  int
}

const (
	headerContentLines = 1 // spinner y estado de los forges
	headerLines        = headerContentLines + 2
	listChrome         = 2
	detailChrome       = 2
	keybindsChrome     = 2
)

// The detail keeps its 40% while the central body can stay at minListRows, and degrades in order if
// not: the detail panel drops to its minimum, then the header box hides, then the hints drop to one
// line, then the hint box hides. That walk leaves the essentials and is what makes the view usable in a
// 20-line terminal.
// show is false before the first WindowSizeMsg, when nothing is clipped and the whole view paints.
func computeLayout(height, hintAvailable int, show bool) layout {
	if !show || height <= 0 {
		return layout{}
	}

	lay := layout{
		showHeader:   true,
		showKeybinds: true,
		hintLines:    min(maxHintLines, max(1, hintAvailable)),
	}
	lay.bodyLines = height
	lay.detailLines = max(minDetailRows, height*detailShare/5)

	reserved := func() int {
		n := listChrome + detailChrome + lay.detailLines
		if lay.showHeader {
			n += headerLines
		}
		if lay.showKeybinds {
			n += keybindsChrome + lay.hintLines
		}
		return n
	}

	for reserved()+minListRows > height && lay.detailLines > minDetailRows {
		lay.detailLines--
	}
	for reserved()+minListRows > height && lay.showHeader {
		lay.showHeader = false
	}
	for reserved()+minListRows > height && lay.hintLines > 1 {
		lay.hintLines--
	}
	for reserved()+minListRows > height && lay.showKeybinds {
		lay.showKeybinds = false
		lay.hintLines = 0
	}
	for reserved()+1 > height && lay.detailLines > 0 {
		lay.detailLines--
	}

	lay.bodyLines = max(1, height-reserved())
	return lay
}
