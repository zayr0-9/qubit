package listwindow

import "testing"

func TestVisibleFitsAllRows(t *testing.T) {
	got := Visible(3, 2, 5)
	want := Window{Start: 0, End: 3}
	if got != want {
		t.Fatalf("window = %#v, want %#v", got, want)
	}
}

func TestVisibleKeepsCursorVisibleBelowFold(t *testing.T) {
	got := Visible(20, 10, 5)
	if got.Start > 10 || got.End <= 10 {
		t.Fatalf("window = %#v, does not include cursor 10", got)
	}
	if !got.HasAbove || !got.HasBelow {
		t.Fatalf("window = %#v, want above and below hints", got)
	}
}

func TestVisibleShowsEndForLastCursor(t *testing.T) {
	got := Visible(10, 9, 5)
	if got.End != 10 || got.Start >= 9 || !got.HasAbove || got.HasBelow {
		t.Fatalf("window = %#v, want final window with above hint only", got)
	}
}

func TestVisibleClampsCursor(t *testing.T) {
	got := Visible(10, 99, 5)
	if got.End != 10 || got.Start >= 9 {
		t.Fatalf("window = %#v, want clamped final cursor visible", got)
	}
}
