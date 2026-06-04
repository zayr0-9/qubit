package tui

import "github.com/qubit/graviton-cli/internal/tui/components/listwindow"

type listWindow = listwindow.Window

func visibleListWindow(total int, cursor int, maxRows int) listWindow {
	return listwindow.Visible(total, cursor, maxRows)
}
