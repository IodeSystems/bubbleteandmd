package codestream

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
)

// chromaTies: as in stream_test.go. chroma v2.14.0 findClosest ranges over
// a map, so a colour equally far from two palette entries gets either, per
// render (docs/decisions.md). Both sides are mapped before comparing.
var chromaTies = strings.NewReplacer(
	"\x1b[38;5;242m", "\x1b[38;5;241m",
	"\x1b[38;5;196m", "\x1b[38;5;9m",
)

func untie(s string) string { return chromaTies.Replace(s) }

type fenceCase struct{ name, lang, code string }

var handFences = []fenceCase{
	{"go", "go", "package main\n\nimport \"fmt\"\n\n/* block comment\n   spanning lines\n*/\nfunc main() {\n\ts := `raw\nmulti\nline`\n\tfmt.Println(\"hi\", s)\n\t// done\n\tfor i := 0; i < 3; i++ {\n\t\tfmt.Println(i)\n\t}\n}\n\ntype T struct {\n\tA int\n\tB string\n}"},
	{"ts", "ts", "import { x } from \"y\"\n\n/**\n * doc comment\n * more\n */\nexport async function f(a: number): Promise<string> {\n  const s = `template\n  multi ${a}\n  line`\n  // note\n  return s + await x(a)\n}\n\ninterface I {\n  a: number\n  b?: string\n}\nconst z = 1"},
	{"python", "python", "import os\n\ndef f(a):\n    \"\"\"doc\n    string\n    more\n    \"\"\"\n    x = '''triple\nquote\n'''\n    # c\n    return a + 1\n\nclass C:\n    def m(self):\n        return {'a': 1}\n\nprint(f(2))"},
	{"bash", "bash", "#!/bin/bash\nset -e\ncat <<EOF\nhello $USER\nsecond line\nEOF\n\nif [ -f x ]; then\n  echo \"yes\"\nfi\n# comment\nfor i in 1 2 3; do\n  echo $i\ndone\ncat <<'END'\nraw $x\nEND\necho 'multi\nline string'"},
	{"json", "json", "{\n  \"a\": 1,\n  \"b\": [\n    1,\n    2\n  ],\n  \"c\": {\n    \"d\": \"e\\nf\",\n    \"g\": null,\n    \"h\": true\n  }\n}"},
	{"yaml", "yaml", "a: 1\nb:\n  - x\n  - y\nc:\n  d: e\n  f: |\n    literal\n    block\n  g: \"quoted\"\n# comment\nh: >\n  folded\n  text\ni: end"},
	{"sql", "sql", "SELECT a, b\nFROM t\nWHERE c = 'x'\n  AND d > 3\n/* block\ncomment */\nORDER BY a;\n\n-- line comment\nINSERT INTO t (a, b) VALUES (1, 'multi\nline');\nCREATE TABLE u (\n  id INT PRIMARY KEY,\n  name TEXT\n);"},
	{"nolang", "", "func main() {\n\tfmt.Println(\"hi\")\n}\n\nvar x = 1\nint y = 2;\nif (x) { return 3; }\n/* c\nd */\nprint(\"a\")"},
	{"long", "go", "// " + strings.Repeat("a comment that is much wider than the block ", 5) + "\nx := \"" + strings.Repeat("string-without-spaces-", 12) + "\"\n\ny := 1 // short\nfunc f(" + strings.Repeat("arg int, ", 20) + ") {}\n" + strings.Repeat("z", 250) + "\nend()"},
	{"tabs", "go", "func f() {\n\tif x {\n\t\t\ty\t:=\t1\n\t}\n\t// tab\tin\tcomment\n}\n\tleading\n\t\n\t\t\n\tz"},
	{"emoji", "python", "# 🎉 party 🎉\ns = \"héllo wörld ✓\"\nt = '👩‍👩‍👧 family'\n\nprint(s, t)  # 中文注释 and more text to wrap past the width of forty\nu = \"\"\"\n✨ multi\n🚀 line\n\"\"\""},
}

