package bubbleteandmd

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
)

// diffBody: hunk, context, a paired run, an unpaired line, a wide line.
const diffBody = "@@ -1,5 +1,6 @@ func f() {\n" +
	" \tx := 1\n" +
	"-\treturn nil\n" +
	"-\t// old\n" +
	"+\treturn fmt.Errorf(\"f: %w\", err)\n" +
	"+\t// new\n" +
	"+\tlog.Printf(\"a line that is wider than every pane in these tests, by a fair margin: %d\", x)\n" +
	"-\tgone()\n" +
	" }\n"

// diffDocs: untitled, titled (Go), patch with an unknown extension, each
// the last block, and one with text after it.
func diffDocs() []invCase {
	return []invCase{
		{"untitled", "Intro.\n\n```diff\n" + diffBody + "```\n"},
		{"titled", "Intro.\n\n```diff title=\"cmd/main.go\"\n" + diffBody + "```\n"},
		{"patch", "Intro.\n\n```patch title=\"x.unknownext\"\n" + diffBody + "```\n"},
		{"after", "```diff\n" + diffBody + "```\n\nAfter the diff.\n"},
	}
}

// TestDiffStyles: the tints per style, the faint headers, word marks on
// paired lines; notty writes no SGR. Every line of an untitled diff piece
// is the code block's width: nothing wraps.
func TestDiffStyles(t *testing.T) {
	md := "```diff\n" + diffBody + "```\n"
	for _, c := range []struct {
		style string
		add   string
		del   string
	}{
		{"dark", "48;5;22m", "48;5;52m"},
		{"light", "48;5;194m", "48;5;224m"},
		{"notty", "", ""},
	} {
		for _, w := range []int{40, 78} {
			r := newTestRenderer(t, c.style, w)
			out, err := r.Render(md)
			if err != nil {
				t.Fatal(err)
			}
			lines := rows(out)
			var code []string
			for _, l := range lines {
				if strings.TrimSpace(xansi.Strip(l)) != "" {
					code = append(code, l)
				}
			}
			src := strings.Split(strings.TrimSuffix(diffBody, "\n"), "\n")
			if len(code) != len(src) {
				t.Fatalf("%s/w%d: %d code lines, want %d (one per source line):\n%s", c.style, w, len(code), len(src), out)
			}
			for i, l := range code {
				if got := xansi.StringWidth(l); got != w-2 {
					t.Errorf("%s/w%d: line %d is %d cells, want %d: %q", c.style, w, i, got, w-2, l)
				}
			}
			if c.style == "notty" {
				if strings.Contains(out, "\x1b") {
					t.Errorf("notty/w%d: SGR in output: %q", w, out)
				}
				if !strings.Contains(code[2], "-    return nil") || !strings.Contains(code[4], "+    return fmt") {
					t.Errorf("notty/w%d: markers or tabs: %q %q", w, code[2], code[4])
				}
				continue
			}
			strong := map[string]string{"dark": "48;5;28m", "light": "48;5;157m"}[c.style]
			strongDel := map[string]string{"dark": "48;5;88m", "light": "48;5;217m"}[c.style]
			for i, want := range []struct{ has, not string }{
				{"\x1b[2m", "48;5;"}, // hunk header: faint, no tint
				{"", "48;5;"},        // context
				{c.del, ""},          // -return nil (paired)
				{c.del, ""},          // -// old (paired)
				{c.add, ""},          // +return fmt...
				{c.add, ""},          // +// new
				{c.add, strong},      // +log... (unpaired: no word marks)
				{c.del, strongDel},   // -gone() (unpaired)
				{"", "48;5;"},        // }
			} {
				if want.has != "" && !strings.Contains(code[i], want.has) {
					t.Errorf("%s/w%d: line %d lacks %q: %q", c.style, w, i, want.has, code[i])
				}
				if want.not != "" && strings.Contains(code[i], want.not) {
					t.Errorf("%s/w%d: line %d has %q: %q", c.style, w, i, want.not, code[i])
				}
			}
			if !strings.Contains(code[2], strongDel+"nil") || !strings.Contains(code[3], strongDel+"old") || !strings.Contains(code[5], strong+"new") {
				t.Errorf("%s/w%d: word marks missing: %q / %q / %q", c.style, w, code[2], code[3], code[5])
			}
			// The cut: the wide line ends in "…" at the block edge.
			if !strings.HasSuffix(strings.TrimRight(xansi.Strip(code[6]), " "), "…") {
				t.Errorf("%s/w%d: wide line not cut with …: %q", c.style, w, xansi.Strip(code[6]))
			}
		}
	}
}

// TestDiffTitledColours: a titled fence colours code by its file's lexer
// (Go keywords); with an unknown extension and with no title the code is
// one colour.
func TestDiffTitledColours(t *testing.T) {
	r := newTestRenderer(t, "dark", 78)
	esc := func(md string) map[string]bool {
		out, err := r.Render(md)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, m := range sgrRe.FindAllString(out, -1) {
			if strings.HasPrefix(m, "\x1b[38;5;") {
				seen[m] = true
			}
		}
		return seen
	}
	body := " func f() {\n-\treturn nil\n+\treturn err\n }\n"
	goC, plainC, noneC := esc("```diff title=\"a.go\"\n"+body+"```\n"), esc("```diff title=\"a.unknownext\"\n"+body+"```\n"), esc("```diff\n"+body+"```\n")
	if len(goC) <= len(plainC) {
		t.Errorf("Go title: %d foreground colours, unknown extension %d: want more", len(goC), len(plainC))
	}
	if len(noneC) > len(plainC)+1 {
		t.Errorf("no title: %d foreground colours, unknown extension %d", len(noneC), len(plainC))
	}
}

