package bubbleteandmd

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf16"
	"unsafe"

	"charm.land/glamour/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/codestream"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
	"github.com/iodesystems/bubbleteandmd/internal/join"
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

// heldChecks counts Frame checks of a held table tail.
var heldChecks atomic.Int64

func untie(s string) string { return chromaTies.Replace(s) }

type invCase struct{ name, text string }

// fenceChecks counts Frame checks of a tail spliced from code chunks
// (relaxed: Carl B1), and how many of those differ from the whole render.
var fenceChecks, fenceDiffers atomic.Int64

var sgrRe = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// sgrOnly reports whether got and want differ in SGR sequences only: the
// same lines, the same visible text per line, and the same bytes once SGR
// sequences are removed.
func sgrOnly(want, got string) bool {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	if len(w) != len(g) {
		return false
	}
	for i := range w {
		if xansi.Strip(w[i]) != xansi.Strip(g[i]) || sgrRe.ReplaceAllString(w[i], "") != sgrRe.ReplaceAllString(g[i], "") {
			return false
		}
	}
	return true
}

// fenceTail reports whether the last block of src opens with a code fence
// of more than codestream.ChunkLines lines.
func fenceTail(src string) bool {
	bs := blocks.Split(src)
	if len(bs) == 0 {
		return false
	}
	b := bs[len(bs)-1].Source
	return fenceOpenRe.MatchString(b[:max(0, strings.IndexByte(b, '\n'))]) && strings.Count(b, "\n") > codestream.ChunkLines
}

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
	f0, d0 := fenceChecks.Load(), fenceDiffers.Load()
	t.Cleanup(func() {
		t.Logf("invariant: %d prefix checks run, %d passed; %d Frame checks of a growing fence (SGR-only), %d of them differ", run.Load(), passed.Load(), fenceChecks.Load()-f0, fenceDiffers.Load()-d0)
	})
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

// checkStream streams prefixes through one Stream and returns it.
func checkStream(t *testing.T, r Renderer, text string, prefixes []string, run, passed *atomic.Int64) *Stream {
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
			if s.stats.lastFence {
				// Carl B1: a Frame of a growing fence differs in SGR bytes only.
				if k.name != "Frame" || !fenceTail(healed) {
					t.Fatalf("prefix len %d: %s spliced from code chunks, tail is not a growing fence: %q", len(p), k.name, tailOf(p))
				}
				fenceChecks.Add(1)
				if untie(got) != untie(k.want) {
					fenceDiffers.Add(1)
					if !sgrOnly(untie(k.want), untie(got)) {
						t.Fatalf("prefix len %d: Frame of a growing fence differs in more than SGR: %s\nprefix: %q", len(p), firstDiff(untie(k.want), untie(got)), tailOf(p))
					}
				}
				passed.Add(1)
				continue
			}
			bs := blocks.Split(p)
			isHeld := k.name == "Frame" && len(bs) > 0 && func() bool {
				at := bs[len(bs)-1].Offset
				return heldTable(heal.Close(p[at:])) && heal.Close(p[at:]) == healed[at:]
			}()
			if s.stats.lastHeld != isHeld {
				t.Fatalf("prefix len %d: %s held=%v, want %v: %q", len(p), k.name, s.stats.lastHeld, isHeld, tailOf(p))
			}
			if isHeld {
				// A held table: the whole render with the tail block replaced
				// by the placeholder piece.
				heldChecks.Add(1)
				at := bs[len(bs)-1].Offset
				var pieces []string
				if at > 0 {
					head, err := r.Render(healed[:at])
					if err != nil {
						t.Fatal(err)
					}
					if head, err = join.Strip(head, s.p, s.s); err != nil {
						t.Fatal(err)
					}
					pieces = append(pieces, head)
				}
				ph, err := s.holdPiece(at == 0)
				if err != nil {
					t.Fatal(err)
				}
				want := join.Join(s.p, s.s, append(pieces, ph))
				if got != want && untie(got) != untie(want) {
					t.Fatalf("prefix len %d: held Frame != whole render with placeholder: %s\nprefix: %q", len(p), firstDiff(untie(want), untie(got)), tailOf(p))
				}
				passed.Add(1)
				continue
			}
			if got != k.want && untie(got) != untie(k.want) {
				t.Fatalf("prefix len %d: %s != whole render: %s\nprefix: %q", len(p), k.name, firstDiff(untie(k.want), untie(got)), tailOf(p))
			}
			passed.Add(1)
		}
	}
	return s
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