func fenceMD(lang, code string) string { return "```" + lang + "\n" + code + "\n```\n" }

// pres: text before the fence (join.Sentinel is what Stream puts there).
var pres = []string{"", "<!-- -->\n\n", "Here is the file:\n\n"}

var (
	styles = []string{"dark", "light", "notty"}
	widths = []int{40, 78, 98}
)

func newRenderer(t testing.TB, style string, w int) *glamour.TermRenderer {
	t.Helper()
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(w))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// tally counts frames checked, frames that took the chunked path, and
// chunked frames that are byte-equal to the whole render anyway.
type tally struct{ frames, spliced, equal, finals atomic.Int64 }

func (t *tally) String() string {
	s, e := t.spliced.Load(), t.equal.Load()
	share := 0.0
	if s > 0 {
		share = 100 * float64(e) / float64(s)
	}
	return fmt.Sprintf("%d frames, %d chunked, %d of those byte-equal (%.1f%%), %d Final checks", t.frames.Load(), s, e, share, t.finals.Load())
}

// check runs guarantees (2) and (3) for one frame, and (1) when final.
func check(t *testing.T, f *Fence, r Renderer, src string, final bool, tl *tally) {
	t.Helper()
	whole, err := r.Render(src)
	if err != nil {
		t.Fatal(err)
	}
	want := untie(whole)
	got, err := f.Frame(src, r)
	if err != nil {
		t.Fatal(err)
	}
	got = untie(got)
	tl.frames.Add(1)
	if f.spliced {
		tl.spliced.Add(1)
		if got == want {
			tl.equal.Add(1)
		}
	}
	if got != want {
		wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
		if len(wl) != len(gl) {
			t.Fatalf("src len %d: %d lines, whole has %d", len(src), len(gl), len(wl))
		}
		for i := range wl {
			if ansi.Strip(wl[i]) != ansi.Strip(gl[i]) {
				t.Fatalf("src len %d: line %d text differs\nwhole: %q\nframe: %q", len(src), i, ansi.Strip(wl[i]), ansi.Strip(gl[i]))
			}
			if sgrRe.ReplaceAllString(wl[i], "") != sgrRe.ReplaceAllString(gl[i], "") {
				t.Fatalf("src len %d: line %d differs in more than SGR\nwhole: %q\nframe: %q", len(src), i, wl[i], gl[i])
			}
		}
	}
	if final {
		fin, err := f.Final(src, r)
		if err != nil {
			t.Fatal(err)
		}
		tl.finals.Add(1)
		if untie(fin) != want {
			t.Fatalf("src len %d: Final is not the whole render", len(src))
		}
	}
}

// streams returns the frame and line prefixes of doc, raw and healed (as
// Stream passes the tail).
func streams(doc string, healed bool) map[string][]string {
	out := map[string][]string{"frame": corpus.FramePrefixes(doc), "line": corpus.LinePrefixes(doc)}
	if healed {
		for _, k := range []string{"frame", "line"} {
			hs := make([]string, len(out[k]))
			for i, p := range out[k] {
				hs[i] = heal.Close(p)
			}
			out[k+"-healed"] = hs
		}
	}
	return out
}

// chunkSizes: 40 is ChunkLines; the small sizes put a seam at most lines.
var chunkSizes = []int{1, 2, 3, 5, 40}

func TestHandFences(t *testing.T) {
	tls := map[int]*tally{}
	for _, size := range chunkSizes {
		tls[size] = &tally{}
	}
	t.Cleanup(func() {
		for _, size := range chunkSizes {
			t.Logf("hand fences, chunk %d lines: %s", size, tls[size])
		}
	})
	for _, style := range styles {
		for _, w := range widths {
			t.Run(fmt.Sprintf("%s-%d", style, w), func(t *testing.T) {
				t.Parallel()
				r := newRenderer(t, style, w)
				for _, fc := range handFences {
					for pi, pre := range pres {
						doc := pre + fenceMD(fc.lang, fc.code)
						for name, ps := range streams(doc, true) {
							for _, size := range chunkSizes {
								t.Run(fmt.Sprintf("%s/pre%d/%s/chunk%d", fc.name, pi, name, size), func(t *testing.T) {
									f := &Fence{size: size}
									for _, p := range ps {
										check(t, f, r, p, true, tls[size])
									}
								})
							}
						}
					}
				}
			})
		}
	}
}