// TestDiffWide: with WideBlocks a wide line is not cut at the block but at
// the pane, with the edge note; it does not wrap.
func TestDiffWide(t *testing.T) {
	for _, style := range []string{"dark", "notty"} {
		r := newWideRenderer(t, style, 40, 70)
		out, err := r.Render("```diff\n" + diffBody + "```\n")
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, l := range rows(out) {
			if w := xansi.StringWidth(l); w > 70 {
				t.Errorf("%s: line wider than the pane (%d): %q", style, w, l)
			}
			if strings.Contains(l, "log.Printf") {
				n++
				if !strings.Contains(xansi.Strip(l), "cols →") {
					t.Errorf("%s: no edge note: %q", style, xansi.Strip(l))
				}
			}
		}
		if n != 1 {
			t.Errorf("%s: wide line drawn on %d lines, want 1", style, n)
		}
	}
}

// TestDiffStream: every token prefix of the diff documents, per style,
// width and pane: Frame is the whole render of the healed prefix, Final
// the whole render (checkStream); the frame never shrinks; and inside a
// diff body, every line drawn so far equals the Final's line once SGR is
// removed (Carl B1: colours may change, nothing moves).
func TestDiffStream(t *testing.T) {
	var run, passed, sgrChecks atomic.Int64
	t.Cleanup(func() {
		t.Logf("diff stream: %d prefix checks, %d passed, %d lines compared with Final", run.Load(), passed.Load(), sgrChecks.Load())
	})
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 78} {
			for _, pane := range []int{0, w + 30} {
				for _, c := range diffDocs() {
					t.Run(fmt.Sprintf("%s/w%d/p%d/%s", style, w, pane, c.name), func(t *testing.T) {
						t.Parallel()
						r := newWideRenderer(t, style, w, pane)
						prefixes := corpus.TokenPrefixes(c.text)
						checkStream(t, r, c.text, prefixes, &run, &passed)
						final, err := r.Render(c.text)
						if err != nil {
							t.Fatal(err)
						}
						fl := rows(final)
						opener := strings.Index(c.text, "```")
						bodyAt := opener + strings.IndexByte(c.text[opener:], '\n') + 1
						closeAt := bodyAt + len(diffBody)
						// tailN: lines after the last body line when the fence
						// is the last block.
						probe, err := r.Render(c.text[:bodyAt] + " QQQ\n```\n")
						if err != nil {
							t.Fatal(err)
						}
						pl := rows(probe)
						tailN := 0
						for i := len(pl) - 1; i >= 0; i-- {
							if strings.Contains(pl[i], "QQQ") {
								tailN = len(pl) - i - 1
								break
							}
						}
						s := NewStream(newWideRenderer(t, style, w, pane), "k", NewCache(64))
						last := 0
						for _, p := range prefixes {
							out, err := s.Frame(p)
							if err != nil {
								t.Fatal(err)
							}
							fr := rows(out)
							if len(fr) < last {
								t.Fatalf("prefix len %d: frame shrank %d → %d lines", len(p), last, len(fr))
							}
							last = len(fr)
							if len(p) <= bodyAt || len(p) >= closeAt {
								continue
							}
							n := len(fr) - tailN
							if !strings.HasSuffix(p, "\n") {
								n--
							}
							for i := 0; i < n; i++ {
								sgrChecks.Add(1)
								if sgrRe.ReplaceAllString(fr[i], "") != sgrRe.ReplaceAllString(fl[i], "") {
									t.Fatalf("prefix len %d: line %d moved:\nframe %q\nfinal %q", len(p), i, fr[i], fl[i])
								}
							}
						}
						out, err := s.Final(c.text)
						if err != nil {
							t.Fatal(err)
						}
						if out != final {
							t.Fatalf("Final != Render: %s", firstDiff(final, out))
						}
						if len(rows(out)) < last {
							t.Fatalf("the end shrank %d → %d", last, len(rows(out)))
						}
					})
				}
			}
		}
	}
}

// TestDiffFenceNotChunked: a growing diff fence tail renders whole (the
// line memos make that cheap), so Frame is exact, not SGR-relaxed.
func TestDiffFenceNotChunked(t *testing.T) {
	var b strings.Builder
	b.WriteString("```diff\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "-old %d\n+new %d\n", i, i)
	}
	text := b.String()
	r := newTestRenderer(t, "dark", 78)
	s := NewStream(r, "k", NewCache(64))
	for _, p := range corpus.FramePrefixes(text) {
		got, err := s.Frame(p)
		if err != nil {
			t.Fatal(err)
		}
		if s.stats.lastFence {
			t.Fatalf("prefix len %d: diff fence spliced from chunks", len(p))
		}
		want, err := r.Render(heal.Close(p))
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("prefix len %d: Frame != Render(heal): %s", len(p), firstDiff(want, got))
		}
	}
}

func TestDiffFence(t *testing.T) {
	for src, want := range map[string]string{
		"```diff\n-a\n```\n":        "-a\n",
		"```diff\n-a\n```\n\n":      "-a\n",
		"```patch\n-a\n":            "-a\n",
		"~~~diff\n-a\n~~~\n":        "-a\n",
		"````diff\n```\n-a\n````\n": "```\n-a\n",
		"```Diff title=\"x\"\n-a\n": "-a\n",
		"```diff\n":                 "",
		"```diff\n```\n":            "",
	} {
		got, ok := diffFence(src)
		if !ok || got != want {
			t.Errorf("diffFence(%q) = %q, %v; want %q", src, got, ok, want)
		}
	}
	for _, src := range []string{"```go\n-a\n```\n", " ```diff\n-a\n", "```diff", "```diff\n-a\n```\ntext\n", "``` diff`x\n"} {
		if _, ok := diffFence(src); ok {
			t.Errorf("diffFence(%q) ok", src)
		}
	}
}
