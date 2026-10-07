package tui

const (
	detailShare   = 2 // 2/5 = 40% of the height
	minListRows   = 3
	minDetailRows = 6
	// The bar wraps to the inner width but never past this cap: the layout clips hints, not recomputes them.
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
	headerLines    = 3
	listChrome     = 2
	detailChrome   = 2
	keybindsChrome = 2
)

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