// code50k returns the code-50k scenario text: prose, one 900-line fence,
// prose.
func code50k(t testing.TB) string {
	for _, s := range corpus.Scenarios() {
		if s.Name == "code-50k" {
			return s.Text
		}
	}
	t.Fatal("no code-50k scenario")
	return ""
}

// TestCode50k streams the code-50k scenario as Stream would hand its tail
// over: healed frame and line prefixes. Each pattern is cut into shards of
// consecutive prefixes, each with its own Fence (it starts cold), so the run
// is spread over the cores. Every check needs a whole render to compare
// with (up to 212 ms): all ~30k checks take over an hour of CPU, past go
// test's 10-minute default. So: every 10th prefix by default, every 50th
// with -short, every one with CODESTREAM_FULL=1 (and -timeout 90m).
func TestCode50k(t *testing.T) {
	doc := code50k(t)
	ps := streams(doc, true)
	var tl tally
	t.Cleanup(func() { t.Logf("code-50k: %s", &tl) })
	const shards = 6
	stride := 10
	switch {
	case os.Getenv("CODESTREAM_FULL") == "1":
		stride = 1
	case testing.Short():
		stride = 50
	}
	for _, style := range styles {
		for _, w := range widths {
			for _, name := range []string{"frame-healed", "line-healed"} {
				all := ps[name]
				for k := range shards {
					lo, hi := k*len(all)/shards, (k+1)*len(all)/shards
					t.Run(fmt.Sprintf("%s-%d-%s-%d", style, w, name, k), func(t *testing.T) {
						t.Parallel()
						r := newRenderer(t, style, w)
						f := New()
						for i := lo; i < hi; i++ {
							last := i == len(all)-1
							if i%stride != 0 && !last {
								continue
							}
							check(t, f, r, all[i], i%25 == 0 || last, &tl)
						}
					})
				}
			}
		}
	}
}

// TestWrapsLongLines makes sure the long-line fence does wrap at width 40,
// so TestHandFences checks wrapped lines.
func TestWrapsLongLines(t *testing.T) {
	r := newRenderer(t, "dark", 40)
	var long fenceCase
	for _, fc := range handFences {
		if fc.name == "long" {
			long = fc
		}
	}
	out, err := r.Render(fenceMD(long.lang, long.code))
	if err != nil {
		t.Fatal(err)
	}
	src, got := strings.Count(long.code, "\n")+1, strings.Count(out, "\n")
	if got < src+8 {
		t.Fatalf("long fence: %d source lines render as %d lines; expected wrapping", src, got)
	}
}

func BenchmarkCode50k(b *testing.B) {
	doc := code50k(b)
	full := heal.Close(doc[:strings.LastIndex(doc, "\n```")+1])
	r := newRenderer(b, "dark", 78)
	b.Run("whole", func(b *testing.B) {
		for range b.N {
			if _, err := r.Render(full); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("frame-warm", func(b *testing.B) {
		f := New()
		if _, err := f.Frame(full, r); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for range b.N {
			if _, err := f.Frame(full, r); err != nil {
				b.Fatal(err)
			}
		}
	})
	// stream: every frame prefix of the scenario, healed, one Fence; ns/op
	// is the whole stream, /frame is the mean per frame.
	ps := corpus.FramePrefixes(doc)
	for i := range ps {
		ps[i] = heal.Close(ps[i])
	}
	b.Run("stream", func(b *testing.B) {
		for range b.N {
			f := New()
			for _, p := range ps {
				if _, err := f.Frame(p, r); err != nil {
					b.Fatal(err)
				}
			}
		}
		b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(ps)), "ns/frame")
	})
}
