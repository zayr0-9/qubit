package listwindow

type Window struct {
	Start    int
	End      int
	HasAbove bool
	HasBelow bool
}

func Visible(total int, cursor int, maxRows int) Window {
	if total <= 0 || maxRows <= 0 {
		return Window{}
	}
	cursor = clamp(cursor, 0, total-1)
	if total <= maxRows {
		return Window{Start: 0, End: total}
	}
	if maxRows < 3 {
		return Window{Start: cursor, End: cursor + 1, HasAbove: cursor > 0, HasBelow: cursor < total-1}
	}

	edgeRows := maxRows - 1
	if cursor < edgeRows {
		return Window{Start: 0, End: edgeRows, HasBelow: true}
	}
	if cursor >= total-edgeRows {
		return Window{Start: total - edgeRows, End: total, HasAbove: true}
	}

	visibleRows := maxRows - 2
	start := cursor - visibleRows/2
	start = clamp(start, 1, total-visibleRows-1)
	return Window{Start: start, End: start + visibleRows, HasAbove: true, HasBelow: true}
}

func clamp(value int, low int, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
