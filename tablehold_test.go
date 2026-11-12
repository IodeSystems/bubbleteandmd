package bubbleteandmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

func TestHeldTable(t *testing.T) {
	hdr := "| a | b |\n| --- | --- |\n"
	row := "| 1 | 2 |\n"
	for _, c := range []struct {
		src  string
		want bool
	}{
		{"| a", true},            // header still typing
		{"| a | b |\n", true},    // no delimiter row yet: held (option A)
		{"| a | b |\n\n", false}, // a blank line ends it: a paragraph
		{"| a | b |\n|", true},   // delimiter row still typing
		{"| a | b |\n| -", true},
		{"| a | b |\n| --- | --- |", true}, // no "\n" yet: a cell can still follow
		{"| a | b |\n|  |\n", false},       // whole second line, not a delimiter row
		{"| a | b |\nx", false},
		{"| not a", true},
		{hdr, false}, // the table draws from its delimiter row on
		{hdr + row, false},
		{hdr + "| 1", false},
		{"| not a table\nprose\n", false},
		{"prose\n", false},
	} {
		if got := heldTable(c.src); got != c.want {
			t.Errorf("heldTable(%q) = %v, want %v", c.src, got, c.want)
		}
	}
}

func lineCount(s string) int { return strings.Count(s, "\n") }

// ruleLine: a table's header rule as glamour draws it (notty `-|-`, styled
// `─┼─`), whole or cut with the edge note.
var ruleLine = regexp.MustCompile(`^ *[-─][-─|┼:+ ]*[-─|┼]( *… \+\d+ cols →)? *$`)

