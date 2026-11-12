package directive

import "testing"

// TestCursorRow: the cursor row among drawn rows, checked against the
// parent walk over the whole tree (a hidden row shows on its nearest shown
// ancestor).
func TestCursorRow(t *testing.T) {
	// a repeated name, so the plain path alone does not pick the row.
	rows := ParseTree("README.md\nsrc/\n├── app/\n│   └── main.go\n└── lib/\n    └── util.go\nsrc/\n└── app/\n    └── x.go\n")
	closedSets := []map[string]bool{
		nil,
		{rows[1].Path: true},
		{rows[2].Path: true},
		{rows[7].Path: true},
		{rows[6].Path: true, rows[2].Path: true},
	}
	for _, closed := range closedSets {
		shown := make([]bool, len(rows))
		var drawn []string
		var at []int
		for i, r := range rows {
			shown[i] = r.Parent < 0 || (shown[r.Parent] && !closed[rows[r.Parent].Path])
			if shown[i] {
				drawn = append(drawn, r.Path)
				at = append(at, i)
			}
		}
		for i, r := range rows {
			want := i
			for !shown[want] {
				want = rows[want].Parent
			}
			if got := at[CursorRow(drawn, r.Path)]; got != want {
				t.Errorf("closed %v, cursor %s: row %d, want %d", closed, r.Path, got, want)
			}
		}
		for _, c := range []string{"", "nope", "nope#99"} {
			if got := CursorRow(drawn, c); got != 0 {
				t.Errorf("cursor %q: %d, want 0", c, got)
			}
		}
	}
	if CursorRow(nil, "x#0") != -1 {
		t.Error("no rows")
	}
}