// TestCRLFFrame: Stream turns "\r\n" into "\n" before heal, so a CRLF reply
// heals as the LF one does (heal itself, like the TS, rejects a fence line
// that ends in "\r").
func TestCRLFFrame(t *testing.T) {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(40))
	if err != nil {
		t.Fatal(err)
	}
	s := NewStream(r, "k", nil)
	for _, src := range []string{"```\r\na*b\r\n", "x*y\r\n\r\nfoo", "# T\r\n\r\n- a\r\n- **b\r\n"} {
		crlf, err := s.Frame(src)
		if err != nil {
			t.Fatal(err)
		}
		lf, _ := s.Frame(strings.ReplaceAll(src, "\r\n", "\n"))
		if crlf != lf {
			t.Errorf("%q: CRLF frame differs from LF: %s", src, firstDiff(lf, crlf))
		}
	}
}

// TestReopenedBlockNotStale: the splitter can still change the block before
// the last (a list item continued, a `[!CONS]` joining a `[!PROS]`). Stream
// memoizes that block by its source, so a changed block is a miss: every
// frame equals the whole render, and the texts below do reopen it.
func TestReopenedBlockNotStale(t *testing.T) {
	texts := []string{
		"intro\n\n> [!PROS]\n> - fast\n> - small\n\n> [!CONS]\n> - new\n\nend para\n",
		"intro\n\n- a\n\n  b continues a\n\n- c\n\nafter\n",
		"intro\n\n1. one\n\n2. two\n\n   more two\n\ntail\n",
		"# T\n\npara\n\n```go\nx := 1\n\ny := 2\n```\n\nend\n",
		"a\n\n> quote\n\n> more quote\n\nb\n",
	}
	reopened := 0
	for _, text := range texts {
		for _, style := range []string{"dark", "notty"} {
			r := newTestRenderer(t, style, 40)
			var run, passed atomic.Int64
			checkStream(t, r, text, corpus.TokenPrefixes(text), &run, &passed)
			// Count the frames where the block before the last changed in
			// place: same offset, other source, as the previous frame saw it.
			reopened += reopens(text)
		}
	}
	if reopened == 0 {
		t.Fatal("no text reopened the block before the last; the test proves nothing")
	}
	t.Logf("%d frames reopened the block before the last", reopened)
}

// reopens counts the token prefixes of text where the block that was before
// the last in the previous prefix has another source now (or is merged into
// the block above it).
func reopens(text string) int {
	n := 0
	var prev blocks.Block
	for _, raw := range corpus.TokenPrefixes(text) {
		bs := blocks.Split(Sanitize(raw))
		if prev.Source != "" {
			same := false
			for _, b := range bs {
				if b.Offset == prev.Offset && b.Source == prev.Source {
					same = true
				}
			}
			if !same {
				n++
			}
		}
		prev = blocks.Block{}
		if len(bs) >= 2 {
			prev = bs[len(bs)-2]
		}
	}
	return n
}

// counting counts Render calls per input.
type counting struct {
	r Renderer
	n map[string]int
}

func (c counting) Render(s string) (string, error) {
	c.n[s]++
	return c.r.Render(s)
}

// TestNearBlockRendersOnce: while the last block grows, the block before it
// renders once, and once more never when it becomes stable.
func TestNearBlockRendersOnce(t *testing.T) {
	g, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(40))
	if err != nil {
		t.Fatal(err)
	}
	r := counting{g, map[string]int{}}
	s := NewStream(r, "k", NewCache(16))
	head := "# Title\n\npara **two**\n\n"
	text := head + "third paragraph grows here, word by word.\n\nfourth\n"
	for _, p := range corpus.TokenPrefixes(text[len(head):]) {
		if _, err := s.Frame(head + p); err != nil {
			t.Fatal(err)
		}
	}
	if k := r.n[join.Sentinel+"para **two**\n\n"]; k != 1 {
		t.Errorf("block before the last rendered %d times, want 1", k)
	}
	for in, k := range r.n {
		if strings.Contains(in, "two") && strings.Contains(in, "third") {
			t.Errorf("rendered %d times with the last block: %q", k, in)
		}
	}
}