// delimRow: a GFM delimiter row.
var delimRow = regexp.MustCompile(`^\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)

// tableBody returns the number of lines after the first table rule line
// of out up to the next blank line, and whether out has a rule line with
// one header line above it and a blank line above that.
func tableBody(out string) (int, bool) {
	ls := strings.Split(xansi.Strip(out), "\n")
	for i, l := range ls {
		if !ruleLine.MatchString(l) || strings.Count(l, "-")+strings.Count(l, "─") < 3 {
			continue
		}
		if i < 2 || strings.TrimSpace(ls[i-1]) == "" || strings.TrimSpace(ls[i-2]) != "" {
			return 0, false
		}
		n := 0
		for _, b := range ls[i+1:] {
			if strings.TrimSpace(b) == "" {
				break
			}
			n++
		}
		return n, true
	}
	return 0, false
}

// sourceRows: the body rows of table block src (lines after the delimiter
// row up to a blank line), whole lines only.
func sourceRows(src string) int {
	_, after, ok := strings.Cut(src, "\n")
	if !ok {
		return 0
	}
	_, body, ok := strings.Cut(after, "\n")
	if !ok {
		return 0
	}
	n := 0
	for _, l := range strings.SplitAfter(body, "\n") {
		if !strings.HasSuffix(l, "\n") || strings.TrimSpace(l) == "" {
			break
		}
		n++
	}
	return n
}

// TestTableRows: table-20k at frame prefixes (Carl 2026-11-11: rows draw as
// they come, single line, widths may re-adjust). Held only before the
// delimiter row's "\n"; after it, the frame has one line per whole body
// row (a partial row is hidden) and never shrinks; Final has all rows.
// w40 is narrower than the table (cut); w98 with WideBlocks too.
func TestTableRows(t *testing.T) {
	text := scenario(t, "table-20k")
	at := strings.Index(text, "|")
	delimEnd := at + strings.Index(text[at:], "---")
	delimEnd += strings.IndexByte(text[delimEnd:], '\n')
	for _, style := range []string{"dark", "notty"} {
		for _, c := range []struct{ w, pane int }{{40, 0}, {78, 0}, {98, 0}, {78, 120}} {
			t.Run(fmt.Sprintf("%s/w%d/pane%d", style, c.w, c.pane), func(t *testing.T) {
				r := newTestRenderer(t, style, c.w)
				r.WideBlocks(c.pane)
				s := NewStream(r, "k", NewCache(256))
				prev := 0
				frames := corpus.FramePrefixes(text)
				for i, p := range frames {
					out, err := s.Frame(p)
					if err != nil {
						t.Fatal(err)
					}
					if n := lineCount(out); n < prev {
						t.Fatalf("frame %d shrank: %d -> %d lines", i, prev, n)
					}
					prev = lineCount(out)
					if want := len(p) > at && len(p) <= delimEnd; s.stats.lastHeld != want {
						t.Fatalf("frame %d (len %d): held %v, want %v", i, len(p), s.stats.lastHeld, want)
					}
					if len(p) <= delimEnd {
						continue
					}
					n, ok := tableBody(out)
					if want := sourceRows(p[at:]); !ok || n != want {
						t.Fatalf("frame %d: %d body lines (table %v), want %d rows:\n%s", i, n, ok, want, xansi.Strip(out))
					}
				}
				final, err := s.Final(text)
				if err != nil {
					t.Fatal(err)
				}
				if n, ok := tableBody(final); !ok || n != 160 || lineCount(final) < prev {
					t.Fatalf("Final: %d body lines (table %v), %d lines after %d", n, ok, lineCount(final), prev)
				}
				t.Logf("spliced %d, held %d", s.stats.tableSpliced, s.stats.held)
				if s.stats.tableSpliced < len(frames)*8/10 {
					t.Errorf("spliced %d of %d frames from cached rows (T5)", s.stats.tableSpliced, len(frames))
				}
			})
		}
	}
}

// tableCorpora: documents with top-level tables, wide and narrow.
func tableCorpora(t *testing.T) map[string]string {
	t.Helper()
	show, err := os.ReadFile("cmd/demo/showcase.md")
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{
		"table-20k":  scenario(t, "table-20k"),
		"answer-20k": scenario(t, "answer-20k"),
		"showcase":   string(show),
	}
	for _, c := range tableDocs() {
		docs[c.name] = c.text
	}
	return docs
}

// TestTableRowsSingleLine: every top-level table block renders one line per
// body row, at a width narrower and wider than the table, with and without
// WideBlocks.
func TestTableRowsSingleLine(t *testing.T) {
	for name, doc := range tableCorpora(t) {
		for _, style := range []string{"dark", "notty"} {
			for _, w := range []int{40, 79} {
				for _, pane := range []int{0, w + 30} {
					r := newTestRenderer(t, style, w)
					r.WideBlocks(pane)
					for _, b := range blocks.Split(doc) {
						ls := strings.SplitN(b.Source, "\n", 3)
						if !strings.HasPrefix(b.Source, "|") || len(ls) < 3 || !delimRow.MatchString(ls[1]) {
							continue
						}
						out, err := r.Render(b.Source)
						if err != nil {
							t.Fatal(err)
						}
						n, ok := tableBody(out)
						if want := sourceRows(b.Source + "\n"); !ok || n != want {
							t.Errorf("%s %s/w%d/pane%d: table at %d: %d lines (read %v), want %d rows:\n%s", name, style, w, pane, b.Offset, n, ok, want, xansi.Strip(out))
						}
					}
				}
			}
		}
	}
}

// TestTableNoShrinkTokens: the table documents at token prefixes, w40/w79,
// dark/notty: no frame has fewer lines than the one before while a table
// is the tail block or just ended; Final equals Render.
func TestTableNoShrinkTokens(t *testing.T) {
	if testing.Short() {
		t.Skip("token prefixes of 20k docs: long")
	}
	for name, doc := range tableCorpora(t) {
		for _, style := range []string{"dark", "notty"} {
			for _, w := range []int{40, 79} {
				t.Run(fmt.Sprintf("%s/%s/w%d", name, style, w), func(t *testing.T) {
					tableNoShrink(t, newTestRenderer(t, style, w), doc, corpus.TokenPrefixes(doc))
				})
			}
		}
	}
}

// tableNoShrink streams prefixes and fails on a shrink where the tail block
// of the frame before or of this one starts with `|`; other shrinks are
// logged (not tables). Final must equal r.Render(Sanitize(doc)).
func tableNoShrink(t *testing.T, r *DirectiveRenderer, doc string, prefixes []string) {
	t.Helper()
	s := NewStream(r, "k", NewCache(256))
	prev, prevTail, other := 0, "", 0
	for i, p := range prefixes {
		out, err := s.Frame(p)
		if err != nil {
			t.Fatal(err)
		}
		bs := blocks.Split(p)
		tail := bs[len(bs)-1].Source
		if n := lineCount(out); n < prev {
			if strings.HasPrefix(tail, "|") || strings.HasPrefix(prevTail, "|") {
				t.Fatalf("prefix %d (len %d) shrank %d -> %d lines at a table tail %q", i, len(p), prev, n, tail[:min(len(tail), 80)])
			}
			other++
		}
		prev, prevTail = lineCount(out), tail
	}
	fin, err := s.Final(doc)
	if err != nil {
		t.Fatal(err)
	}
	want, err := r.Render(Sanitize(doc))
	if err != nil {
		t.Fatal(err)
	}
	if fin != want {
		t.Fatalf("Final != Render: %s", firstDiff(want, fin))
	}
	if other > 0 {
		t.Logf("%d shrinks not at a table tail", other)
	}
}

// TestShortTableAppears: a short table draws its rows while it is the tail.
func TestShortTableAppears(t *testing.T) {
	doc := "Intro.\n\n| k | v |\n| --- | --- |\n| a | 1 |\n| b | 2 |\n"
	r := newTestRenderer(t, "dark", 60)
	s := NewStream(r, "k", NewCache(16))
	out, err := s.Frame(doc)
	if err != nil || s.stats.lastHeld || strings.Contains(xansi.Strip(out), holdText) {
		t.Fatalf("short table tail held: %v %q", err, xansi.Strip(out))
	}
	fin, err := s.Final(doc)
	if err != nil || out != fin {
		t.Fatalf("Frame != Final: %v\n%s", err, firstDiff(fin, out))
	}
}

// TestTableHoldNoPipeRows: body rows written without a leading pipe draw
// while the table streams.
func TestTableHoldNoPipeRows(t *testing.T) {
	src := "| a | b |\n|---|---|\n" + strings.Repeat("x | y\n", 12)
	r := newTestRenderer(t, "notty", 50)
	s := NewStream(r, "k", nil)
	out, err := s.Frame(src)
	if err != nil || s.stats.lastHeld || strings.Contains(xansi.Strip(out), holdText) {
		t.Fatalf("12-row table held: %v %q", err, xansi.Strip(out))
	}
	fin, err := s.Final(src)
	if err != nil || out != fin {
		t.Fatalf("Frame != Final: %v\n%s", err, firstDiff(fin, out))
	}
}

// framesNoShrink streams every byte prefix of doc through Frame and fails
// if a frame has fewer lines than the one before; the end must not shrink
// and Final must equal r.Render(Sanitize(doc)). It returns the frames.
func framesNoShrink(t *testing.T, r *DirectiveRenderer, doc string) []string {
	t.Helper()
	s := NewStream(r, "k", nil)
	var outs []string
	prev := 0
	for i := 1; i <= len(doc); i++ {
		out, err := s.Frame(doc[:i])
		if err != nil {
			t.Fatal(err)
		}
		if n := lineCount(out); n < prev {
			t.Fatalf("prefix %d shrank %d -> %d lines:\n%s\n->\n%s", i, prev, n, xansi.Strip(outs[len(outs)-1]), xansi.Strip(out))
		}
		prev = lineCount(out)
		outs = append(outs, out)
	}
	fin, err := s.Final(doc)
	if err != nil {
		t.Fatal(err)
	}
	want, err := r.Render(Sanitize(doc))
	if err != nil {
		t.Fatal(err)
	}
	if fin != want {
		t.Fatalf("Final != Render: %s", firstDiff(want, fin))
	}
	if n := lineCount(fin); n < prev {
		t.Fatalf("the end shrank %d -> %d lines", prev, n)
	}
	return outs
}

// TestTableHoldHeaderLine: a tail line that starts with `|` is held from its
// first byte to the delimiter row's "\n" (Carl 2026-11-11, option A), so a
// header never draws as a paragraph; then the table draws, header first,
// and the frame never shrinks, also when the header is wider than the
// width (79 cells at w50: cut, one line).
func TestTableHoldHeaderLine(t *testing.T) {
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{50, 78} {
			for _, hdr := range []string{"| a | b |", "| " + strings.Repeat("long header words ", 4) + "| b |"} {
				t.Run(fmt.Sprintf("%s/w%d/%d", style, w, len(hdr)), func(t *testing.T) {
					r := newTestRenderer(t, style, w)
					pre := "Intro.\n\n"
					doc := pre + hdr + "\n|---|---|\n| 1 | 2 |\n"
					delimEnd := len(pre) + len(hdr) + len("\n|---|---|")
					outs := framesNoShrink(t, r, doc)
					for i := len(pre) + 1; i <= len(doc); i++ {
						got := xansi.Strip(outs[i-1])
						held := strings.Contains(got, holdText)
						if held != (i <= delimEnd) {
							t.Fatalf("prefix %d: held %v: %q", i, held, got)
						}
						if !held && !strings.Contains(got, "a") {
							t.Fatalf("prefix %d: no header: %q", i, got)
						}
					}
				})
			}
		}
	}
}

// TestTableHoldNotTable: a `|` line whose next line is not a delimiter row
// is held, then draws as a paragraph; the frame only grows.
func TestTableHoldNotTable(t *testing.T) {
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{50, 78} {
			r := newTestRenderer(t, style, w)
			for _, doc := range []string{
				"Intro.\n\n| not a table, " + strings.Repeat("some words ", 6) + "\nprose after it\n",
				"Intro.\n\n| a | b |\n|  |\n",
				"Intro.\n\n| a | b | c |\n|---|---|\n",
				"| a | b |\n\nNext.\n",
			} {
				outs := framesNoShrink(t, r, doc)
				if got := xansi.Strip(outs[len(outs)-1]); strings.Contains(got, holdText) {
					t.Errorf("%s/w%d: %q ends held: %q", style, w, doc, got)
				}
			}
		}
	}
}

// TestTableHoldScope: a `|` line inside a code fence or a list item is not
// held (the hold reads only a tail block that starts with `|`); a table
// with its delimiter row is not held.
func TestTableHoldScope(t *testing.T) {
	r := newTestRenderer(t, "dark", 60)
	for _, doc := range []string{"```\n| a | b |\n", "- | a | b |\n", "- item\n\n  | a | b |\n", "| a | b |\n|---|---|\n"} {
		s := NewStream(r, "k", nil)
		out, err := s.Frame(doc)
		if err != nil || s.stats.lastHeld || strings.Contains(xansi.Strip(out), holdText) {
			t.Errorf("%q held: %v %q", doc, err, xansi.Strip(out))
		}
	}
}
