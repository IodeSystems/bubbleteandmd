package blocks

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func docs(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("testdata/*.md")
	if err != nil || len(files) == 0 {
		t.Fatalf("testdata: %v", err)
	}
	out := map[string]string{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(f)] = string(raw)
	}
	return out
}

func trimmed(blocks []Block) []string {
	out := []string{}
	for _, b := range blocks {
		out = append(out, strings.TrimSpace(b.Source))
	}
	return out
}

func TestSplitCases(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      []string
	}{
		// blocks.test.tsx
		{"form and what it governs", "Intro.\n\n> [!STEPS]\n1. **A**\n2. **B**\n\nAfter.",
			[]string{"Intro.", "> [!STEPS]\n1. **A**\n2. **B**", "After."}},
		{"tabs run to a shallower heading", "> [!TABS]\n#### A\nOne.\n\n#### B\nTwo.\n\n## Next\n\nText.",
			[]string{"> [!TABS]\n#### A\nOne.\n\n#### B\nTwo.", "## Next", "Text."}},
		{"pros sit with the cons after them", "> [!PROS]\n- a\n\n> [!CONS]\n- b\n\nAfter.",
			[]string{"> [!PROS]\n- a\n\n> [!CONS]\n- b", "After."}},
		{"footnotes keep the document whole", "A claim.[^1]\n\nMore text.\n\n[^1]: The source.",
			[]string{"A claim.[^1]\n\nMore text.\n\n[^1]: The source."}},
		{"link definitions keep the document whole", "See [the docs][docs].\n\nAnother paragraph.\n\n[docs]: https://example.com",
			[]string{"See [the docs][docs].\n\nAnother paragraph.\n\n[docs]: https://example.com"}},
		// further llx behaviour
		{"paragraphs", "a\n\nb", []string{"a", "b"}},
		{"leading blank lines go with the first block", "\n\na\n\nb", []string{"a", "b"}},
		{"fence is its own block", "P\n\n```\ncode\n\n```\nQ", []string{"P", "```\ncode\n\n```", "Q"}},
		{"list continues after a blank line", "- a\n\n  b\n\nc", []string{"- a\n\n  b", "c"}},
		{"indented code: the line after it is never cut", "p\n\n    a\n\n    b\n\nq", []string{"p\n\n    a\n\n    b\n\nq"}},
		{"html block closed further down", "<div>\n\nx\n\n</div>\n\ny", []string{"<div>\n\nx\n\n</div>", "y"}},
		{"display formula with blank lines", "$$\na\n\nb\n$$\n\nc", []string{"$$\na\n\nb\n$$", "c"}},
		{"image takes its caption", "![a](u)\n\n*cap*\n\nnext", []string{"![a](u)\n\n*cap*", "next"}},
		{"label broken over lines is a definition", "[foo\nbar]: /u\n\nx", []string{"[foo\nbar]: /u\n\nx"}},
		{"crlf", "a\r\n\r\n```\r\nx\r\n```\r\nb", []string{"a", "```\r\nx\r\n```", "b"}},
		// goldmark DefinitionList (Go only)
		{"description joins its term", "Term\n\n: def\n\n# H\n\nP", []string{"Term\n\n: def", "# H", "P"}},
		{"second term joins the list", "T1\n: d1\n\nT2\n: d2", []string{"T1\n: d1\n\nT2\n: d2"}},
		{"indented text continues a description", "T\n: d\n\n  more\n\n# H", []string{"T\n: d\n\n  more", "# H"}},
		{"indented fence in a description sticks", "T\n: d\n\n  ```\n  x\n  ```\n\n# H",
			[]string{"T\n: d\n\n  ```\n  x\n  ```\n\n# H"}},
	} {
		if got := trimmed(Split(c.src)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestSplitEmpty(t *testing.T) {
	if got := Split(""); got != nil {
		t.Errorf("Split(\"\") = %v", got)
	}
}

// checkBlocks: the blocks are src, cut and in order, with offset and line.
func checkBlocks(t *testing.T, name, src string, blocks []Block) {
	t.Helper()
	var joined strings.Builder
	for _, b := range blocks {
		if b.Offset != joined.Len() {
			t.Fatalf("%s: block at %d, want %d", name, b.Offset, joined.Len())
		}
		if want := strings.Count(src[:b.Offset], "\n") + 1; b.Line != want {
			t.Fatalf("%s: block at %d on line %d, want %d", name, b.Offset, b.Line, want)
		}
		joined.WriteString(b.Source)
	}
	if joined.String() != src {
		t.Fatalf("%s: joined blocks are not the document", name)
	}
}

func TestBlocksAreTheDocument(t *testing.T) {
	for name, src := range docs(t) {
		checkBlocks(t, name, src, Split(src))
		crlf := strings.ReplaceAll(src, "\n", "\r\n")
		blocks := Split(crlf)
		checkBlocks(t, name+" crlf", crlf, blocks)
		var lf []string
		for _, b := range blocks {
			lf = append(lf, strings.ReplaceAll(b.Source, "\r\n", "\n"))
		}
		var want []string
		for _, b := range Split(src) {
			want = append(want, b.Source)
		}
		if !reflect.DeepEqual(lf, want) {
			t.Errorf("%s: crlf cut differs from lf", name)
		}
	}
}

func TestDocumentsAreSplit(t *testing.T) {
	// Guard against a corpus that the splitter leaves whole by accident.
	for _, name := range []string{"reply.md", "html.md", "deflist.md"} {
		if n := len(Split(docs(t)[name])); n < 3 {
			t.Errorf("%s: %d blocks", name, n)
		}
	}
}

func TestSplitterEveryPrefix(t *testing.T) {
	for name, src := range docs(t) {
		for _, text := range []string{src, strings.ReplaceAll(src, "\n", "\r\n")} {
			var s Splitter
			for end := 0; end <= len(text); end++ {
				p := text[:end]
				got, want := s.Split(p), Split(p)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s @%d:\n got %+v\nwant %+v", name, end, got, want)
				}
				checkBlocks(t, name, p, got)
			}
		}
	}
}

func TestSplitterKeepsBlocks(t *testing.T) {
	src := docs(t)["reply.md"]
	var s Splitter
	before := s.Split(src[:len(src)/2])
	after := s.Split(src[:len(src)/2+40])
	kept := 0
	for i := range before {
		if i < len(after) && before[i] == after[i] {
			kept++
		}
	}
	if kept < len(before)-reopened {
		t.Errorf("kept %d of %d blocks", kept, len(before))
	}
}

func TestSplitterResets(t *testing.T) {
	d := docs(t)
	var s Splitter
	s.Split(d["reply.md"])
	for _, src := range []string{
		d["lists.md"],
		d["reply.md"][:400] + "\n\nA different ending.",
		d["reply.md"][:200],
		"",
		d["footnotes.md"],
		d["html.md"],
	} {
		if got, want := s.Split(src), Split(src); !reflect.DeepEqual(got, want) {
			t.Errorf("after reset:\n got %+v\nwant %+v", got, want)
		}
	}
}

func TestSplitterIsLinear(t *testing.T) {
	// Each update scans the end of the document, not all of it.
	src := strings.Repeat(docs(t)["reply.md"]+"\n\n", 20)
	var s Splitter
	updates := 0
	for end := 0; end <= len(src); end += 7 {
		s.Split(src[:end])
		updates++
	}
	if whole := len(src) * updates / 2; s.lexed > whole/10 {
		t.Errorf("lexed %d bytes, whole-document rescans would be %d", s.lexed, whole)
	}
}
