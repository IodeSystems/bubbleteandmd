package bubbleteandmd

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// TestInlineMathRender: an inline formula (flowdown v0.7.1's reading,
// docs/decisions.md "inline math") renders as the code span of its TeX,
// byte-equal, whole and streamed token by token; prices stay text. notty
// draws code with no mark, so there the span keeps the `$` (notty).
func TestInlineMathRender(t *testing.T) {
	cases := []struct{ in, taught, notty string }{
		{"Sum: $2 + 2 = 4$ and $x_i^2$, for $5 and $10.\n", "Sum: `2 + 2 = 4` and `x_i^2`, for $5 and $10.\n", "Sum: `$2 + 2 = 4$` and `$x_i^2$`, for $5 and $10.\n"},
		{"> [!NOTE]\n> pay $5 now and 7$ later, or $n^2$.\n", "> [!NOTE]\n> pay $5 now and 7$ later, or `n^2`.\n", "> [!NOTE]\n> pay $5 now and 7$ later, or `$n^2$`.\n"},
		{"> [!TABS]\n#### $a$\nOne $b = c$.\n", "> [!TABS]\n#### `a`\nOne `b = c`.\n", "> [!TABS]\n#### `$a$`\nOne `$b = c$`.\n"},
		{"| a | b |\n|---|---|\n| $x^2$ | $5 |\n", "| a | b |\n|---|---|\n| `x^2` | $5 |\n", "| a | b |\n|---|---|\n| `$x^2$` | $5 |\n"},
		{"Then $a_1 + b_1$ and $\\{x\\}$.\n", "Then `a_1 + b_1` and `\\{x\\}`.\n", "Then `$a_1 + b_1$` and `$\\{x\\}$`.\n"},
	}
	var run, passed atomic.Int64
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{40, 78} {
			r := newTestRenderer(t, style, w)
			for _, c := range cases {
				got, err := r.Render(c.in)
				if err != nil {
					t.Fatal(err)
				}
				taught := c.taught
				if style == "notty" {
					taught = c.notty
				}
				want, err := r.Render(taught)
				if err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Errorf("%s/w%d/%q: %s", style, w, c.in, firstDiff(want, got))
				}
				checkStream(t, r, c.in, corpus.TokenPrefixes(c.in), &run, &passed)
			}
		}
	}
	if run.Load() != passed.Load() {
		t.Errorf("%d of %d prefix checks passed", passed.Load(), run.Load())
	}
}

// TestInlineMathNotty: in notty a formula shows with its `$` and its TeX as
// written (`_` reads as no emphasis, `\{` keeps its backslash); a form after
// formulas keeps its key, the offset as written.
func TestInlineMathNotty(t *testing.T) {
	r := newTestRenderer(t, "notty", 78)
	got, err := r.Render("Then $a_1 + b_1$ and $\\{x\\}$, for $5 and $10.\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Then $a_1 + b_1$ and $\\{x\\}$, for $5 and $10."; !strings.Contains(got, want) {
		t.Errorf("got %q, want it to hold %q", got, want)
	}
	src := "Set $a_1$ and $b_1$.\n\n> [!TABS]\n#### $x$\nOne.\n#### $y_2$\nTwo.\n"
	_, marks, err := r.RenderState(src, 0, &State{})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Index(src, "> [!TABS]")
	if len(marks) == 0 || marks[0].Key != strconv.Itoa(want) {
		t.Errorf("marks = %+v, want key %d", marks, want)
	}
}
