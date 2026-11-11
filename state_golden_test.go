package bubbleteandmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
)

var update = flag.Bool("update", false, "rewrite golden files")

// init renders one code block in the dark style before any test runs.
// glamour v2.0.1 registers its chroma theme once per process, from the first
// style that renders a code block (ansi/codeblock.go), so a golden with code
// in it depends on which test ran first; this pins the theme to dark for
// every test of the package, also under -run.
func init() {
	r, err := glamour.NewTermRenderer(glamour.WithStyles(*styles.DefaultStyles["dark"]))
	if err == nil {
		_, _ = r.Render("```go\nx := 1\n```\n")
	}
}

// golden compares untie(got) with testdata/golden/<name>.golden; -update
// rewrites it.
func golden(t *testing.T, name, got string) {
	t.Helper()
	got = untie(got)
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
		t.Errorf("golden %s differs: %s", p, firstDiff(string(want), got))
	}
}

// TestDirectiveDocsGolden: each testdata/directives document as
// DirectiveRenderer.Render draws it (no state), per style and width.
func TestDirectiveDocsGolden(t *testing.T) {
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{36, 76} {
			r := newTestRenderer(t, style, w)
			for _, d := range directiveDocs(t) {
				out, err := r.Render(d.text)
				if err != nil {
					t.Fatal(err)
				}
				golden(t, fmt.Sprintf("docs/%s_%s_%d", strings.TrimPrefix(d.name, "dir-"), style, w), out)
			}
		}
	}
}
