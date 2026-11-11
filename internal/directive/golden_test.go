package directive

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

var update = flag.Bool("update", false, "rewrite golden files")

// golden compares got with testdata/golden/<name>.golden; -update rewrites it.
func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name+".golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("golden %s: %v (run with -update)", p, err)
	}
	if string(want) != got {
		t.Errorf("golden %s differs\n got %q\nwant %q", p, got, string(want))
	}
}

// checkWidth fails for any line wider than width cells.
func checkWidth(t *testing.T, lines []string, width int) {
	t.Helper()
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("line %d is %d cells, max %d: %q", i, w, width, l)
		}
	}
}
