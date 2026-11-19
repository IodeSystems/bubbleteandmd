package directive

import (
	"strings"

	"github.com/iodesystems/bubbleteandmd/internal/heal"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// InlineMath rewrites each inline `$…$` formula of src as a code span: its
// two `$` become backticks, so the text keeps its length and every offset
// (no shifts). A formula shows as its TeX source, as code, as a `$$` one
// does (MathFences). Which `$` pair is flowdown v0.7.1's reading
// (heal.MathSpans: prices and shell variables stay text, a formula may
// start with a number when it holds a sign only math writes), within one
// paragraph, heading or table cell as goldmark reads it; code spans, code
// blocks and HTML are not read. Not rewritten: a formula that holds a
// backtick or has one right before or after it (the span would change).
func InlineMath(src string) string {
	out, _ := inlineMath(src, false)
	return out
}

// InlineMathDollars is InlineMath for notty, where glamour draws inline code
// with no mark: the code span keeps the formula's `$` (`$a^2$` becomes
// "`$a^2$`"), so a formula does not read as prose. Each span adds two bytes;
// shifts map the rewritten text back to src.
func InlineMathDollars(src string) (string, []MathShift) {
	return inlineMath(src, true)
}

func inlineMath(src string, dollars bool) (string, []MathShift) {
	if strings.Count(src, "$") < 2 {
		return src, nil
	}
	b := []byte(src)
	root := md.Parser().Parse(text.NewReader(b))
	var out []byte
	var spans [][2]int
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.CodeBlock, *ast.FencedCodeBlock, *ast.HTMLBlock:
			return ast.WalkSkipChildren, nil
		}
		lines := n.Lines()
		if lines.Len() == 0 {
			return ast.WalkContinue, nil
		}
		from, to := lines.At(0).Start, lines.At(lines.Len()-1).Stop
		if strings.Count(src[from:to], "$") < 2 {
			return ast.WalkSkipChildren, nil
		}
		prose := []byte(src[from:to])
		blankInert(n, prose, from)
		for _, p := range heal.MathSpans(string(prose)) {
			o, c := from+p[0], from+p[1]
			if strings.Contains(src[o:c], "`") || (o > 0 && src[o-1] == '`') || (c+1 < len(src) && src[c+1] == '`') {
				continue
			}
			if out == nil {
				out = b
			}
			if dollars {
				spans = append(spans, [2]int{o, c})
			} else {
				out[o], out[c] = '`', '`'
			}
		}
		return ast.WalkSkipChildren, nil
	})
	if out == nil {
		return src, nil
	}
	if !dollars {
		return string(out), nil
	}
	// A backtick goes before each span's opening `$` and after its closing
	// one; the bytes after an inserted backtick sit one further on.
	var w strings.Builder
	var shifts []MathShift
	done := 0
	for _, sp := range spans {
		w.WriteString(src[done:sp[0]])
		w.WriteByte('`')
		shifts = append(shifts, MathShift{w.Len(), 1})
		w.WriteString(src[sp[0] : sp[1]+1])
		w.WriteByte('`')
		shifts = append(shifts, MathShift{w.Len(), 1})
		done = sp[1] + 1
	}
	w.WriteString(src[done:])
	return w.String(), shifts
}

// blankInert blanks, in prose (the text of block n from byte from on), the
// text of n's code spans and raw HTML: a `$` there is no delimiter.
func blankInert(n ast.Node, prose []byte, from int) {
	blank := func(seg text.Segment) {
		for i := max(seg.Start-from, 0); i < seg.Stop-from && i < len(prose); i++ {
			if prose[i] != '\n' {
				prose[i] = ' '
			}
		}
	}
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || c == n {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.CodeSpan:
			for k := c.FirstChild(); k != nil; k = k.NextSibling() {
				if t, ok := k.(*ast.Text); ok {
					blank(t.Segment)
				}
			}
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			for i := 0; i < c.Segments.Len(); i++ {
				blank(c.Segments.At(i))
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
}
