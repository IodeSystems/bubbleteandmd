package join

import (
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestAffixes(t *testing.T) {
	p, s, err := Affixes("\n\n")
	if err != nil || p != "\n" || s != "\n" {
		t.Fatalf("got %q %q %v", p, s, err)
	}
	if _, _, err := Affixes("\n\n\n"); err != ErrAffixes {
		t.Fatalf("odd render: err %v", err)
	}
	if _, _, err := Affixes("ab"); err != ErrAffixes {
		t.Fatalf("unequal halves: err %v", err)
	}
}

func TestInput(t *testing.T) {
	if Input("a\n", true) != "a\n" || Input("a\n", false) != Sentinel+"a\n" {
		t.Fatal("Input")
	}
}

func TestStrip(t *testing.T) {
	if got, err := Strip("\nbody\n", "\n", "\n"); err != nil || got != "body" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := Strip("body\n", "\n", "\n"); err != ErrPiece {
		t.Fatalf("no prefix: err %v", err)
	}
	if _, err := Strip("\n", "\n", "\n"); err != ErrPiece {
		t.Fatalf("overlap: err %v", err)
	}
}

const (
	cell  = "\x1b[38;5;234m \x1b[m"
	empty = "\x1b[38;5;234m\x1b[m"
)

func TestDropCells(t *testing.T) {
	for _, c := range []struct {
		name, in string
		n        int
		want     string
	}{
		{"plain", "abcd", 2, "cd"},
		{"zero", "abcd", 0, "abcd"},
		{"all", "ab", 5, ""},
		{"styled cells take their reset", cell + cell + "x", 1, cell + "x"},
		{"opening before text", "\x1b[1mab\x1b[m", 1, "b\x1b[m"},
		// glamour writes an empty styled run where an element starts; the
		// whole render keeps it after the previous element's cells.
		{"empty run kept", "  " + empty + cell + cell + cell, 4, empty + cell},
		{"escape after nth cell kept", "ab\x1b[1mc", 2, "\x1b[1mc"},
		// OSC 8 (ended by BEL, as glamour writes it, or by ST) is no cell
		// and is kept: the kept cells stay inside their link.
		{"osc 8 BEL is no cell", "\x1b]8;;http://x\aab\x1b]8;;\acd", 2, "\x1b]8;;http://x\a\x1b]8;;\acd"},
		{"osc 8 ST is no cell", "\x1b]8;;http://x\x1b\\ab\x1b]8;;\x1b\\cd", 1, "\x1b]8;;http://x\x1b\\b\x1b]8;;\x1b\\cd"},
		{"non-SGR CSI is no cell", "\x1b[Kab\x1b[31mc", 1, "\x1b[Kb\x1b[31mc"},
		{"incomplete CSI", "a\x1b[3", 2, ""},
		{"incomplete OSC", "a\x1b]8;;x", 3, ""},
		{"lone ESC", "a\x1b", 2, ""},
	} {
		if got := DropCells(c.in, c.n); got != c.want {
			t.Errorf("%s: DropCells(%q, %d) = %q, want %q", c.name, c.in, c.n, got, c.want)
		}
	}
}

func TestJoin(t *testing.T) {
	for _, c := range []struct {
		name   string
		pieces []string
		want   string
	}{
		{"none", nil, "PS"},
		{"newline ends", []string{"a\n", "b\n"}, "Pa\nb\nS"},
		{"empty skipped", []string{"a\n", "", "b"}, "Pa\nbS"},
		{"open line drops cells", []string{"x\n  ab", "    cd\nef"}, "Px\n  abcd\nefS"},
		{"open line, one-line piece", []string{"ab", "  cd"}, "PabcdS"},
		{"empty piece does not reset prev", []string{"ab", "", "  cd"}, "PabcdS"},
	} {
		if got := Join("P", "S", c.pieces); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestDropCellsGlamourLinks: on glamour's own lines with links (OSC 8 ended
// by BEL), dropping k cells leaves the text after the kth cell.
func TestDropCellsGlamourLinks(t *testing.T) {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(40))
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render("[x](http://e.com) see [a long label](http://e.com/y) <https://e.com/z>\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\x1b]8;") {
		t.Fatalf("no OSC 8 in glamour output %q", out)
	}
	for _, line := range strings.Split(out, "\n") {
		text := []rune(ansi.Strip(line))
		for k := 0; k <= len(text); k++ {
			got := DropCells(line, k)
			if s := ansi.Strip(got); s != string(text[k:]) {
				t.Errorf("DropCells(%q, %d) shows %q, want %q", line, k, s, string(text[k:]))
			}
		}
	}
}
