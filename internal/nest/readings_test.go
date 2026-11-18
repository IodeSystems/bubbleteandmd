package nest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
)

// shape is the block tree of root: each node's kind (a list with its
// tightness), its children in brackets; text and inlines left out.
func shape(root ast.Node) string {
	var b strings.Builder
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if n.Kind() == ast.KindDocument {
			return ast.WalkContinue, nil
		}
		if n.Type() != ast.TypeBlock {
			return ast.WalkSkipChildren, nil
		}
		if !entering {
			b.WriteString("]")
			return ast.WalkContinue, nil
		}
		b.WriteString(n.Kind().String())
		if l, ok := n.(*ast.List); ok {
			fmt.Fprintf(&b, "(tight=%v)", l.IsTight)
		}
		b.WriteString("[")
		return ast.WalkContinue, nil
	})
	return b.String()
}

// TestMicromarkReadings: the four readings flowdown v0.7.1 (654bfee,
// engine/differential.test.ts) fixed to match micromark, read by our
// goldmark. The first three agree and are pinned. The fourth differs and is
// pinned as goldmark reads it (docs/decisions.md, "flowdown v0.7.1 heal
// port"): a change there is a goldmark change to look at.
func TestMicromarkReadings(t *testing.T) {
	md := New()
	for _, c := range []struct{ name, src, want string }{
		// (1) A blank line after indented code in a list item makes the list loose.
		{"code gap loose", "+     _\n\n  t", "List(tight=false)[ListItem[CodeBlock[]Paragraph[]]]"},
		{"code gap loose", "-     c\n\n  x", "List(tight=false)[ListItem[CodeBlock[]Paragraph[]]]"},
		{"last blank tight", "- a\n- b\n\n", "List(tight=true)[ListItem[TextBlock[]]ListItem[TextBlock[]]]"},
		// (2) A link definition is a block of its item: a blank line before it makes the list loose.
		{"definition loose", "- a\n\n  [r]: /u", "List(tight=false)[ListItem[Paragraph[]LinkReferenceDefinition[]]]"},
		{"example 317", "- a\n- b\n\n  [ref]: /url\n- d",
			"List(tight=false)[ListItem[Paragraph[]]ListItem[Paragraph[]LinkReferenceDefinition[]]ListItem[Paragraph[]]]"},
		// (3) A list item that can interrupt a paragraph comes before a table's row of dashes.
		{"item before dashes", "b\n- |", "Paragraph[]List(tight=true)[ListItem[TextBlock[]]]"},
		{"item before dashes", "a | b\n- | -", "Paragraph[]List(tight=true)[ListItem[TextBlock[]]]"},
		{"dashes without space", "a | b\n-|-", "Table[TableHeader[TableCell[]TableCell[]]]"},
		// (4) Gap: micromark reads `|\n-|` as a paragraph (a lone pipe heads no
		// table); goldmark reads a one-column table with an empty header.
		{"lone pipe header (gap)", "|\n-|", "Table[TableHeader[TableCell[]]]"},
		{"empty cell header", "| |\n|-|", "Table[TableHeader[TableCell[]]]"},
	} {
		if got := shape(parse(md, c.src)); got != c.want {
			t.Errorf("%s %q:\n got %s\nwant %s", c.name, c.src, got, c.want)
		}
	}
}
