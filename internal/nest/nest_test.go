package nest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func parse(md goldmark.Markdown, src string) ast.Node {
	return md.Parser().Parse(text.NewReader([]byte(src)))
}

// depth returns the most nodes of a kind in keep on one path from root,
// walked without a call per level.
func depth(root ast.Node, keep func(ast.Node) bool) int {
	type at struct {
		n ast.Node
		d int
	}
	most := 0
	stack := []at{{root, 0}}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		d := x.d
		if keep(x.n) {
			d++
			most = max(most, d)
		}
		for c := x.n.FirstChild(); c != nil; c = c.NextSibling() {
			stack = append(stack, at{c, d})
		}
	}
	return most
}

func blocks(n ast.Node) bool {
	return n.Kind() == ast.KindBlockquote || n.Kind() == ast.KindListItem
}

// texts is the text of every Text leaf, in order.
func texts(root ast.Node, src string) string {
	var b strings.Builder
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := n.(*ast.Text); ok && entering {
			b.WriteString(string(t.Segment.Value([]byte(src))))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

func TestBlocksCapped(t *testing.T) {
	md := New()
	for _, c := range []struct{ name, unit, rest string }{
		{"quote", ">", ">>>>> x"},
		{"list", "- ", "- - - - - x"},
		{"ordered", "1. ", "1. 1. 1. 1. 1. x"},
		{"mixed", "> - ", "> - > - > - > - > - x"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := strings.Repeat(c.unit, Blocks) + c.rest + "\n"
			if c.name == "mixed" {
				src = strings.Repeat(c.unit, Blocks/2) + c.rest + "\n"
			}
			doc := parse(md, src)
			if got := depth(doc, blocks); got != Blocks {
				t.Fatalf("depth %d, want %d", got, Blocks)
			}
			// past the cap the line is text of the deepest block
			if got := texts(doc, src); !strings.HasSuffix(got, c.rest) {
				t.Fatalf("text %q, want suffix %q", got, c.rest)
			}
		})
	}
}

func TestInlinesCapped(t *testing.T) {
	md := New()
	n := Inlines + 50
	stock := goldmark.New(goldmark.WithExtensions(extension.GFM))
	for _, c := range []struct{ name, src string }{
		{"emphasis", strings.Repeat("*a _b ", n) + "x" + strings.Repeat("_ *", n)},
		{"strong", strings.Repeat("**a ", n) + "x" + strings.Repeat("**", n)},
		{"links", strings.Repeat("*[a ", n) + "x" + strings.Repeat("](u)*", n)},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.src + "\n"
			doc := parse(md, src)
			spans := func(x ast.Node) bool { return spans(x) }
			full := parse(stock, src)
			if depth(full, spans) <= Inlines {
				t.Fatalf("input nests %d deep in goldmark: not past the cap", full)
			}
			if got := depth(doc, spans); got != Inlines {
				t.Fatalf("depth %d, want %d", got, Inlines)
			}
			// past the cap the spans are kept as their text
			if got, want := texts(doc, src), texts(full, src); got != want {
				t.Fatalf("text %q\nwant %q", got, want)
			}
		})
	}
}

// Under the caps nothing changes: the tree is goldmark's as glamour builds it.
func TestUnderCapsSameTree(t *testing.T) {
	stock := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList), goldmark.WithParserOptions(parser.WithAutoHeadingID()))
	srcs := []string{
		"# Title\n\n> a\n> > b *c* [d](e)\n\n- x\n  - y\n    1. z\n\n| a | b |\n|---|---|\n| *1* | ~~2~~ |\n",
		strings.Repeat(">", Blocks) + " deep\n",
		strings.Repeat("- ", Blocks) + "deep\n",
		strings.Repeat("*a ", Inlines) + "x" + strings.Repeat("*", Inlines) + "\n",
		"Term\n: definition\n\n```go\n> not a quote\n```\n",
	}
	for _, src := range srcs {
		var a, b bytes.Buffer
		dump(&a, parse(stock, src), src)
		dump(&b, parse(New(), src), src)
		if a.String() != b.String() {
			t.Errorf("%q: trees differ\nstock:\n%s\ncapped:\n%s", src, a.String(), b.String())
		}
	}
}

func dump(w *bytes.Buffer, root ast.Node, src string) {
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			w.WriteString(")")
			return ast.WalkContinue, nil
		}
		w.WriteString("(" + n.Kind().String())
		if t, ok := n.(*ast.Text); ok {
			w.WriteString(" " + string(t.Segment.Value([]byte(src))))
		}
		return ast.WalkContinue, nil
	})
}
