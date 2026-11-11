package bubbleteandmd

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf16"

	"charm.land/glamour/v2"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
)

// scenarioCut is how much of each scenario is streamed: llx's
// stream-split test takes the first 12k characters (UTF-16 units).
const scenarioCut = 12_000

// fuzzCount is how many corpus.Fuzz documents join the invariant.
const fuzzCount = 300

// chromaTies maps the 256-colour codes chroma picks between at random for
// one style colour: chroma v2.14.0 formatters/tty_indexed.go findClosest
// ranges over a map, so a colour equally far from two palette entries gets
// either, per code block, per render. glamour's own whole render is not
// deterministic there (dark Comment #676767 -> 241|242, light Operator
// #FF2626 -> 9|196). glamour v2.0.1 ansi/codeblock.go registers its chroma
// theme once per process, from whichever style renders a code block first,
// so the map does not depend on the style under test. Both sides are mapped
// before comparing; no other byte is changed.
var chromaTies = strings.NewReplacer(
	"\x1b[38;5;242m", "\x1b[38;5;241m",
	"\x1b[38;5;196m", "\x1b[38;5;9m",
)

func untie(s string) string { return chromaTies.Replace(s) }

type invCase struct{ name, text string }

func invCases() []invCase {
	var cs []invCase
	for _, d := range corpus.Docs() {
		cs = append(cs, invCase{"doc-" + d.Name, d.Text})
	}
	for _, sc := range corpus.Scenarios() {
		cs = append(cs, invCase{"scenario-" + sc.Name, cutUnits(sc.Text, scenarioCut)})
	}
	for i, d := range corpus.Fuzz(1, fuzzCount) {
		cs = append(cs, invCase{fmt.Sprintf("fuzz-%d", i+1), d})
	}
	return cs
}

// cutUnits returns the first n UTF-16 units of s (as the TS slices),
// moved back off a surrogate pair.
func cutUnits(s string, n int) string {
	u := utf16.Encode([]rune(s))
	if len(u) <= n {
		return s
	}
	if utf16.IsSurrogate(rune(u[n])) && u[n] >= 0xdc00 {
		n--
	}
	return string(utf16.Decode(u[:n]))
}

type pattern struct {
	name     string
	prefixes func(string) []string
}

func patterns(short bool) []pattern {
	ps := []pattern{{"frame", corpus.FramePrefixes}, {"line", corpus.LinePrefixes}}
	if !short {
		ps = append(ps, pattern{"token", corpus.TokenPrefixes})
	}
	return ps
}

// firstDiff names the first byte where got differs from want, with
// quoted context.
func firstDiff(want, got string) string {
	i := 0
	for i < len(want) && i < len(got) && want[i] == got[i] {
		i++
	}
	lo := max(i-24, 0)
	ctx := func(s string) string { return s[min(lo, len(s)):min(i+40, len(s))] }
	return fmt.Sprintf("first diff at byte %d (len want %d, got %d)\n want: %q\n  got: %q", i, len(want), len(got), ctx(want), ctx(got))
}

// isStableSource reports whether source is a block of Split(src) before
// the last two.
func isStableSource(src, source string) bool {
	bs := blocks.Split(src)
	for _, b := range bs[:max(0, len(bs)-2)] {
		if b.Source == source {
			return true
		}
	}
	return false
}

// tokenScenarioCut: token prefixes are ~4x frame prefixes and every check
// renders the whole prefix, O(n^2); scenarios stream only this much in the
// token run. Docs and fuzz docs stream whole.
const tokenScenarioCut = 4_000

func TestInvariant(t *testing.T) {
	cases := invCases()
	ps := patterns(testing.Short())
	var run, passed atomic.Int64
	t.Cleanup(func() { t.Logf("invariant: %d prefix checks run, %d passed", run.Load(), passed.Load()) })
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 78, 98} {
			for _, c := range cases {
				for _, pat := range ps {
					t.Run(fmt.Sprintf("%s/w%d/%s/%s", style, w, c.name, pat.name), func(t *testing.T) {
						t.Parallel()
						text := c.text
						if pat.name == "token" && strings.HasPrefix(c.name, "scenario-") {
							text = cutUnits(text, tokenScenarioCut)
						}
						// A TermRenderer is not safe for concurrent use: one per subtest.
						r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(w))
						if err != nil {
							t.Fatal(err)
						}
						checkStream(t, r, text, pat.prefixes(text), &run, &passed)
					})
				}
			}
		}
	}
}

func checkStream(t *testing.T, r Renderer, text string, prefixes []string, run, passed *atomic.Int64) {
	t.Helper()
	cur := ""
	c := NewCache(256)
	c.put = func(source string) {
		if !isStableSource(cur, source) {
			t.Errorf("prefix len %d: cache stores %q, not a stable block of the raw prefix", len(cur), source)
		}
	}
	s := NewStream(r, "k", c)
	s.seen = func(src string) {
		if src != cur {
			t.Errorf("prefix len %d: splitter sees text that is not the raw prefix", len(cur))
		}
	}
	for _, p := range prefixes {
		cur = p
		if !strings.HasPrefix(text, p) {
			t.Fatalf("prefix len %d is not a prefix", len(p))
		}
		final, err := r.Render(p)
		if err != nil {
			t.Fatal(err)
		}
		healed := heal.Close(p)
		// Frame heals the tail alone; that must equal the tail of healing
		// the whole prefix, with the stable blocks untouched.
		if bs := blocks.Split(p); len(bs) > 2 {
			at := bs[len(bs)-2].Offset
			if healed != p[:at]+heal.Close(p[at:]) {
				t.Fatalf("prefix len %d: heal.Close(p) != p[:tail] + heal.Close(tail): %s", len(p), firstDiff(healed, p[:at]+heal.Close(p[at:])))
			}
		}
		frame := final
		if healed != p {
			if frame, err = r.Render(healed); err != nil {
				t.Fatal(err)
			}
		}
		for _, k := range []struct {
			name string
			f    func(string) (string, error)
			want string
		}{{"Frame", s.Frame, frame}, {"Final", s.Final, final}} {
			run.Add(1)
			got, err := k.f(p)
			if err != nil {
				t.Fatalf("prefix len %d: %s: %v", len(p), k.name, err)
			}
			if got != k.want && untie(got) != untie(k.want) {
				t.Fatalf("prefix len %d: %s != whole render: %s\nprefix: %q", len(p), k.name, firstDiff(untie(k.want), untie(got)), tailOf(p))
			}
			passed.Add(1)
		}
	}
}

func tailOf(s string) string {
	if len(s) > 200 {
		return "…" + s[len(s)-200:]
	}
	return s
}
