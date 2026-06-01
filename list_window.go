package main

type listWindow struct {
	Start    int
	End      int
	HasAbove bool
	HasBelow bool
}

func visibleListWindow(total int, cursor int, maxRows int) listWindow {
	if total <= 0 || maxRows <= 0 {
		return listWindow{}
	}
	cursor = clampInt(cursor, 0, total-1)
	if total <= maxRows {
		return listWindow{Start: 0, End: total}
	}
	if maxRows < 3 {
		return listWindow{Start: cursor, End: cursor + 1, HasAbove: cursor > 0, HasBelow: cursor < total-1}
	}

	edgeRows := maxRows - 1
	if cursor < edgeRows {
		return listWindow{Start: 0, End: edgeRows, HasBelow: true}
	}
	if cursor >= total-edgeRows {
		return listWindow{Start: total - edgeRows, End: total, HasAbove: true}
	}

	visibleRows := maxRows - 2
	start := cursor - visibleRows/2
	start = clampInt(start, 1, total-visibleRows-1)
	return listWindow{Start: start, End: start + visibleRows, HasAbove: true, HasBelow: true}
}
