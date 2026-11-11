package join

import "testing"

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
