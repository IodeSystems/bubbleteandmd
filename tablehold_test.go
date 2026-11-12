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
		{"| a | b |\n", false}, // no delimiter row yet
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

// TestTableHoldHeaderLine: a lone header line draws as a paragraph, then as
// the one-line placeholder once the delimiter row arrives. A header that
// wraps makes that frame shorter than the one before (logged; the hold is
// Carl's design, 2026-11-11).
func TestTableHoldHeaderLine(t *testing.T) {
	for _, hdr := range []string{"| a | b |", "| " + strings.Repeat("long header words ", 4) + "| b |"} {
		r := newTestRenderer(t, "notty", 50)
		s := NewStream(r, "k", nil)
		p, err := s.Frame("Intro.\n\n" + hdr + "\n")
		if err != nil {
			t.Fatal(err)
		}
		h, err := s.Frame("Intro.\n\n" + hdr + "\n|---|---|\n")
		if err != nil || !s.stats.lastHeld {
			t.Fatalf("not held: %v", err)
		}
		t.Logf("header %d cells: paragraph frame %d lines, held frame %d lines", len(hdr), lineCount(p), lineCount(h))
	}
}
