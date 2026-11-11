package bubbleteandmd

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/glamour/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
)

// wideAnswer is dun's (internal/tui/wide_test.go): prose, a code block with
// a 150-column line, prose, and a table wider than the prose column.
var wideAnswer = "Here is the call.\n\n```go\nfunc main() { fmt.Println(\"" + strings.Repeat("x", 120) + "\") } // END\n" +
	"return\n```\n\nAnd the table:\n\n| name | what it does |\n|---|---|\n| poly-lsp | " + strings.Repeat("code navigation ", 7) + "TAIL |\n"

// wideCode is a fence of n lines, every 7th one 150 columns or more.
func wideCode(n int) string {
	var b strings.Builder
	b.WriteString("```go\n")
	for i := 0; i < n; i++ {
		if i%7 == 3 {
			fmt.Fprintf(&b, "\tlog.Printf(\"step %d: %s\", %s) // tail %d\n", i, strings.Repeat("value ", 18), strings.Repeat("arg, ", 6), i)
			continue
		}
		fmt.Fprintf(&b, "\tx%d := f(%d)\n", i, i)
	}
	b.WriteString("```\n")
	return b.String()
}

// wideTable is a table of n rows whose last column runs past 120 columns;
// row widths grow with i so the natural width changes while it streams.
func wideTable(n int) string {
	var b strings.Builder
	b.WriteString("| # | name | what it does |\n| ---: | --- | --- |\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "| %d | tool-%d | %s[docs](https://example.com/%d) end %d |\n", i, i, strings.Repeat("code navigation ", 3+i%5+i/20), i%4, i)
	}
	return b.String()
}

// wideDocs: documents with code lines and tables wider than every test width.
func wideDocs() []invCase {
	return []invCase{
		{"wide-answer", wideAnswer},
		{"wide-code", "Here is the file:\n\n" + wideCode(120) + "\nDone, with *prose* that wraps at the word wrap width as before, " + strings.Repeat("and keeps going ", 8) + ".\n"},
		{"wide-code-tail", "Writing it:\n\n" + wideCode(90)},
		{"wide-table", "# Tools\n\n" + wideTable(60) + "\nProse after the table.\n"},
		{"wide-table-tail", "Results:\n\n" + wideTable(70)},
		{"wide-mixed", "Mixed:\n\n- a list item with code:\n\n  ```sh\n  " + strings.Repeat("echo nested ", 14) + "\n  ```\n\n" +
			"```\n" + strings.Repeat("東京 ", 40) + "\n\tindent\t" + strings.Repeat("y", 100) + "\n```\n\n" +
			"| a | b |\n|---|---|\n| `" + strings.Repeat("code ", 25) + "` | **" + strings.Repeat("bold ", 20) + "** |\n\n" +
			"> [!NOTE]\n> " + strings.Repeat("quoted ", 30) + "\n\n```python title=\"long.py\"\n" + strings.Repeat("z", 140) + "\n```\n\n" +
			"~~~\n" + strings.Repeat("tilde ", 30) + "\n"},
	}
}

func newWideRenderer(t *testing.T, style string, w, pane int) *DirectiveRenderer {
	t.Helper()
	r := newTestRenderer(t, style, w)
	r.WideBlocks(pane)
	return r
}

func rows(out string) []string { return strings.Split(out, "\n") }

// At widths below the code line and the table rows: each code line and each
// table row is one row, cut at the pane with what is left said at the edge.
// Prose still wraps at the word wrap width. (dun TestWide_CutNotWrapped.)
func TestWideCutNotWrapped(t *testing.T) {
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 49, 78, 98, 100} {
			out, err := newWideRenderer(t, style, w, w).Render(wideAnswer)
			if err != nil {
				t.Fatal(err)
			}
			rs := rows(out)
			code, table := -1, -1
			for i, r := range rs {
				if strings.Contains(xansi.Strip(r), "func main()") {
					code = i
				}
				if strings.Contains(xansi.Strip(r), "poly-lsp") {
					table = i
				}
				if xansi.StringWidth(r) > w {
					t.Errorf("%s/%d: row wider than the pane (%d): %q", style, w, xansi.StringWidth(r), r)
				}
			}
			if code < 0 || table < 0 {
				t.Fatalf("%s/%d: no code or table row:\n%s", style, w, out)
			}
			if !strings.Contains(rs[code], " cols →") || !strings.HasPrefix(strings.TrimSpace(xansi.Strip(rs[code+1])), "return") {
				t.Errorf("%s/%d: the code line wrapped or has no edge note:\n%q\n%q", style, w, rs[code], rs[code+1])
			}
			if !strings.Contains(rs[table], " cols →") || strings.TrimSpace(xansi.Strip(rs[table+1])) != "" {
				t.Errorf("%s/%d: the table row wrapped or has no edge note:\n%q\n%q", style, w, rs[table], rs[table+1])
			}
			if strings.Contains(xansi.Strip(out), "END") || strings.Contains(xansi.Strip(out), "TAIL") {
				t.Errorf("%s/%d: the cut part is on screen:\n%s", style, w, out)
			}
			// The note counts the cells cut off: shown + N = the line's width
			// (read from a render with a pane wide enough for all of it).
			full, err := newWideRenderer(t, style, w, 2000).Render(wideAnswer)
			if err != nil {
				t.Fatal(err)
			}
			for i, key := range map[int]string{code: "func main()", table: "poly-lsp"} {
				cw := 0
				for _, l := range rows(full) {
					if strings.Contains(xansi.Strip(l), key) {
						cw = xansi.StringWidth(strings.TrimRight(xansi.Strip(l), " "))
					}
				}
				_, note := edgeNote(cw, w)
				if note == "" || !strings.HasSuffix(xansi.Strip(rs[i]), note) {
					t.Errorf("%s/%d: %s row %q does not end in %q (width %d)", style, w, key, xansi.Strip(rs[i]), note, cw)
				}
			}
		}
	}
}

