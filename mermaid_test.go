package bubbleteandmd

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// mermaidDoc is testdata/directives/mermaid.md.
func mermaidDoc(t *testing.T) string {
	t.Helper()
	b, err := directiveFiles.ReadFile("testdata/directives/mermaid.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// asCode renders md with diagrams off: every mermaid fence as code.
func asCode(t *testing.T, style string, w int, md string) string {
	t.Helper()
	r := newTestRenderer(t, style, w)
	r.Mermaid(false)
	out, err := r.Render(md)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func renderMD(t *testing.T, style string, w int, md string) string {
	t.Helper()
	out, err := newTestRenderer(t, style, w).Render(md)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestMermaidDraws: flowchart, sequence and ER fences draw (box drawing in
// the output, the source gone); the fallbacks in mermaid.md draw as they
// do with diagrams off. Goldens: testdata/golden/docs/mermaid_*.
func TestMermaidDraws(t *testing.T) {
	for _, src := range []string{
		"flowchart TD\n  A[Start] --> B{Ready?}\n",
		"sequenceDiagram\n  U->>S: GET /items\n",
		"erDiagram\n  CUSTOMER ||--o{ ORDER : places\n",
	} {
		for _, style := range []string{"dark", "light", "notty"} {
			md := "```mermaid\n" + src + "```\n"
			out := renderMD(t, style, 76, md)
			plain := xansi.Strip(out)
			if !strings.Contains(plain, "┌") || strings.Contains(plain, "-->") || strings.Contains(plain, "->>") || strings.Contains(plain, "||--") {
				t.Errorf("%s: %q not drawn:\n%s", style, src, plain)
			}
			if style == "notty" && strings.Contains(out, "\x1b") {
				t.Errorf("notty: SGR in %q", out)
			}
		}
	}
}

// TestMermaidFallback: each case draws as the code block it is with
// diagrams off.
func TestMermaidFallback(t *testing.T) {
	wide := "graph LR\n  Alpha --> Bravo --> Charlie --> Delta --> Echo --> Foxtrot --> Golf --> Hotel --> India --> Juliet\n"
	cases := map[string]string{
		"too wide":     "```mermaid\n" + wide + "```\n",
		"parse error":  "```mermaid\nsequenceDiagram\n  A->>\n```\n",
		"unsupported":  "```mermaid\npie title Pets\n  \"Dogs\" : 3\n```\n",
		"empty":        "```mermaid\n```\n",
		"unclosed":     "```mermaid\ngraph LR\n  A --> B\n",
		"titled":       "```mermaid title=\"a.mmd\"\ngraph LR\n  A --> B\n```\n",
		"in a list":    "- a\n\n  ```mermaid\n  graph LR\n    A --> B\n  ```\n",
		"in a quote":   "> ```mermaid\n> graph LR\n>   A --> B\n> ```\n",
		"in a callout": "> [!NOTE]\n> ```mermaid\n> graph LR\n>   A --> B\n> ```\n",
		"indented":     " ```mermaid\n graph LR\n   A --> B\n ```\n",
		"too long":     "```mermaid\ngraph LR\n" + strings.Repeat("  A --> B\n", mermaidMaxLines) + "```\n",
	}
	for name, md := range cases {
		for _, style := range []string{"dark", "notty"} {
			for _, w := range []int{36, 76} {
				if got, want := renderMD(t, style, w, md), asCode(t, style, w, md); got != want {
					t.Errorf("%s/%s/w%d: %s", name, style, w, firstDiff(want, got))
				}
			}
		}
	}
}

// TestMermaidLibraryFails: an error, a panic and an empty output of the
// library draw the fence as code.
func TestMermaidLibraryFails(t *testing.T) {
	defer func(f func(string, int) (string, error)) { mermaidRender = f }(mermaidRender)
	for i, f := range []func(string, int) (string, error){
		func(string, int) (string, error) { return "", errors.New("no") },
		func(string, int) (string, error) { panic("boom") },
		func(string, int) (string, error) { return " \n\n", nil },
		func(string, int) (string, error) { return "a\tb", nil },
	} {
		mermaidRender = f
		md := fmt.Sprintf("```mermaid\ngraph LR\n  F%d --> G\n```\n", i)
		if got, want := renderMD(t, "dark", 76, md), asCode(t, "dark", 76, md); got != want {
			t.Errorf("case %d: %s", i, firstDiff(want, got))
		}
	}
}

// TestMermaidNotShorter: a diagram with fewer lines than its code block
// gets blank code lines under it: the frame does not shrink at the closer.
func TestMermaidNotShorter(t *testing.T) {
	md := "```mermaid\ngraph LR\n  A --> B\n" + strings.Repeat("%% note\n", 20) + "```\n"
	for _, style := range []string{"dark", "notty"} {
		got, code := renderMD(t, style, 76, md), asCode(t, style, 76, md)
		if !strings.Contains(got, "┌") {
			t.Fatalf("%s: not drawn", style)
		}
		if a, b := strings.Count(got, "\n"), strings.Count(code, "\n"); a != b {
			t.Errorf("%s: %d lines, code %d", style, a, b)
		}
	}
}

// TestMermaidCache: a diagram is drawn once per (source, width): again in
// another renderer and in every frame of a Stream it is a cache hit.
func TestMermaidCache(t *testing.T) {
	md := "Intro\n\n```mermaid\ngraph LR\n  CacheA --> CacheB\n```\n\nAfter.\n"
	count := func() int {
		mermaids.mu.Lock()
		defer mermaids.mu.Unlock()
		return mermaids.renders
	}
	pass := func() int {
		before := count()
		for _, style := range []string{"dark", "light", "notty"} {
			renderMD(t, style, 76, md)
			s := NewStream(newTestRenderer(t, style, 76), style, NewCache(64))
			for _, p := range corpus.TokenPrefixes(md) {
				if _, err := s.Frame(p); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Final(md); err != nil {
				t.Fatal(err)
			}
		}
		return count() - before
	}
	// One per code block width: the styles may share one.
	if n := pass(); n < 1 || n > 3 {
		t.Errorf("first pass: %d library calls, want 1-3", n)
	}
	if n := pass(); n != 0 {
		t.Errorf("second pass: %d library calls, want 0", n)
	}
	before := count()
	renderMD(t, "dark", 60, md)
	if n := count() - before; n != 1 {
		t.Errorf("new width: %d library calls, want 1", n)
	}
}

// TestMermaidStream: while a mermaid fence streams it draws as code; the
// frame where its closer line is whole draws the diagram, and Frame equals
// the whole render of the healed text at every prefix.
func TestMermaidStream(t *testing.T) {
	md := "Intro\n\n```mermaid\ngraph LR\n  A --> B\n```\n\nAfter.\n"
	closer := strings.Index(md, "```\n\nAfter") + 3
	for _, style := range []string{"dark", "notty"} {
		s := NewStream(newTestRenderer(t, style, 76), style, NewCache(64))
		for _, p := range runePrefixes(md) {
			out, err := s.Frame(p)
			if err != nil {
				t.Fatal(err)
			}
			drawn := strings.Contains(xansi.Strip(out), "┌")
			if drawn != (len(p) >= closer) {
				t.Errorf("%s: prefix %q: drawn %v", style, tailOf(p), drawn)
			}
		}
	}
}

// TestMermaidNoShrink: Carl's no-shrink rule over mermaid.md streamed into
// a Doc (wide blocks on), token and rune prefixes, dark/notty, w40/w79.
func TestMermaidNoShrink(t *testing.T) {
	md := mermaidDoc(t)
	short := "Intro\n\n```mermaid\ngraph LR\n  A --> B\n" + strings.Repeat("%% note\n", 12) + "```\n\nAfter.\n"
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{40, 79} {
			for _, pat := range []pattern{{"token", corpus.TokenPrefixes}, {"rune", runePrefixes}} {
				checkNoShrink(t, style, w, "mermaid", pat, md)
				checkNoShrink(t, style, w, "mermaid short", pat, short)
			}
		}
	}
}

// TestMermaidOff: Mermaid(false) draws every fence as code: the bytes of a
// `text` fence.
func TestMermaidOff(t *testing.T) {
	md := "```mermaid\ngraph LR\n  A --> B\n```\n"
	if got, want := asCode(t, "dark", 76, md), renderMD(t, "dark", 76, strings.Replace(md, "mermaid", "text", 1)); got != want {
		t.Errorf("off: %s", firstDiff(want, got))
	}
}

// TestMermaidConcurrent: renderers in many goroutines draw diagrams at
// once (run with -race: mermaid-ascii has no lock).
func TestMermaidConcurrent(t *testing.T) {
	srcs := []string{
		"graph LR\n  A:::red --> B\n  classDef red fill:#f00,color:#f00\n",
		"sequenceDiagram\n  A->>B: hi\n  B-->>A: ok\n",
		"erDiagram\n  CUSTOMER ||--o{ ORDER : places\n",
		"graph TD\n  A --> B\n  B --> C\n  C --> A\n",
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 8 {
				// a width per goroutine and round: cache misses
				if _, ok := drawMermaid(srcs[(i+j)%len(srcs)], 40+i*8+j); !ok {
					t.Errorf("not drawn: %q", srcs[(i+j)%len(srcs)])
				}
			}
		}()
	}
	wg.Wait()
}
