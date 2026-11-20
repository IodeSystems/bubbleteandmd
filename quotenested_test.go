package bubbleteandmd

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// quoteNestedExample: a plain quote holding a pros and a cons, each a
// marker-only quote taking the list after it (flowdown
// core/remark-answer-containers.ts:263-286, inside quotes too).
const quoteNestedExample = "> #### Option B\n> > [!PROS]\n> - fits the current layout\n>\n> > [!CONS]\n> - reverses an earlier change\n"

// TestQuoteNestedForms: forms in a plain quote draw (no marker prints) as
// they do at top level, behind the quote's rail; a plain quote with no
// form keeps its marker lines as text.
func TestQuoteNestedForms(t *testing.T) {
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 78} {
			r := newTestRenderer(t, style, w)
			got, err := r.Render(quoteNestedExample)
			if err != nil {
				t.Fatal(err)
			}
			plain := xansi.Strip(got)
			if strings.Contains(plain, "[!") {
				t.Errorf("%s/w%d: a marker prints:\n%s", style, w, plain)
			}
			for _, want := range []string{"Pros", "Cons", "fits the", "reverses"} {
				if !strings.Contains(plain, want) {
					t.Errorf("%s/w%d: %q missing:\n%s", style, w, want, plain)
				}
			}
			text, err := r.Render("> A plain quote.\n> [!PROS]\n> - a\n")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(xansi.Strip(text), "[!PROS]") {
				t.Errorf("%s/w%d: a marker line in a plain quote is not text:\n%s", style, w, xansi.Strip(text))
			}
		}
	}
}

// TestQuoteNestedNoShrink: while the example and
// testdata/directives/quotenested.md stream token by token and rune by
// rune into a Doc (wide blocks on), the line count never goes down, and
// Final takes no lines away. No exceptions.
func TestQuoteNestedNoShrink(t *testing.T) {
	b, err := directiveFiles.ReadFile("testdata/directives/quotenested.md")
	if err != nil {
		t.Fatal(err)
	}
	ps := []pattern{{"token", corpus.TokenPrefixes}, {"rune", runePrefixes}}
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{40, 79} {
			for _, d := range []struct{ name, text string }{{"example", quoteNestedExample}, {"quotenested", string(b)}} {
				for _, pat := range ps {
					checkNoShrink(t, style, w, d.name, pat, d.text)
				}
			}
		}
	}
}