// With a pane wider than the lines the 150-column line fits: no cut, no
// note; prose still wraps at the word wrap width.
// (dun TestWide_FitsOnAWideScreen.)
func TestWideFitsOnAWideScreen(t *testing.T) {
	r := newWideRenderer(t, "dark", 78, 180)
	out, err := r.Render(wideAnswer)
	if err != nil {
		t.Fatal(err)
	}
	if p := xansi.Strip(out); !strings.Contains(p, "// END") || !strings.Contains(p, "TAIL") || strings.Contains(p, "cols →") {
		t.Errorf("at pane 180:\n%s", out)
	}
	long := strings.Repeat("prose that wraps ", 12)
	out, err = r.Render(long + "\n\n" + wideAnswer)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range rows(out) {
		if strings.Contains(l, "prose that wraps") && xansi.StringWidth(l) > 78 {
			t.Errorf("prose row wider than the word wrap width: %d", xansi.StringWidth(l))
		}
	}
}

// Wide blocks change nothing in a document whose code and tables fit: the
// bytes are the off renderer's (and glamour's).
func TestWideFitsUnchanged(t *testing.T) {
	cases := append(rendererCases(t, 2000), tableDocs()...)
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{78, 98} {
			on, off := newWideRenderer(t, style, w, w+40), newTestRenderer(t, style, w)
			for _, c := range cases {
				if hasWide(on, c.text) {
					continue
				}
				a, err := on.Render(c.text)
				if err != nil {
					t.Fatal(err)
				}
				b, err := off.Render(c.text)
				if err != nil {
					t.Fatal(err)
				}
				if a != b {
					t.Errorf("%s/w%d/%s: wide on differs with no wide block: %s", style, w, c.name, firstDiff(b, a))
				}
			}
		}
	}
}

// hasWide reports whether a top-level block of text is wide for r.
func hasWide(r *DirectiveRenderer, text string) bool {
	for _, b := range blocks.Split(Sanitize(text)) {
		src, math := mathFences(b.Source)
		if _, form := directive.Parse(src); !form && !math && r.wideWidth(src) > 0 {
			return true
		}
	}
	return false
}

// A code block's lines that fit the word wrap width have glamour's bytes,
// padding included, also when the block is wide.
func TestWideFittingLinesAsGlamour(t *testing.T) {
	for _, style := range []string{"dark", "light", "notty"} {
		w := 60
		short := "```go\nfunc f() {\n\treturn 1\n}\n```\n"
		long := "```go\nfunc f() {\n\treturn 1\n}\n" + strings.Repeat("// long ", 20) + "\n```\n"
		g, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(w))
		if err != nil {
			t.Fatal(err)
		}
		want, err := g.Render(short)
		if err != nil {
			t.Fatal(err)
		}
		got, err := newWideRenderer(t, style, w, w).Render(long)
		if err != nil {
			t.Fatal(err)
		}
		wl, gl := rows(want), rows(got)
		// Rows before the long line: the margin row and the three code rows.
		for i := 0; i < 5; i++ {
			if untie(wl[i]) != untie(gl[i]) {
				t.Errorf("%s: row %d:\n want %q\n  got %q", style, i, wl[i], gl[i])
			}
		}
	}
}

