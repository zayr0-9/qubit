package main

import "testing"

func TestVisibleListWindowFitsAllRows(t *testing.T) {
	got := visibleListWindow(3, 2, 5)
	want := listWindow{Start: 0, End: 3}
	if got != want {
		t.Fatalf("window = %#v, want %#v", got, want)
	}
}

func TestVisibleListWindowKeepsCursorVisibleBelowFold(t *testing.T) {
	got := visibleListWindow(20, 10, 5)
	if got.Start > 10 || got.End <= 10 {
		t.Fatalf("window = %#v, does not include cursor 10", got)
	}
	if !got.HasAbove || !got.HasBelow {
		t.Fatalf("window = %#v, want above and below hints", got)
	}
}

func TestVisibleListWindowShowsEndForLastCursor(t *testing.T) {
	got := visibleListWindow(10, 9, 5)
	if got.End != 10 || got.Start >= 9 || !got.HasAbove || got.HasBelow {
		t.Fatalf("window = %#v, want final window with above hint only", got)
	}
}

func TestVisibleListWindowClampsCursor(t *testing.T) {
	got := visibleListWindow(10, 99, 5)
	if got.End != 10 || got.Start >= 9 {
		t.Fatalf("window = %#v, want clamped final cursor visible", got)
	}
}
