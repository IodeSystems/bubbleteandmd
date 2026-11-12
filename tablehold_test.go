package bubbleteandmd

import (
	"fmt"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
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
		{"| a | b |\n|  |\n", false}, // whole second line, not a delimiter row
		{"| a | b |\nx", false},
		{"| not a", true},
		{hdr, true},
		{hdr + strings.Repeat(row, holdRows-1), true},
		{hdr + strings.Repeat(row, holdRows), false},
		{hdr + strings.Repeat("x | y\n", holdRows-1), true},
		{hdr + strings.Repeat("x | y\n", holdRows), false},        // no leading pipe: still body rows
		{hdr + "x | y\n\n" + strings.Repeat(row, holdRows), true}, // rows after a blank line are not the table's
		{"| not a table\nprose\n", false},
		{"prose\n", false},
	} {
		if got := heldTable(c.src); got != c.want {
			t.Errorf("heldTable(%q) = %v, want %v", c.src, got, c.want)
		}
	}
}

func lineCount(s string) int { return strings.Count(s, "\n") }

// tableWidth: the widest line of out in cells.
func tableWidth(out string) int {
	w := 0
	for _, l := range strings.Split(out, "\n") {
		w = max(w, xansi.StringWidth(l))
	}
	return w
}

// rule: the table's header rule line (box drawing, no letters or digits).
func rule(s string) string {
	for _, l := range strings.Split(xansi.Strip(s), "\n") {
		if strings.ContainsAny(l, "─-") && strings.ContainsAny(l, "┼|+") && !strings.ContainsAny(l, "abcdefghijklmnopqrstuvwxyz0123456789") {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

// TestTableHold: table-20k at frame prefixes. A held tail is one placeholder
// line; once the table shows, the frame never shrinks; its widths at first
// appearance have the final total width (column rules are logged: column 1
// grows by 2 cells after row 8 in table-20k); Final never holds.
func TestTableHold(t *testing.T) {
	text := scenario(t, "table-20k")
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{78, 98} {
			t.Run(fmt.Sprintf("%s/w%d", style, w), func(t *testing.T) {
				r := newTestRenderer(t, style, w)
				s := NewStream(r, "k", NewCache(256))
				frames := corpus.FramePrefixes(text)
				prev := 0
				shown := false
				var first string
				for i, p := range frames {
					out, err := s.Frame(p)
					if err != nil {
						t.Fatal(err)
					}
					if s.stats.lastHeld {
						if shown {
							t.Fatalf("frame %d held after the table showed", i)
						}
						if !strings.Contains(xansi.Strip(out), holdText) {
							t.Fatalf("frame %d: held frame has no placeholder", i)
						}
						continue
					}
					if !shown && strings.Contains(p, "---") {
						shown, first = true, out
					}
					if n := lineCount(out); shown && n < prev {
						t.Fatalf("frame %d shrank: %d -> %d lines", i, prev, n)
					}
					prev = lineCount(out)
				}
				if s.stats.held == 0 || !shown {
					t.Fatalf("held %d frames, shown %v", s.stats.held, shown)
				}
				final, err := s.Final(text)
				if err != nil {
					t.Fatal(err)
				}
				if s.stats.lastHeld {
					t.Fatal("Final held")
				}
				fw, lw := tableWidth(first), tableWidth(final)
				t.Logf("held %d frames; width at first appearance %d, final %d; rule equal: %v", s.stats.held, fw, lw, rule(first) == rule(final))
				if fw != lw {
					t.Errorf("width at first appearance %d != final %d", fw, lw)
				}
			})
		}
	}
}

// TestShortTableAppears: a table with fewer rows than holdRows is a
// placeholder while it is the tail and appears when the next block starts
// or at Final.
func TestShortTableAppears(t *testing.T) {
	doc := "Intro.\n\n| k | v |\n| --- | --- |\n| a | 1 |\n| b | 2 |\n"
	r := newTestRenderer(t, "dark", 60)
	s := NewStream(r, "k", NewCache(16))
	out, err := s.Frame(doc)
	if err != nil || !s.stats.lastHeld || !strings.Contains(xansi.Strip(out), holdText) {
		t.Fatalf("short table tail not held: %v %q", err, xansi.Strip(out))
	}
	out, err = s.Frame(doc + "\nAfter.\n")
	if err != nil || s.stats.lastHeld || strings.Contains(xansi.Strip(out), holdText) {
		t.Fatalf("table held after a later block started: %v %q", err, xansi.Strip(out))
	}
	out, err = s.Final(doc)
	if err != nil || s.stats.lastHeld || strings.Contains(xansi.Strip(out), holdText) || !strings.Contains(xansi.Strip(out), "a") {
		t.Fatalf("Final: %v %q", err, xansi.Strip(out))
	}
}

// TestTableHoldNoPipeRows: body rows written without a leading pipe count,
// so a long table shows while it streams, not only at Final (audit
// 2026-11-11).
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
// first byte (Carl 2026-11-11, option A), so a header never draws as a
// paragraph and the frame never shrinks, also when the header wraps
// (79 cells at w50 was 6 -> 5 lines before).
func TestTableHoldHeaderLine(t *testing.T) {
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{50, 78} {
			for _, hdr := range []string{"| a | b |", "| " + strings.Repeat("long header words ", 4) + "| b |"} {
				t.Run(fmt.Sprintf("%s/w%d/%d", style, w, len(hdr)), func(t *testing.T) {
					r := newTestRenderer(t, style, w)
					pre := "Intro.\n\n"
					doc := pre + hdr + "\n|---|---|\n| 1 | 2 |\n"
					outs := framesNoShrink(t, r, doc)
					for i := len(pre) + 1; i <= len(doc); i++ {
						if got := xansi.Strip(outs[i-1]); !strings.Contains(got, holdText) || strings.Contains(got, "header") {
							t.Fatalf("prefix %d not held: %q", i, got)
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
// is released at holdRows body rows.
func TestTableHoldScope(t *testing.T) {
	r := newTestRenderer(t, "dark", 60)
	for _, doc := range []string{"```\n| a | b |\n", "- | a | b |\n", "- item\n\n  | a | b |\n"} {
		s := NewStream(r, "k", nil)
		out, err := s.Frame(doc)
		if err != nil || s.stats.lastHeld || strings.Contains(xansi.Strip(out), holdText) {
			t.Errorf("%q held: %v %q", doc, err, xansi.Strip(out))
		}
	}
	hdr := "| a | b |\n|---|---|\n"
	s := NewStream(r, "k", nil)
	if _, err := s.Frame(hdr + strings.Repeat("| 1 | 2 |\n", holdRows-1)); err != nil || !s.stats.lastHeld {
		t.Fatalf("%d rows not held: %v", holdRows-1, err)
	}
	if out, err := s.Frame(hdr + strings.Repeat("| 1 | 2 |\n", holdRows)); err != nil || s.stats.lastHeld || strings.Contains(xansi.Strip(out), holdText) {
		t.Fatalf("%d rows held: %v", holdRows, err)
	}
}