// A wide table renders at its natural column widths: one row per source
// row, no column stretched or shrunk.
func TestWideTableNatural(t *testing.T) {
	src := wideTable(12)
	for _, style := range []string{"dark", "light", "notty"} {
		r := newWideRenderer(t, style, 40, 400)
		out, err := r.Render(src)
		if err != nil {
			t.Fatal(err)
		}
		var body []string
		for _, l := range rows(out) {
			if strings.TrimSpace(xansi.Strip(l)) != "" {
				body = append(body, l)
			}
		}
		// header, separator, 12 rows, footer links list
		if got := strings.Count(out, "tool-"); got != 12 {
			t.Errorf("%s: %d tool rows", style, got)
		}
		for _, l := range body {
			p := xansi.Strip(l)
			if strings.Contains(p, "tool-") && !strings.Contains(p, "end ") {
				t.Errorf("%s: a row wrapped: %q", style, p)
			}
		}
		// No stretch: the widest row ends in its own cell's text.
		widest := 0
		for _, l := range body {
			if strings.Contains(l, "tool-") {
				widest = max(widest, xansi.StringWidth(strings.TrimRight(xansi.Strip(l), " ")))
			}
		}
		nat, ok := r.nat.Width(src)
		if !ok {
			t.Fatal("no natural width")
		}
		if want := nat + r.margin() - 1; widest != want {
			t.Errorf("%s: widest row %d, natural %d + margins", style, widest, nat)
		}
	}
}

