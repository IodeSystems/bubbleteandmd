package bubbleteandmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// codeSample is one code block: its fence language and its text.
type codeSample struct{ lang, code string }

var fenceRe = regexp.MustCompile("(?ms)^```([^\\s`]*)[^\\n]*\\n(.*?)^```")

// codeSamples: hand samples (CRLF, multi-line tokens, no lexer) and every
// fenced code block of the corpus documents and scenarios.
func codeSamples() []codeSample {
	out := []codeSample{
		{"go", "package main\n\nimport \"fmt\"\n\n// hi\nfunc main() {\n\tfmt.Println(\"x\", 1.5, `raw\nmulti`)\r\n}\n"},
		{"py", "def f(x):\n    \"\"\"doc\n    string\"\"\"\n    return [i for i in range(3)] # c\n"},
		{"json", "{\"a\": [1,2,{\"b\": null}], \"c\": true}\n"},
		{"sh", "echo $HOME | grep -v 'x' && ls -la # c\n"},
		{"diff", "--- a\n+++ b\n@@ -1 +1 @@\n-x\n+y\n"},
		{"", "plain text with no lexer\n\nline"},
	}
	var texts []string
	for _, d := range corpus.Docs() {
		texts = append(texts, d.Text)
	}
	for _, s := range corpus.Scenarios() {
		texts = append(texts, s.Text)
	}
	for _, t := range texts {
		for _, m := range fenceRe.FindAllStringSubmatch(t, -1) {
			out = append(out, codeSample{m[1], m[2]})
		}
	}
	return out
}

func lexer(lang, code string) chroma.Lexer {
	if l := lexers.Get(lang); l != nil {
		return l
	}
	if l := lexers.Analyse(code); l != nil {
		return l
	}
	return lexers.Fallback
}

