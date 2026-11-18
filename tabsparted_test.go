package bubbleteandmd

import (
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// partedTabs: a `---` before a heading of the tabs' depth parts two tabs
// (flowdown v0.7.1, docs/decisions.md "rule between tabs"), with the form it
// renders as: the same tabs with no rules.
var partedTabs = []struct{ name, in, taught string }{
	{"blank lines", "> [!TABS]\n#### npm\n`npm i jq`\n\n---\n\n#### bun\n`bun add jq`\n\n---\n\nAfter.\n",
		"> [!TABS]\n#### npm\n`npm i jq`\n\n#### bun\n`bun add jq`\n\n---\n\nAfter.\n"},
	// (`One.\n---` would be a setext heading, as anywhere.)
	{"no blank line under the rule", "> [!TABS]\n#### A\n- One.\n\n---\n#### B\nTwo.\n",
		"> [!TABS]\n#### A\n- One.\n\n#### B\nTwo.\n"},
}

// TestPartedTabsRender: parted tabs render as the tabs without their
// rules, byte-equal, whole and streamed token by token; the rule after the
// last tab still draws.
func TestPartedTabsRender(t *testing.T) {
	var run, passed atomic.Int64
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 78} {
			r := newTestRenderer(t, style, w)
			for _, c := range partedTabs {
				got, err := r.Render(c.in)
				if err != nil {
					t.Fatal(err)
				}
				want, err := r.Render(c.taught)
				if err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Errorf("%s/w%d/%s: %s", style, w, c.name, firstDiff(want, got))
				}
				checkStream(t, r, c.in, corpus.TokenPrefixes(c.in), &run, &passed)
			}
		}
	}
	if run.Load() != passed.Load() {
		t.Errorf("%d of %d prefix checks passed", passed.Load(), run.Load())
	}
}

// TestPartedTabsNoShrink: while testdata/directives/tabsparted.md and the
// cases stream token by token into a Doc, the line count goes down only
// where the taught forms' does (knownShrink), and Final takes no lines away.
func TestPartedTabsNoShrink(t *testing.T) {
	b, err := directiveFiles.ReadFile("testdata/directives/tabsparted.md")
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{"tabsparted": string(b)}
	for _, c := range partedTabs {
		docs[c.name] = c.in
	}
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{40, 79} {
			for name, text := range docs {
				doc := NewDoc(newWideRenderer(t, style, w, w), "k", NewCache(256))
				last, lastP := 0, ""
				for i, p := range corpus.TokenPrefixes(text) {
					if _, err := doc.Frame(p); err != nil {
						t.Fatal(err)
					}
					if n := doc.Lines(); n < last && !knownShrink(lastP, p) {
						t.Errorf("%s/w%d/%s: frame %d: lines %d → %d:\n%q\n→ %q", style, w, name, i, last, n, tailOf(lastP), tailOf(p))
					}
					last, lastP = doc.Lines(), p
				}
				if _, err := doc.Final(text); err != nil {
					t.Fatal(err)
				}
				if doc.Lines() < last {
					t.Errorf("%s/w%d/%s: the end shrank %d → %d", style, w, name, last, doc.Lines())
				}
			}
		}
	}
}

// TestPartedTabsStops: parted tabs are one Doc stop with every panel, and
// the note in the last panel is not a stop of its own.
func TestPartedTabsStops(t *testing.T) {
	b, err := directiveFiles.ReadFile("testdata/directives/tabsparted.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, style := range docStyles {
		d := newTestDoc(t, style, 76)
		out := docFinal(t, d, string(b))
		ss := d.Stops()
		kinds := make([]StopKind, len(ss))
		for i, s := range ss {
			kinds[i] = s.Kind
		}
		if !slices.Equal(kinds, []StopKind{StopTabs, StopTabs}) {
			t.Fatalf("%s: stops %+v", style, ss)
		}
		plain := xansi.Strip(out)
		if !strings.Contains(plain, "npm · bun · pnpm") {
			t.Errorf("%s: three tabs in one form:\n%s", style, plain)
		}
	}
}