func TestKeepCells(t *testing.T) {
	pad := "\x1b[38;5;252m \x1b[m"
	for _, c := range []struct{ in, want string }{
		{"abc", "ab"},
		{"\x1b[1mabc\x1b[m", "\x1b[1mab\x1b[m"},
		{"a" + pad + pad + pad, "a" + pad},
		{"東京x", "東"},
		{"\x1b]8;;http://x\x07link\x1b]8;;\x07", "\x1b]8;;http://x\x07li\x1b]8;;\x07"},
		{"ab", "ab"},
		{"a\x1b[0mb\x1b[31mc", "a\x1b[0mb"},
	} {
		if got := keepCells(c.in, 2); got != c.want {
			t.Errorf("keepCells(%q, 2) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEdgeNote(t *testing.T) {
	// room: dun's, the first room from 12 up the note fits (N depends on it).
	for _, c := range []struct{ w, pane, room int }{{150, 78, 13}, {100, 40, 13}, {2000, 98, 15}, {300, 78, 14}, {30, 20, 0}} {
		room, note := edgeNote(c.w, c.pane)
		if room != c.room {
			t.Errorf("edgeNote(%d, %d): room %d %q, want %d", c.w, c.pane, room, note, c.room)
			continue
		}
		if room == 0 {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(strings.TrimLeft(note, " "), "… +%d cols →", &n); err != nil || n != c.w-(c.pane-room) || xansi.StringWidth(note) != room {
			t.Errorf("edgeNote(%d, %d) = %q (n %d)", c.w, c.pane, note, n)
		}
	}
}

// TestWideInvariant: with WideBlocks, Frame and Final equal the whole render
// at every prefix (layer-1 invariant, D7), at panes equal to and wider than
// the word wrap width.
func TestWideInvariant(t *testing.T) {
	cases := append(wideDocs(), tableDocs()...)
	cut := tokenScenarioCut
	for _, c := range rendererCases(t, cut) {
		if strings.HasPrefix(c.name, "doc-") || strings.HasPrefix(c.name, "dir-") || c.name == "scenario-answer-20k" || c.name == "scenario-code-50k" {
			cases = append(cases, c)
		}
	}
	ps := patterns(testing.Short())
	var run, passed, spliced atomic.Int64
	f0, d0 := fenceChecks.Load(), fenceDiffers.Load()
	t.Cleanup(func() {
		t.Logf("wide invariant: %d prefix checks run, %d passed; %d tail tables spliced; %d Frame checks of a growing fence (SGR-only), %d differ", run.Load(), passed.Load(), spliced.Load(), fenceChecks.Load()-f0, fenceDiffers.Load()-d0)
	})
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 78, 98} {
			for _, pane := range []int{w, w + 40} {
				for _, c := range cases {
					for _, pat := range ps {
						t.Run(fmt.Sprintf("%s/w%d/p%d/%s/%s", style, w, pane, c.name, pat.name), func(t *testing.T) {
							t.Parallel()
							text := c.text
							if pat.name == "token" && strings.HasPrefix(c.name, "scenario-") {
								text = cutUnits(text, tokenScenarioCut)
							}
							r := newWideRenderer(t, style, w, pane)
							s := checkStream(t, r, text, pat.prefixes(text), &run, &passed)
							spliced.Add(int64(s.stats.tableSpliced))
						})
					}
				}
			}
		}
	}
}

// TestWideNoShrink: dun's no-screen-jump rule (noshrink_test.go) for one
// reply: while it streams, the frame's height never goes down, and the end
// does not take rows away. Wide blocks do not wrap, so a growing code line
// or table row adds no row and takes none away. Checked on the wide
// documents at every frame prefix (and every line prefix without -short).
//
// Allowed, as dun allows rows it did not add: a shrink the same Stream with
// wide blocks off has at the same prefix (heal's, not wide blocks': a fence
// in a list item closed by heal draws one row more than the closed fence).
func TestWideNoShrink(t *testing.T) {
	ps := []pattern{{"frame", corpus.FramePrefixes}}
	if !testing.Short() {
		ps = append(ps, pattern{"line", corpus.LinePrefixes})
	}
	allowed := 0
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{40, 78, 98} {
			for _, c := range wideDocs() {
				for _, pat := range ps {
					s := NewStream(newWideRenderer(t, style, w, w), "k", NewCache(256))
					off := NewStream(newTestRenderer(t, style, w), "k", NewCache(256))
					last, lastOff, lastP := 0, 0, ""
					for _, p := range pat.prefixes(c.text) {
						out, err := s.Frame(p)
						if err != nil {
							t.Fatal(err)
						}
						o, err := off.Frame(p)
						if err != nil {
							t.Fatal(err)
						}
						h, hOff := strings.Count(out, "\n"), strings.Count(o, "\n")
						if h < last {
							if hOff < lastOff {
								allowed++
							} else {
								t.Errorf("%s/w%d/%s/%s: height %d → %d at prefix len %d (wide off: %d → %d):\n%q\n→ %q", style, w, c.name, pat.name, last, h, len(p), lastOff, hOff, tailOf(lastP), tailOf(p))
								break
							}
						}
						last, lastOff, lastP = h, hOff, p
					}
					out, err := s.Final(c.text)
					if err != nil {
						t.Fatal(err)
					}
					if h := strings.Count(out, "\n"); h < last {
						t.Errorf("%s/w%d/%s/%s: the end shrank %d → %d", style, w, c.name, pat.name, last, h)
					}
				}
			}
		}
	}
	t.Logf("%d shrinks allowed (wide off shrinks at the same prefix)", allowed)
}

// TestWideFastPaths: a wide table in the tail grows from cached rows, and
// a wide code fence in chunks, as their plain forms do.
func TestWideFastPaths(t *testing.T) {
	r := newWideRenderer(t, "dark", 78, 78)
	text := "Results:\n\n" + wideTable(160)
	s := NewStream(r, "k", NewCache(256))
	frames := corpus.FramePrefixes(text)
	for _, p := range frames {
		if _, err := s.Frame(p); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("wide table: %d of %d frames spliced", s.stats.tableSpliced, len(frames))
	if s.stats.tableSpliced < len(frames)*6/10 {
		t.Errorf("wide table: spliced %d of %d frames", s.stats.tableSpliced, len(frames))
	}
	text = "Here is the file:\n\n" + wideCode(600)
	s = NewStream(newWideRenderer(t, "dark", 78, 78), "k", NewCache(256))
	frames = corpus.FramePrefixes(text)
	for _, p := range frames {
		if _, err := s.Frame(p); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("wide code: %d of %d frames spliced", s.stats.fenceSpliced, len(frames))
	if s.stats.fenceSpliced < len(frames)/2 {
		t.Errorf("wide code: spliced %d of %d frames", s.stats.fenceSpliced, len(frames))
	}
}
