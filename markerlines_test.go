package bubbleteandmd

import (
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// markerLineCases: marker lines inside content (docs/decisions.md, "marker
// lines inside content"), each with the taught form it renders as.
var markerLineCases = []struct{ name, in, taught string }{
	{"tabs", "> [!TABS]\n> #### Use a cache\n> [!PROS]\n> - One-line change, no schema change\n> [!CONS]\n> - Stale reads after a write\n>\n> #### Leave as is\n> [!PROS]\n> - Nothing to do\n> [!CONS]\n> - Slow page loads stay, for every user\n",
		"> [!TABS]\n> #### Use a cache\n> > [!PROS]\n> > - One-line change, no schema change\n>\n> > [!CONS]\n> > - Stale reads after a write\n>\n> #### Leave as is\n> > [!PROS]\n> > - Nothing to do\n>\n> > [!CONS]\n> > - Slow page loads stay, for every user\n"},
	{"tab nested quote", "> [!TABS]\n> #### A\n> > [!PROS]\n> > - a\n> > [!CONS]\n> > - b\n>\n> #### B\n> text\n",
		"> [!TABS]\n> #### A\n> > [!PROS]\n> > - a\n>\n> > [!CONS]\n> > - b\n>\n> #### B\n> text\n"},
	{"one quote", "Intro.\n\n> [!PROS]\n> - a\n> [!CONS]\n> - b\n\nAfter.\n",
		"Intro.\n\n> [!PROS]\n> - a\n\n> [!CONS]\n> - b\n\nAfter.\n"},
}

// TestMarkerLinesRender: each input renders as its taught form, byte-equal,
// at every style, whole and streamed token by token.
func TestMarkerLinesRender(t *testing.T) {
	var run, passed atomic.Int64
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 78} {
			r := newTestRenderer(t, style, w)
			for _, c := range markerLineCases {
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
		}
	}
	if run.Load() != passed.Load() {
		t.Errorf("%d of %d prefix checks passed", passed.Load(), run.Load())
	}
}

var (
	partialHeadingRe = regexp.MustCompile(`^[> ]*#{1,6}$`)
	sideMarkerRe     = regexp.MustCompile(`^[> ]*\[!(?i:pros|cons)\]$`)
)

// knownShrink reports a shrink the taught forms show too (measured on
// them, 2026-11-18), not one marker lines add: a partial `#`-only line
// drawn as an empty heading before it becomes a panel heading (dark), and
// the first item of a side that sets a pros and cons side by side.
func knownShrink(prev, next string) bool {
	lines := strings.Split(next, "\n")
	if pl := strings.Split(prev, "\n"); partialHeadingRe.MatchString(pl[len(pl)-1]) {
		return true
	}
	return len(lines) >= 2 && sideMarkerRe.MatchString(lines[len(lines)-2])
}

// TestMarkerLinesNoShrink: while the inputs and
// testdata/directives/markerlines.md stream token by token into a Doc
// (wide blocks on, as TestShowcaseNoShrink), the line count goes down only
// where the taught forms' does (knownShrink), and Final takes no lines away.
func TestMarkerLinesNoShrink(t *testing.T) {
	b, err := directiveFiles.ReadFile("testdata/directives/markerlines.md")
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{"markerlines": string(b)}
	for _, c := range markerLineCases {
		docs[c.name] = c.in
	}
	known := 0
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{40, 79} {
			for name, text := range docs {
				doc := NewDoc(newWideRenderer(t, style, w, w), "k", NewCache(256))
				last, lastP := 0, ""
				for i, p := range corpus.TokenPrefixes(text) {
					if _, err := doc.Frame(p); err != nil {
						t.Fatal(err)
					}
					if n := doc.Lines(); n < last {
						if !knownShrink(lastP, p) {
							t.Errorf("%s/w%d/%s: frame %d: lines %d → %d:\n%q\n→ %q", style, w, name, i, last, n, tailOf(lastP), tailOf(p))
						}
						known++
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
	t.Logf("%d shrinks, each of a kind the taught forms show", known)
}

// TestMarkerLinesStops: the tabs and a details a marker line opens in a tab
// panel are Doc stops, the details nested in the tabs.
func TestMarkerLinesStops(t *testing.T) {
	b, err := directiveFiles.ReadFile("testdata/directives/markerlines.md")
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
		if !slices.Equal(kinds, []StopKind{StopTabs, StopDetails}) {
			t.Fatalf("%s: stops %+v", style, ss)
		}
		lines := strings.Split(xansi.Strip(out), "\n")
		if !strings.HasPrefix(ss[1].Key, ss[0].Key+"/t2@") || !strings.Contains(lines[ss[1].Line], "More") {
			t.Errorf("%s: details stop %+v under tabs %+v", style, ss[1], ss[0])
		}
		if !d.Focus() || !d.Do(ActionNext) || d.State().Focused != ss[1].Key {
			t.Errorf("%s: focus did not reach the details: %q", style, d.State().Focused)
		}
	}
}

// TestMarkerLinesStayText: a marker mid-line, a marker line in a code fence
// (in a tab, in a note, at top level), a top-level unquoted marker line and
// `>` lines in an HTML block render as before: glamour's text, and no form
// where none was.
func TestMarkerLinesStayText(t *testing.T) {
	r := newTestRenderer(t, "notty", 78)
	for _, c := range []struct{ name, in, keep string }{
		{"mid-line in a tab", "> [!TABS]\n> #### A\n> Names [!PROS] mid-line.\n> - a\n", "Names [!PROS] mid-line."},
		{"mid-line in a quote", "> [!PROS]\n> - a\n> see [!CONS] here\n", "see [!CONS] here"},
		{"fence in a tab", "> [!TABS]\n> #### A\n> ```text\n> [!CONS]\n> - b\n> ```\n", "[!CONS]"},
		{"fence in a note", "> [!NOTE]\n> ```text\n> [!TIP]\n> ```\n", "[!TIP]"},
		{"fence at top", "```md\n> [!PROS]\n> - a\n> [!CONS]\n> - b\n```\n", "> [!CONS]"},
		{"top-level line", "[!PROS]\n- A top-level line is text, as written.\n", "[!PROS]"},
		{"html block", "<details>\n> [!DETAILS]\n> [!DETAILS]\n", "> [!DETAILS]"},
	} {
		got, err := r.Render(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(xansi.Strip(got), c.keep) {
			t.Errorf("%s: %q not kept as text:\n%s", c.name, c.keep, xansi.Strip(got))
		}
	}
	// No form at all: the bytes glamour draws.
	for _, in := range []string{"[!PROS]\n- a\n[!CONS]\n- b\n", "<details>\n> [!DETAILS]\n> [!DETAILS]\n", "```md\n> [!PROS]\n> - a\n> [!CONS]\n> - b\n```\n"} {
		got, err := r.Render(in)
		if err != nil {
			t.Fatal(err)
		}
		want, err := r.outer.g.Render(in)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%q: %s", in, firstDiff(want, got))
		}
	}
}
