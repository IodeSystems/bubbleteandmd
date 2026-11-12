package bubbleteandmd

import (
	"os"
	"testing"
	"unicode/utf8"

	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// runePrefixes is every prefix of s that ends on a rune boundary.
func runePrefixes(s string) []string {
	var out []string
	for at := 0; at < len(s); {
		_, n := utf8.DecodeRuneInString(s[at:])
		at += n
		out = append(out, s[:at])
	}
	return out
}

// TestShowcaseNoShrink: Carl's no-shrink rule on the demo's reply. While
// cmd/demo/showcase.md streams into a Doc (wide blocks on, as the demo),
// the line count never goes down between two frames, and Final does not
// take lines away. No exceptions are listed: a shrink here is a bug.
// Token prefixes (seed 42) always; every rune prefix without -short, and
// the corpus documents with forms at token granularity without -short.
func TestShowcaseNoShrink(t *testing.T) {
	b, err := os.ReadFile("cmd/demo/showcase.md")
	if err != nil {
		t.Fatal(err)
	}
	type doc struct{ name, text string }
	docs := []doc{{"showcase", string(b)}}
	ps := []pattern{{"token", corpus.TokenPrefixes}}
	if !testing.Short() {
		ps = append(ps, pattern{"rune", runePrefixes})
		for _, d := range corpus.Docs() {
			if hasForm(d.Text) {
				docs = append(docs, doc{"doc-" + d.Name, d.Text})
			}
		}
	}
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{40, 79} {
			for _, d := range docs {
				for _, pat := range ps {
					if pat.name == "rune" && d.name != "showcase" {
						continue
					}
					checkNoShrink(t, style, w, d.name, pat, d.text)
				}
			}
		}
	}
}

func checkNoShrink(t *testing.T, style string, w int, name string, pat pattern, text string) {
	t.Helper()
	doc := NewDoc(newWideRenderer(t, style, w, w), "k", NewCache(256))
	last, lastP, shrinks := 0, "", 0
	for i, p := range pat.prefixes(text) {
		if _, err := doc.Frame(p); err != nil {
			t.Fatal(err)
		}
		if n := doc.Lines(); n < last {
			shrinks++
			if shrinks <= 10 {
				t.Errorf("%s/w%d/%s/%s: frame %d: lines %d → %d:\n%q\n→ %q", style, w, name, pat.name, i, last, n, tailOf(lastP), tailOf(p))
			}
		}
		last, lastP = doc.Lines(), p
	}
	if _, err := doc.Final(text); err != nil {
		t.Fatal(err)
	}
	if n := doc.Lines(); n < last {
		shrinks++
		t.Errorf("%s/w%d/%s/%s: the end shrank %d → %d", style, w, name, pat.name, last, n)
	}
	if shrinks > 0 {
		t.Logf("%s/w%d/%s/%s: %d shrinks", style, w, name, pat.name, shrinks)
	}
}
