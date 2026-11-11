package tablestream

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
	"github.com/iodesystems/bubbleteandmd/internal/join"
)

func newGlamour(t testing.TB, style string, w int) *glamour.TermRenderer {
	t.Helper()
	g, err := glamour.NewTermRenderer(glamour.WithStyles(*styles.DefaultStyles[style]), glamour.WithWordWrap(w))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func newTable(c config) *Table {
	return New(ansi.Options{Styles: *styles.DefaultStyles[c.style], WordWrap: c.width})
}

func table20k(t testing.TB) string {
	for _, s := range corpus.Scenarios() {
		if s.Name == "table-20k" {
			return s.Text
		}
	}
	t.Fatal("no table-20k")
	return ""
}

// hand tables: links, inline code, emphasis, wide runes, alignment.
var handTables = map[string]string{
	"align": "| Left | Center | Right | None |\n| :--- | :----: | ----: | ---- |\n" +
		"| a | b | c | d |\n| longer left cell here | mid | 12345 | plain words that run on and on past the width |\n" +
		"| x | centered text | 9 | y |\n| left again | c | right side value | none none none none none none |\n",
	"wide": "| Name | 名前 | Emoji |\n| --- | --- | --- |\n| alpha | 東京都 | 👍 ok |\n" +
		"| beta | 大阪府大阪市北区の長い住所がここに入ります | 🎉🎉🎉 |\n| gamma | ソウル | 👨‍👩‍👧 family |\n| delta | 北京 | done |\n",
	"links": "| Site | Link |\n| --- | --- |\n| one | [docs](https://example.com/docs) |\n" +
		"| two | <https://example.org/a> |\n| three | www.example.net |\n| four | [docs](https://example.com/docs) again |\n",
	"code": "| Cmd | What |\n| --- | --- |\n| `go test` | runs tests |\n| `go vet ./...` | checks code |\n" +
		"| `x` | short |\n| plain | text after code rows that is long enough to wrap at forty |\n",
	"emph": "| A | B |\n| --- | --- |\n| *em* | **strong** |\n| ~~gone~~ | _under_ text |\n| plain | plain |\n" +
		"| more plain text | and some more plain text that wraps |\n",
	"narrow": "| a | b | c | d | e | f |\n|---|---|---|---|---|---|\n| 1 | 2 | 3 | 4 | 5 | 6 |\n" +
		"| one two three four five six | seven | eight nine ten | eleven | twelve thirteen | fourteen |\n| q | w | e | r | t | y |\n",
	"short-rows": "| a | b | c |\n| --- | --- | --- |\n| 1 |\n| 1 | 2 |\n| 1 | 2 | 3 | 4 |\n| solo row with text |\n| |\n|\n| end | end | end |\n",
}

type config struct {
	style string
	width int
}

var configs = []config{
	{"dark", 40}, {"dark", 78}, {"dark", 98},
	{"light", 40}, {"light", 78}, {"light", 98},
	{"notty", 40}, {"notty", 78}, {"notty", 98},
}

// stream feeds srcs to one Table and checks each output against a whole
// render. It returns the Table's stats.
func stream(t *testing.T, g *glamour.TermRenderer, c config, srcs []string) Stats {
	t.Helper()
	tb := newTable(c)
	for i, src := range srcs {
		want, err := g.Render(src)
		if err != nil {
			t.Fatal(err)
		}
		got, err := tb.Render(src, g)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("frame %d (len %d): output differs\nsrc tail: %q\n%s", i, len(src), tail(src, 200), firstDiff(got, want))
		}
	}
	return tb.Stats
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func firstDiff(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var a, b string
		if i < len(g) {
			a = g[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if a != b {
			return fmt.Sprintf("line %d of %d/%d:\n got %q\nwant %q", i, len(g), len(w), a, b)
		}
	}
	return "same lines"
}

// variants: what Stream passes for a prefix of text, whose first block is
// prose and second the table: the healed tail of two blocks (Frame today),
// the unhealed text (Final), and the table alone after the sentinel (tail =
// one block, N4).
func variants(text string, prefixes []string) map[string][]string {
	cut := strings.Index(text, "\n\n") + 2
	out := map[string][]string{}
	for _, p := range prefixes {
		out["healed"] = append(out["healed"], heal.Close(p))
		out["final"] = append(out["final"], p)
		if cut > 0 && len(p) > cut {
			out["sentinel"] = append(out["sentinel"], join.Input(heal.Close(p[cut:]), false))
		}
	}
	return out
}

func rowPrefixes(text string) []string {
	var out []string
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			out = append(out, text[:i+1], text[:i])
		}
	}
	return append(out, text)
}

