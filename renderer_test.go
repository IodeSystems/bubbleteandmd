package bubbleteandmd

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
	"github.com/iodesystems/bubbleteandmd/internal/join"
)

//go:embed testdata/directives/*.md
var directiveFiles embed.FS

// directiveDocs are the extra documents with every form, nested forms, and
// forms next to code, headings, lists and rules.
func directiveDocs(t *testing.T) []invCase {
	t.Helper()
	es, err := directiveFiles.ReadDir("testdata/directives")
	if err != nil {
		t.Fatal(err)
	}
	var cs []invCase
	for _, e := range es {
		b, err := directiveFiles.ReadFile("testdata/directives/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		cs = append(cs, invCase{"dir-" + strings.TrimSuffix(e.Name(), ".md"), string(b)})
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].name < cs[j].name })
	return cs
}

// rendererCases: cut is how many UTF-16 units of each scenario stream; 0
// streams them whole.
func rendererCases(t *testing.T, cut int) []invCase {
	var cs []invCase
	for _, d := range corpus.Docs() {
		cs = append(cs, invCase{"doc-" + d.Name, d.Text})
	}
	for _, sc := range corpus.Scenarios() {
		text := sc.Text
		if cut > 0 {
			text = cutUnits(text, cut)
		}
		cs = append(cs, invCase{"scenario-" + sc.Name, text})
	}
	return append(cs, directiveDocs(t)...)
}

func newTestRenderer(t *testing.T, style string, w int) *DirectiveRenderer {
	t.Helper()
	r, err := NewRenderer(*styles.DefaultStyles[style], w)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// hasForm reports whether any top-level block of text holds a form or a
// `$$` formula: whether the renderer draws anything glamour does not.
func hasForm(text string) bool {
	for _, b := range blocks.Split(text) {
		if _, ok := directive.Parse(b.Source); ok {
			return true
		}
		if _, ok := mathFences(b.Source); ok {
			return true
		}
	}
	return false
}

// TestRendererInvariant: streaming NewRenderer equals rendering the whole
// prefix with it, at every prefix (D7). Under -short scenarios stream only
// their first tokenScenarioCut units.
func TestRendererInvariant(t *testing.T) {
	cut := scenarioCut
	if testing.Short() {
		cut = tokenScenarioCut
	}
	cases := rendererCases(t, cut)
	ps := patterns(testing.Short())
	widths := []int{40, 78, 98}
	var run, passed, forms atomic.Int64
	t.Cleanup(func() {
		t.Logf("renderer invariant: %d prefix checks run, %d passed; %d of %d cases hold forms", run.Load(), passed.Load(), forms.Load(), len(cases))
	})
	for _, c := range cases {
		if hasForm(c.text) {
			forms.Add(1)
		}
	}
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range widths {
			for _, c := range cases {
				for _, pat := range ps {
					t.Run(fmt.Sprintf("%s/w%d/%s/%s", style, w, c.name, pat.name), func(t *testing.T) {
						t.Parallel()
						text := c.text
						if pat.name == "token" && strings.HasPrefix(c.name, "scenario-") {
							text = cutUnits(text, tokenScenarioCut)
						}
						r := newTestRenderer(t, style, w)
						checkStream(t, r, text, pat.prefixes(text), &run, &passed)
					})
				}
			}
		}
	}
}

// TestRendererEnd: at the end of a whole document the stream does not move:
// Frame == Final. A text heal.Close changes (a last table row with no line
// ending reads as a row still being written) moves with plain glamour too;
// that move is heal's, so for those texts the test checks that the plain
// glamour Stream moves as well.
func TestRendererEnd(t *testing.T) {
	n, healMoves := 0, 0
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 78, 98} {
			r := newTestRenderer(t, style, w)
			g, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(w))
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range rendererCases(t, 0) {
				if heal.Close(c.text) != c.text {
					healMoves++
					if frameEqualsFinal(t, g, c.text) {
						t.Errorf("%s/w%d/%s: heal changes the end but the glamour stream does not move", style, w, c.name)
					}
					continue
				}
				n++
				if !frameEqualsFinal(t, r, c.text) {
					t.Errorf("%s/w%d/%s: Frame != Final at the end", style, w, c.name)
				}
			}
		}
	}
	t.Logf("%d whole documents: Frame == Final; %d end in a construct heal changes (checked to move with glamour too)", n, healMoves)
}

func frameEqualsFinal(t *testing.T, r Renderer, text string) bool {
	t.Helper()
	s := NewStream(r, "k", NewCache(256))
	final, err := s.Final(text)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := s.Frame(text)
	if err != nil {
		t.Fatal(err)
	}
	if untie(frame) != untie(final) {
		t.Log(firstDiff(untie(final), untie(frame)))
		return false
	}
	return true
}

// TestRendererPlain: a document with no form and no `$$` formula renders
// byte-equal to plain glamour.
func TestRendererPlain(t *testing.T) {
	var docs []invCase
	for _, d := range corpus.Docs() {
		docs = append(docs, invCase{"doc-" + d.Name, d.Text})
	}
	for _, sc := range corpus.Scenarios() {
		docs = append(docs, invCase{"scenario-" + sc.Name, cutUnits(sc.Text, scenarioCut)})
	}
	n := 50
	if testing.Short() {
		n = 10
	}
	for i, d := range corpus.Fuzz(1, n) {
		docs = append(docs, invCase{fmt.Sprintf("fuzz-%d", i+1), d})
	}
	checked := 0
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 78, 98} {
			r := newTestRenderer(t, style, w)
			g, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(w))
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range docs {
				if hasForm(d.text) {
					continue
				}
				checked++
				want, err := g.Render(d.text)
				if err != nil {
					t.Fatal(err)
				}
				got, err := r.Render(d.text)
				if err != nil {
					t.Fatal(err)
				}
				if got != want && untie(got) != untie(want) {
					t.Errorf("%s/w%d/%s: %s", style, w, d.name, firstDiff(untie(want), untie(got)))
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no document without forms")
	}
	t.Logf("%d plain renders byte-equal to glamour", checked)
}

// TestRendererDraws: each form draws (its marker is gone from the output),
// and a block after the sentinel renders in not-first place.
func TestRendererDraws(t *testing.T) {
	r := newTestRenderer(t, "notty", 78)
	for _, c := range directiveDocs(t) {
		out, err := r.Render(c.text)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range []string{"[!NOTE]", "[!TIP]", "[!STEPS]", "[!TABS]", "[!PROS]", "[!CONS]", "[!STATS]", "[!DETAILS]", "[!SOURCES]", "```tree", "$$"} {
			if strings.Contains(out, m) {
				t.Errorf("%s: %q not drawn:\n%s", c.name, m, out)
			}
		}
	}
	block := "> [!NOTE]\n> Body.\n"
	first, err := r.Render(block)
	if err != nil {
		t.Fatal(err)
	}
	later, err := r.Render(join.Sentinel + block)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(first, r.outer.p+"\n") || later == first {
		t.Fatalf("first and not-first renders: %q / %q", first, later)
	}
	if !strings.HasPrefix(later, r.outer.p+"  "+strings.Repeat(" ", 74)+"\n") {
		t.Fatalf("not-first render does not open with a blank line: %q", later)
	}
}