// testStyles: every chroma style in the registry (glamour's "charm" too once
// a glamour renderer has drawn code), plus styles with no Text entry and
// with a Background only, so Format's lookup falls through to Background.
func testStyles(t *testing.T) []*chroma.Style {
	t.Helper()
	// Registers glamour's "charm" chroma style (ansi/codeblock.go).
	g, err := glamour.NewTermRenderer(glamour.WithStyles(*styles.DefaultStyles["dark"]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Render("```go\nx\n```\n"); err != nil {
		t.Fatal(err)
	}
	names := chromastyles.Names()
	sort.Strings(names)
	var out []*chroma.Style
	for _, n := range names {
		out = append(out, chromastyles.Get(n))
	}
	return append(out,
		chroma.MustNewStyle("no-text", chroma.StyleEntries{chroma.Keyword: "bold #ff0000", chroma.Comment: "italic #00ff00"}),
		chroma.MustNewStyle("bg-only", chroma.StyleEntries{chroma.Background: "#0000ff bg:#ffffff"}),
		chroma.MustNewStyle("empty", chroma.StyleEntries{}))
}

// palette256 reads stock terminal256's colour tables (unexported fields of
// chroma's indexedTTYFormatter, read with reflect; chroma v2.27.0).
func palette256(t *testing.T) (fg, bg map[chroma.Colour]string) {
	t.Helper()
	tab := reflect.ValueOf(formatters.Get("terminal256")).Elem().Field(0).Elem()
	read := func(m reflect.Value) map[chroma.Colour]string {
		out := map[chroma.Colour]string{}
		for it := m.MapRange(); it.Next(); {
			out[chroma.Colour(it.Key().Int())] = it.Value().String()
		}
		return out
	}
	fg, bg = read(tab.Field(0)), read(tab.Field(1))
	if len(fg) < 200 || len(bg) < 200 {
		t.Fatalf("palette: %d fg, %d bg colours", len(fg), len(bg))
	}
	return fg, bg
}

// closest: the escapes of every palette colour at the least distance from c.
// Stock terminal256 (findClosest) takes the first such colour in map order,
// so with more than one (a tie) its output changes from call to call.
func closest(tab map[chroma.Colour]string, c chroma.Colour) []string {
	best := math.MaxFloat64
	var out []string
	for k, esc := range tab {
		switch d := k.Distance(c); {
		case d < best:
			best, out = d, []string{esc}
		case d == best:
			out = append(out, esc)
		}
	}
	return out
}

// escapes: per token type of style, every escape stock terminal256 can write
// for it (chroma entryToEscapeSequence after clearBackground).
func escapes(t *testing.T, style *chroma.Style) map[chroma.TokenType][]string {
	t.Helper()
	return escapesPick(t, style, func(e []string) []string { return e })
}

// escapeIndex is the palette index in a 256 colour escape.
func escapeIndex(esc string) int {
	var n int
	if _, err := fmt.Sscanf(esc[strings.LastIndex(esc, ";")+1:], "%dm", &n); err != nil {
		panic(err)
	}
	return n
}

// lowestIndex keeps the escape with the lowest palette index.
func lowestIndex(e []string) []string {
	best := e[0]
	for _, x := range e[1:] {
		if escapeIndex(x) < escapeIndex(best) {
			best = x
		}
	}
	return []string{best}
}

// escapesPick is escapes with pick applied to the candidates of each colour.
func escapesPick(t *testing.T, style *chroma.Style, pick func([]string) []string) map[chroma.TokenType][]string {
	t.Helper()
	fg, bg := palette256(t)
	b := style.Builder()
	e := b.Get(chroma.Background)
	e.Background, e.NoInherit = 0, true
	b.AddEntry(chroma.Background, e)
	cleared, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	out := map[chroma.TokenType][]string{}
	for _, tt := range cleared.Types() {
		e := cleared.Get(tt)
		pre := ""
		if e.Bold == chroma.Yes {
			pre += "\x1b[1m"
		}
		if e.Underline == chroma.Yes {
			pre += "\x1b[4m"
		}
		if e.Italic == chroma.Yes {
			pre += "\x1b[3m"
		}
		fgs, bgs := []string{""}, []string{""}
		if e.Colour.IsSet() {
			fgs = pick(closest(fg, e.Colour))
		}
		if e.Background.IsSet() {
			bgs = pick(closest(bg, e.Background))
		}
		for _, f := range fgs {
			for _, g := range bgs {
				out[tt] = append(out[tt], pre+f+g)
			}
		}
	}
	return out
}

// ties: the escapes of the token types of style that stock writes in more
// than one way.
func ties(esc map[chroma.TokenType][]string) []string {
	var out []string
	for _, e := range esc {
		if len(e) > 1 {
			out = append(out, e...)
		}
	}
	return out
}

// TestCachedFormatterEqual: for every style, each cached escape is one stock
// can write; with no tie in the style, the cached formatter writes stock
// terminal256's bytes for every code sample, also on the second use of the
// style (table cached). Stock is not deterministic on a tie (see closest).
func TestCachedFormatterEqual(t *testing.T) {
	stock := formatters.Get("terminal256")
	cf := newCachedFormatter()
	samples := codeSamples()
	if len(samples) < 20 {
		t.Fatalf("only %d code samples", len(samples))
	}
	sts := testStyles(t)
	if testing.Short() {
		sts = append(sts[:6], sts[len(sts)-3:]...)
	}
	checks, tied := 0, 0
	for _, st := range sts {
		esc := escapes(t, st)
		tab := cf.table(st)
		if len(tab) != len(esc) {
			t.Fatalf("style %s: %d cached types, %d stock types", st.Name, len(tab), len(esc))
		}
		for tt, want := range esc {
			if !slices.Contains(want, tab[tt]) {
				t.Fatalf("style %s %v: cached %q, stock writes one of %q", st.Name, tt, tab[tt], want)
			}
		}
		if len(ties(esc)) > 0 {
			tied++
			for tt, want := range escapesPick(t, st, lowestIndex) {
				if tab[tt] != want[0] {
					t.Fatalf("style %s %v: cached %q, lowest index %q", st.Name, tt, tab[tt], want[0])
				}
			}
			continue
		}
		for pass := 0; pass < 2; pass++ {
			for _, s := range samples {
				l := lexer(s.lang, s.code)
				var a, b bytes.Buffer
				it, err := l.Tokenise(nil, s.code)
				if err != nil {
					t.Fatal(err)
				}
				_ = stock.Format(&a, st, it)
				it, _ = l.Tokenise(nil, s.code)
				_ = cf.Format(&b, st, it)
				if a.String() != b.String() {
					t.Fatalf("style %s lang %q: cached differs\nstock  %q\ncached %q", st.Name, s.lang, a.String(), b.String())
				}
				checks++
			}
		}
	}
	t.Logf("%d styles (%d with a tie: escapes checked only), %d samples, %d byte checks", len(sts), tied, len(samples), checks)
}

// TestPaletteEqualsStock: the computed palette has stock's colours and, for
// a colour listed twice, stock's (later) index.
func TestPaletteEqualsStock(t *testing.T) {
	fg, _ := palette256(t)
	want := map[chroma.Colour]string{}
	for i, c := range palette {
		want[c] = "\x1b[38;5;" + strconv.Itoa(i) + "m"
	}
	if !reflect.DeepEqual(fg, want) {
		t.Fatal("palette differs from chroma ttyTables[256]")
	}
	if len(candidates) != len(fg) {
		t.Fatalf("%d candidates, %d stock colours", len(candidates), len(fg))
	}
}

// TestCachedFormatterTieBreak: for the styles with a palette tie, the table
// is the lowest-index choice and equal across 50 fresh formatters (stock
// differs from build to build); for the others it is stock's bytes.
func TestCachedFormatterTieBreak(t *testing.T) {
	stock := formatters.Get("terminal256")
	tied := 0
	for _, st := range testStyles(t) {
		esc := escapes(t, st)
		first := newCachedFormatter().table(st)
		if len(ties(esc)) > 0 {
			tied++
			for tt, want := range escapesPick(t, st, lowestIndex) {
				if first[tt] != want[0] {
					t.Fatalf("style %s %v: %q, lowest index %q", st.Name, tt, first[tt], want[0])
				}
			}
			for i := 0; i < 50; i++ {
				if !reflect.DeepEqual(newCachedFormatter().table(st), first) {
					t.Fatalf("style %s: build %d differs", st.Name, i)
				}
			}
			continue
		}
		var toks []chroma.Token
		for _, tt := range st.Types() {
			toks = append(toks, chroma.Token{Type: tt, Value: "x\ny"})
		}
		var a, b bytes.Buffer
		_ = stock.Format(&a, st, chroma.Literator(toks...))
		_ = newCachedFormatter().Format(&b, st, chroma.Literator(toks...))
		if a.String() != b.String() {
			t.Fatalf("style %s: no tie, but differs from stock", st.Name)
		}
	}
	if tied == 0 {
		t.Fatal("no styles with a tie")
	}
	t.Logf("%d styles with a tie", tied)
}

// TestRendererChromaFormatter: glamour with the cached formatter renders
// every corpus document and scenario as glamour with "terminal256" does, in
// every glamour style and its own chroma colours. glamour registers one
// "charm" chroma style per process (the first style config wins); this test
// removes it before each style and puts the first one back after. Escapes of
// tied token types (glamour dark: Comment, 241 or 242) are read as one.
// Not parallel: chroma's style registry has no lock.
func TestRendererChromaFormatter(t *testing.T) {
	registerChromaFormatter()
	if formatters.Get("terminal256") == formatters.Get(chromaFormatter) {
		t.Fatal("terminal256 replaced")
	}
	var docs []string
	for _, d := range corpus.Docs() {
		docs = append(docs, d.Text)
	}
	for _, s := range corpus.Scenarios() {
		if testing.Short() && len(s.Text) > 30000 {
			continue
		}
		docs = append(docs, s.Text)
	}
	for _, sample := range codeSamples()[:6] {
		docs = append(docs, "```"+sample.lang+"\n"+sample.code+"\n```\n")
	}
	names := make([]string, 0, len(styles.DefaultStyles))
	for n := range styles.DefaultStyles {
		names = append(names, n)
	}
	sort.Strings(names)
	const charm = "charm" // glamour ansi chromaStyleTheme
	if orig, ok := chromastyles.Registry[charm]; ok {
		defer func() { chromastyles.Registry[charm] = orig }()
	} else {
		defer delete(chromastyles.Registry, charm)
	}
	for _, n := range names {
		delete(chromastyles.Registry, charm)
		for _, w := range []int{40, 80} {
			mk := func(f string) *glamour.TermRenderer {
				g, err := glamour.NewTermRenderer(glamour.WithStyles(*styles.DefaultStyles[n]), glamour.WithWordWrap(w), glamour.WithChromaFormatter(f))
				if err != nil {
					t.Fatal(err)
				}
				return g
			}
			stock, cached := mk("terminal256"), mk(chromaFormatter)
			if _, err := stock.Render("```go\nx\n```\n"); err != nil {
				t.Fatal(err)
			}
			var tie *strings.Replacer
			if st, ok := chromastyles.Registry[charm]; ok && styles.DefaultStyles[n].CodeBlock.Chroma != nil {
				if tt := ties(escapes(t, st)); len(tt) > 0 {
					t.Logf("style %s: tied escapes %q", n, tt)
					var pairs []string
					for _, e := range tt {
						pairs = append(pairs, e, "\x00tie\x00")
					}
					tie = strings.NewReplacer(pairs...)
				}
			}
			for i, d := range docs {
				a, err := stock.Render(d)
				if err != nil {
					t.Fatal(err)
				}
				b, err := cached.Render(d)
				if err != nil {
					t.Fatal(err)
				}
				if tie != nil {
					a, b = tie.Replace(a), tie.Replace(b)
				}
				if a != b {
					t.Fatalf("style %s w%d doc %d: cached formatter differs", n, w, i)
				}
			}
		}
	}
}

// countingFormatter counts Format calls of the formatter it wraps.
type countingFormatter struct {
	f chroma.Formatter
	n int
}

func (c *countingFormatter) Format(w io.Writer, s *chroma.Style, it chroma.Iterator) error {
	c.n++
	return c.f.Format(w, s, it)
}

// TestNewRendererUsesCachedFormatter: DirectiveRenderer's outer and inner
// renderers and GlamourOptions use the cached formatter, and its bytes are
// those with stock terminal256 passed as an option (opts come last). Not
// parallel: it swaps the registry entry (chroma's registry has no lock).
func TestNewRendererUsesCachedFormatter(t *testing.T) {
	r := newTestRenderer(t, "dark", 80)
	if o, _ := r.GlamourOptions(); o.ChromaFormatter != chromaFormatter {
		t.Fatalf("GlamourOptions().ChromaFormatter = %q", o.ChromaFormatter)
	}
	stock, err := NewRenderer(*styles.DefaultStyles["dark"], 80, glamour.WithChromaFormatter("terminal256"))
	if err != nil {
		t.Fatal(err)
	}
	orig := formatters.Get(chromaFormatter)
	cnt := &countingFormatter{f: orig}
	formatters.Register(chromaFormatter, cnt)
	defer formatters.Register(chromaFormatter, orig)
	// A code block at the top level (outer) and one in a form body (inner).
	for _, c := range []struct{ md string }{
		{"```go\nfunc main() {}\n```\n"},
		{"> [!NOTE]\n> ```go\n> x := 1\n> ```\n"},
	} {
		n := cnt.n
		b, err := r.Render(c.md)
		if err != nil {
			t.Fatal(err)
		}
		if cnt.n == n {
			t.Errorf("%q: cached formatter not called", c.md)
		}
		n = cnt.n
		a, err := stock.Render(c.md)
		if err != nil {
			t.Fatal(err)
		}
		if cnt.n != n {
			t.Errorf("%q: stock renderer called the cached formatter", c.md)
		}
		if a != b || !strings.Contains(b, "\x1b[") {
			t.Errorf("differs or no colour\nstock  %q\ncached %q", a, b)
		}
	}
}

// panicLexer is chroma's plaintext lexer under its own name, with an
// iterator that panics with v at the first token.
type panicLexer struct {
	chroma.Lexer
	name string
	v    any
}

func (l panicLexer) Config() *chroma.Config {
	return &chroma.Config{Name: l.name, Aliases: []string{l.name}}
}

func (l panicLexer) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	return func() chroma.Token { panic(l.v) }, nil
}

var errLexPanic = errors.New("lexer panic")

// Registered before any test runs: chroma's lexer registry has no lock.
func init() {
	lexers.Register(panicLexer{lexers.Get("text"), "bubbleteandmd-panic-err", errLexPanic})
	lexers.Register(panicLexer{lexers.Get("text"), "bubbleteandmd-panic-str", "boom"})
}

// TestCachedFormatterPanic: a panic in the iterator is an error, not a
// crash (error values wrapped, other values formatted).
func TestCachedFormatterPanic(t *testing.T) {
	f := newCachedFormatter()
	st := chromastyles.Get("monokai")
	err := f.Format(io.Discard, st, func() chroma.Token { panic(errLexPanic) })
	if !errors.Is(err, errLexPanic) {
		t.Errorf("error panic: got %v", err)
	}
	err = f.Format(io.Discard, st, func() chroma.Token { panic("boom") })
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("string panic: got %v", err)
	}
}

// TestRenderLexerPanic: a lexer that panics in a code block (top level, in
// a callout body) makes Render and Stream return "" and the error.
func TestRenderLexerPanic(t *testing.T) {
	r, err := NewRenderer(*styles.DefaultStyles["dark"], 80)
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"bubbleteandmd-panic-err", "bubbleteandmd-panic-str"} {
		for _, md := range []string{
			"# t\n\nprose\n\n```" + lang + "\nx := 1\n```\n",
			"> [!NOTE]\n> ```" + lang + "\n> x := 1\n> ```\n",
		} {
			out, err := r.Render(md)
			if err == nil || out != "" {
				t.Errorf("Render %q: out %q err %v", md, out, err)
			}
			if lang == "bubbleteandmd-panic-err" && !errors.Is(err, errLexPanic) {
				t.Errorf("Render %q: error %v does not wrap the panic", md, err)
			}
			s := NewStream(r, "dark/80", NewCache(16))
			for _, f := range []func(string) (string, error){s.Frame, s.Final} {
				if out, err := f(md); err == nil || out != "" {
					t.Errorf("Stream %q: out %q err %v", md, out, err)
				}
			}
		}
	}
}
