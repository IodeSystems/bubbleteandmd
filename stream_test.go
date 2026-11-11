package bubbleteandmd

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf16"
	"unsafe"

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
		if !isStableSource(Sanitize(cur), source) {
			t.Errorf("prefix len %d: cache stores %q, not a stable block of the sanitized prefix", len(cur), source)
		}
	}
	s := NewStream(r, "k", c)
	s.seen = func(src string) {
		if src != Sanitize(cur) {
			t.Errorf("prefix len %d: splitter sees text that is not the sanitized prefix", len(cur))
		}
	}
	for _, raw := range prefixes {
		cur = raw
		if !strings.HasPrefix(text, raw) {
			t.Fatalf("prefix len %d is not a prefix", len(raw))
		}
		// Final(raw) == r.Render(Sanitize(raw));
		// Frame(raw) == r.Render(heal.Close(Sanitize(raw))).
		p := Sanitize(raw)
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
			got, err := k.f(raw)
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

// TestCacheKeyOwnsSource: a cached key must not point into the Frame text.
// A substring would keep the whole text of that frame alive while the key
// stays in the cache: about blocks x length / 2 bytes over a reply.
func TestCacheKeyOwnsSource(t *testing.T) {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(40))
	if err != nil {
		t.Fatal(err)
	}
	c := NewCache(100)
	var src string
	c.put = func(source string) {
		lo := uintptr(unsafe.Pointer(unsafe.StringData(src)))
		p := uintptr(unsafe.Pointer(unsafe.StringData(source)))
		if p >= lo && p < lo+uintptr(len(src)) {
			t.Errorf("cache key %q points into the frame text (len %d)", source, len(src))
		}
	}
	s := NewStream(r, "k", c)
	text := ""
	for i := range 6 {
		text += fmt.Sprintf("para %d\n\n", i)
		src = strings.Clone(text)
		if _, err := s.Frame(src); err != nil {
			t.Fatal(err)
		}
	}
}

// trimmed wraps a Renderer and trims the newlines glamour writes at both
// ends: what a caller does to print the output with no blank lines.
type trimmed struct{ r Renderer }

func (t trimmed) Render(s string) (string, error) {
	o, err := t.r.Render(s)
	return strings.Trim(o, "\n"), err
}

// TestTrimmedRenderer: a Renderer whose pieces cannot be joined is an
// error, not wrong output.
func TestTrimmedRenderer(t *testing.T) {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(40))
	if err != nil {
		t.Fatal(err)
	}
	src := "```\nx\n```\n# head\n\npara one\n\npara two\n\nthree\n"
	s := NewStream(trimmed{r}, "k", nil)
	for _, f := range []func(string) (string, error){s.Frame, s.Final} {
		if out, err := f(src); !errors.Is(err, ErrTrimmed) {
			t.Errorf("err = %v, want ErrTrimmed; out %q", err, out)
		}
	}
}

// TestTabFenceFrame: "\t```" is indented code (CommonMark, blocks.Split).
// heal.Close does not close it as a fence (internal/heal TestTabBeforeFence);
// the invariant holds at every token prefix of a reply that has one.
func TestTabFenceFrame(t *testing.T) {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(40))
	if err != nil {
		t.Fatal(err)
	}
	text := "para\n\n\t```\n\tcode\n\nafter **bold** text\n\n- item\n\n\t```\n\tin item\n\nend\n"
	var run, passed atomic.Int64
	checkStream(t, r, text, corpus.TokenPrefixes(text), &run, &passed)
}
