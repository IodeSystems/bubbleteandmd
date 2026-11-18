package bubbleteandmd

import (
	"sync/atomic"
	"testing"

	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// TestInlineMathRender: an inline formula (flowdown v0.7.1's reading,
// docs/decisions.md "inline math") renders as the code span of its TeX,
// byte-equal, whole and streamed token by token; prices stay text.
func TestInlineMathRender(t *testing.T) {
	cases := []struct{ in, taught string }{
		{"Sum: $2 + 2 = 4$ and $x_i^2$, for $5 and $10.\n", "Sum: `2 + 2 = 4` and `x_i^2`, for $5 and $10.\n"},
		{"> [!NOTE]\n> pay $5 now and 7$ later, or $n^2$.\n", "> [!NOTE]\n> pay $5 now and 7$ later, or `n^2`.\n"},
		{"> [!TABS]\n#### $a$\nOne $b = c$.\n", "> [!TABS]\n#### `a`\nOne `b = c`.\n"},
		{"| a | b |\n|---|---|\n| $x^2$ | $5 |\n", "| a | b |\n|---|---|\n| `x^2` | $5 |\n"},
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
				want, err := r.Render(c.taught)
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