// TestTable20k: every frame prefix and every row prefix of table-20k, in all
// styles and widths, equals glamour's whole render.
func TestTable20k(t *testing.T) {
	text := table20k(t)
	for _, c := range configs {
		t.Run(fmt.Sprintf("%s-%d", c.style, c.width), func(t *testing.T) {
			t.Parallel()
			g := newGlamour(t, c.style, c.width)
			for name, srcs := range variants(text, corpus.FramePrefixes(text)) {
				if testing.Short() && name != "sentinel" {
					continue
				}
				st := stream(t, g, c, srcs)
				t.Logf("frames %s: %d calls, %+v", name, len(srcs), st)
			}
			st := stream(t, g, c, rowPrefixes(text))
			t.Logf("rows: %+v", st)
		})
	}
}

// TestHandTables: tables with markup (rendered whole), wide runes,
// alignment and short rows, at every token, frame and row prefix.
func TestHandTables(t *testing.T) {
	for name, tab := range handTables {
		text := "Intro line.\n\n" + tab
		for _, c := range configs {
			t.Run(fmt.Sprintf("%s/%s-%d", name, c.style, c.width), func(t *testing.T) {
				t.Parallel()
				g := newGlamour(t, c.style, c.width)
				var total Stats
				for _, ps := range [][]string{corpus.FramePrefixes(text), corpus.TokenPrefixes(text), rowPrefixes(text)} {
					for _, srcs := range variants(text, ps) {
						st := stream(t, g, c, srcs)
						total.Whole += st.Whole
						total.Spliced += st.Spliced
						total.MiniFail += st.MiniFail
					}
				}
				t.Logf("%+v", total)
			})
		}
	}
}

// TestPlainTableSplices: a plain table is served from cached rows on most
// frames (the fast path is used, not only the fallback).
func TestPlainTableSplices(t *testing.T) {
	text := table20k(t)
	g := newGlamour(t, "dark", 78)
	srcs := variants(text, corpus.FramePrefixes(text))["sentinel"]
	tb := newTable(config{"dark", 78})
	for _, src := range srcs {
		if _, err := tb.Render(src, g); err != nil {
			t.Fatal(err)
		}
	}
	if tb.Stats.Spliced < len(srcs)*9/10 {
		t.Fatalf("spliced %d of %d frames: %+v", tb.Stats.Spliced, len(srcs), tb.Stats)
	}
}

// BenchmarkTable20k: cost per frame of streaming table-20k (tail = the
// table after the sentinel, healed), Table vs whole render.
func BenchmarkTable20k(b *testing.B) {
	text := table20k(b)
	for _, w := range []int{78, 98} {
		srcs := variants(text, corpus.FramePrefixes(text))["sentinel"]
		g := newGlamour(b, "dark", w)
		b.Run(fmt.Sprintf("whole/w%d", w), func(b *testing.B) {
			for b.Loop() {
				for _, src := range srcs {
					if _, err := g.Render(src); err != nil {
						b.Fatal(err)
					}
				}
			}
			b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N*len(srcs)), "µs/frame")
		})
		b.Run(fmt.Sprintf("table/w%d", w), func(b *testing.B) {
			var worst time.Duration
			for b.Loop() {
				tb := newTable(config{"dark", w})
				for _, src := range srcs {
					start := time.Now()
					if _, err := tb.Render(src, g); err != nil {
						b.Fatal(err)
					}
					worst = max(worst, time.Since(start))
				}
			}
			b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N*len(srcs)), "µs/frame")
			b.ReportMetric(float64(worst.Microseconds()), "µs-worst")
		})
	}
}
