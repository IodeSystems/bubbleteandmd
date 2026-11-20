package bubbleteandmd

import (
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// joinedTabs: quotes after quoted `[!TABS]`, each opening with a heading of
// the first tab heading's depth, are more tabs (docs/decisions.md "split
// tabs joined"), with the form each renders as: one quote of tabs.
var joinedTabs = []struct{ name, in, taught string }{
	{"three options",
		"> [!TABS]\n> #### Option A\n> > [!PROS]\n> - a\n>\n> > [!CONS]\n> - b\n\n" +
			"> #### Option B\n> > [!PROS]\n> - c\n>\n> > [!CONS]\n> - d\n\n" +
			"> #### Option C\n> > [!PROS]\n> - e\n>\n> > [!CONS]\n> - f\n",
		"> [!TABS]\n> #### Option A\n> > [!PROS]\n> - a\n>\n> > [!CONS]\n> - b\n>\n" +
			"> #### Option B\n> > [!PROS]\n> - c\n>\n> > [!CONS]\n> - d\n>\n" +
			"> #### Option C\n> > [!PROS]\n> - e\n>\n> > [!CONS]\n> - f\n"},
	{"marker lines in a joined panel",
		"> [!TABS]\n> #### A\n> one\n\n> #### B\n> [!PROS]\n> - c\n> [!CONS]\n> - d\n\nAfter.\n",
		"> [!TABS]\n> #### A\n> one\n>\n> #### B\n> [!PROS]\n> - c\n> [!CONS]\n> - d\n\nAfter.\n"},
	{"two headings in a joined quote",
		"> [!TABS]\n> #### A\n> one\n\n> #### B\n> two\n>\n> #### C\n> three\n",
		"> [!TABS]\n> #### A\n> one\n>\n> #### B\n> two\n>\n> #### C\n> three\n"},
}

// notJoined: quotes that stay what they are after quoted tabs.
var notJoined = []struct{ name, in string }{
	{"deeper heading", "> [!TABS]\n> #### A\n> one\n\n> ##### B\n> two\n"},
	{"shallower heading", "> [!TABS]\n> #### A\n> one\n\n> ### B\n> two\n"},
	{"no heading", "> [!TABS]\n> #### A\n> one\n\n> two\n"},
	{"text between", "> [!TABS]\n> #### A\n> one\n\nText.\n\n> #### B\n> two\n"},
	{"marker-only tabs", "> [!TABS]\n#### A\none\n\n> #### B\n> two\n"},
}

// TestJoinedTabsRender: joined tabs render as one quote of tabs,
// byte-equal, whole and streamed token by token; the quotes in notJoined
// render as they do alone after the tabs.
func TestJoinedTabsRender(t *testing.T) {
	var run, passed atomic.Int64
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 78} {
			r := newTestRenderer(t, style, w)
			for _, c := range joinedTabs {
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
				if strings.Contains(xansi.Strip(got), "[!") {
					t.Errorf("%s/w%d/%s: a marker prints:\n%s", style, w, c.name, xansi.Strip(got))
				}
				checkStream(t, r, c.in, corpus.TokenPrefixes(c.in), &run, &passed)
			}
			for _, c := range notJoined {
				got, err := r.Render(c.in)
				if err != nil {
					t.Fatal(err)
				}
				if n := strings.Count(xansi.Strip(got), "─"); n == 0 {
					t.Errorf("%s/w%d/%s: no tabs drawn", style, w, c.name)
				}
				if strings.Contains(xansi.Strip(got), "A · B") {
					t.Errorf("%s/w%d/%s: joined:\n%s", style, w, c.name, xansi.Strip(got))
				}
				checkStream(t, r, c.in, corpus.TokenPrefixes(c.in), &run, &passed)
			}
		}
	}
	if run.Load() != passed.Load() {
		t.Errorf("%d of %d prefix checks passed", passed.Load(), run.Load())
	}
}

// TestJoinedTabsNoShrink: while testdata/directives/tabsjoined.md and the
// cases stream token by token and rune by rune into a Doc (wide blocks
// on), the line count goes down only where the taught forms' does
// (knownShrink: a `[!CONS]` marker line in a panel, drawn until its first
// item sets the sides side by side; the quoted taught form shows the same
// frame), and Final takes no lines away.
func TestJoinedTabsNoShrink(t *testing.T) {
	b, err := directiveFiles.ReadFile("testdata/directives/tabsjoined.md")
	if err != nil {
		t.Fatal(err)
	}
	docs := []struct{ name, text string }{{"tabsjoined", string(b)}}
	for _, c := range joinedTabs {
		docs = append(docs, struct{ name, text string }{c.name, c.in})
	}
	ps := []pattern{{"token", corpus.TokenPrefixes}, {"rune", runePrefixes}}
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{40, 79} {
			for _, d := range docs {
				for _, pat := range ps {
					doc := NewDoc(newWideRenderer(t, style, w, w), "k", NewCache(256))
					last, lastP := 0, ""
					for i, p := range pat.prefixes(d.text) {
						if _, err := doc.Frame(p); err != nil {
							t.Fatal(err)
						}
						if n := doc.Lines(); n < last && !knownShrink(lastP, p) {
							t.Errorf("%s/w%d/%s/%s: frame %d: lines %d → %d:\n%q\n→ %q", style, w, d.name, pat.name, i, last, n, tailOf(lastP), tailOf(p))
						}
						last, lastP = doc.Lines(), p
					}
					if _, err := doc.Final(d.text); err != nil {
						t.Fatal(err)
					}
					if doc.Lines() < last {
						t.Errorf("%s/w%d/%s/%s: the end shrank %d → %d", style, w, d.name, pat.name, last, doc.Lines())
					}
				}
			}
		}
	}
}

// TestJoinedTabsStops: joined tabs are one Doc stop with every panel.
func TestJoinedTabsStops(t *testing.T) {
	for _, style := range docStyles {
		d := newTestDoc(t, style, 76)
		out := docFinal(t, d, joinedTabs[0].in)
		ss := d.Stops()
		kinds := make([]StopKind, len(ss))
		for i, s := range ss {
			kinds[i] = s.Kind
		}
		if !slices.Equal(kinds, []StopKind{StopTabs}) {
			t.Fatalf("%s: stops %+v", style, ss)
		}
		if plain := xansi.Strip(out); !strings.Contains(plain, "Option A · Option B · Option C") {
			t.Errorf("%s: three tabs in one form:\n%s", style, plain)
		}
	}
}
