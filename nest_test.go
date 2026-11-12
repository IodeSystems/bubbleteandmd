package bubbleteandmd

import (
	"fmt"
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
)

// nestBudget: one adversarial render, on a loaded box. Before the caps a
// line of 100 `>` took over 120 s at width 80, and 40 000 nested emphases
// killed the process with a stack overflow (docs/decisions.md, "Nesting
// caps"); with them the slowest case here takes about 7 s.
const nestBudget = 120 * time.Second

// nestInputs: deep nesting n levels deep, of each kind the caps cover.
var nestInputs = []struct {
	name string
	gen  func(n int) string
}{
	{"quote", func(n int) string { return strings.Repeat(">", n) + " x\n" }},
	{"list", func(n int) string { return strings.Repeat("- ", n) + "x\n" }},
	{"ordered", func(n int) string { return strings.Repeat("1. ", n) + "x\n" }},
	{"quoted-list", func(n int) string { return strings.Repeat("> - ", n/2) + "x\n" }},
	{"emphasis", func(n int) string { return strings.Repeat("*a _b ", n) + "x" + strings.Repeat("_ *", n) + "\n" }},
	{"strong", func(n int) string { return strings.Repeat("*", n) + "x" + strings.Repeat("*", n) + "\n" }},
	{"links", func(n int) string { return strings.Repeat("[", n) + "x" + strings.Repeat("](u)", n) + "\n" }},
	{"images", func(n int) string { return strings.Repeat("![", n) + "x" + strings.Repeat("](u)", n) + "\n" }},
	{"parens", func(n int) string { return "[x](" + strings.Repeat("(", n) + "u" + strings.Repeat(")", n) + ")\n" }},
}

// within fails t when f takes longer than nestBudget.
func within(t *testing.T, what string, f func() error) {
	t.Helper()
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- f() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		t.Logf("%s: %v", what, time.Since(start).Round(time.Millisecond))
	case <-time.After(nestBudget):
		t.Fatalf("%s: over %v", what, nestBudget)
	}
}

func TestNestingRender(t *testing.T) {
	r := newTestRenderer(t, "dark", 80)
	for _, in := range nestInputs {
		for _, n := range []int{100, 1000, 10000} {
			src := in.gen(n)
			within(t, fmt.Sprintf("%s/%d", in.name, n), func() error {
				_, err := r.Render(src)
				return err
			})
		}
		// a 100 KB reply of it
		unit := len(in.gen(2)) - len(in.gen(1))
		src := in.gen(100000 / unit)
		within(t, in.name+"/100KB", func() error {
			_, err := r.Render(src)
			return err
		})
	}
	// past the 40 000 levels that overflowed the stack
	src := nestInputs[4].gen(50000)
	within(t, "emphasis/50000", func() error {
		_, err := r.Render(src)
		return err
	})
}

// Past the cap a line is text: the markers show.
func TestNestingPastCapIsText(t *testing.T) {
	r := newTestRenderer(t, "dark", 80)
	for _, c := range []struct{ src, shown string }{
		{strings.Repeat(">", 100) + " x\n", strings.Repeat(">", 100-16) + " x"},
		{strings.Repeat("- ", 100) + "x\n", strings.Repeat("- ", 100-16) + "x"},
	} {
		out, err := r.Render(c.src)
		if err != nil {
			t.Fatal(err)
		}
		var text strings.Builder
		for _, l := range strings.Split(xansi.Strip(out), "\n") {
			// the content after the 16 indents, wrapped over lines
			text.WriteString(strings.TrimSpace(strings.TrimLeft(l, " │•")))
		}
		if !strings.Contains(strings.ReplaceAll(text.String(), " ", ""), strings.ReplaceAll(c.shown, " ", "")) {
			t.Errorf("%q: %q not shown in\n%s", c.src[:10], c.shown, xansi.Strip(out))
		}
	}
}

// Streamed token by token (a Frame per 64 bytes, then Final), as a reply
// arrives.
func TestNestingStream(t *testing.T) {
	r := newTestRenderer(t, "dark", 80)
	for _, in := range nestInputs {
		src := in.gen(1000)
		within(t, in.name+"/1000 streamed", func() error {
			s := NewStream(r, "k", nil)
			for i := 64; i < len(src); i += 64 {
				if _, err := s.Frame(src[:i]); err != nil {
					return err
				}
			}
			_, err := s.Final(src)
			return err
		})
	}
}
