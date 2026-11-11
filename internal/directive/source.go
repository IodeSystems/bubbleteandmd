package directive

import (
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// md reads as glamour v2.0.1 does (glamour.go:71-76): GFM and definition lists.
var md = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList))

// doc is one parse of a Markdown text, with the byte ranges of its top-level
// nodes. goldmark keeps no positions for container nodes, so a range is read
// off the lines its descendants' segments sit on.
type doc struct {
	s      string
	b      []byte
	starts []int // byte offset of each line
	root   ast.Node
	top    []span
}

// span is a node and the lines it owns: from the line it starts on up to the
// line the next sibling starts on, blank lines between included.
type span struct {
	node     ast.Node
	from, to int
}

func parseDoc(s string) *doc {
	d := &doc{s: s, b: []byte(s), starts: []int{0}}
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '\n' {
			d.starts = append(d.starts, i+1)
		}
	}
	d.root = md.Parser().Parse(text.NewReader(d.b))
	d.top = d.spans(d.root, 0, len(s))
	return d
}

// line returns the line holding byte off.
func (d *doc) line(off int) int {
	return sort.Search(len(d.starts), func(i int) bool { return d.starts[i] > off }) - 1
}

// lineStart returns where line i starts; len(s) past the last line.
func (d *doc) lineStart(i int) int {
	if i >= len(d.starts) {
		return len(d.s)
	}
	return d.starts[i]
}

// lineText returns line i without its line ending.
func (d *doc) lineText(i int) string {
	if i < 0 || i >= len(d.starts) {
		return ""
	}
	return strings.TrimRight(d.s[d.starts[i]:d.lineStart(i+1)], "\r\n")
}

func (d *doc) blank(i int) bool { return strings.TrimSpace(d.lineText(i)) == "" }

var (
	underlineLine = regexp.MustCompile(`^ {0,3}(?:=+|-+)[ \t]*$`)
	atxLine       = regexp.MustCompile(`^[ \t>]*#{1,6}(?:[ \t]|$)`)
	markerOnly    = regexp.MustCompile(`^[ \t>]*(?:[-+*]|\d{1,9}[.)])[ \t]*$`)
)

// extent returns the first and last line n's segments touch, a fence's
// opening and closing lines and a setext heading's underline included.
func (d *doc) extent(n ast.Node) (first, last int, ok bool) {
	first, last = -1, -1
	add := func(l int) {
		if l < 0 {
			return
		}
		if first < 0 || l < first {
			first = l
		}
		if l > last {
			last = l
		}
	}
	seg := func(s text.Segment) {
		add(d.line(s.Start))
		if s.Stop > s.Start {
			add(d.line(s.Stop - 1))
		}
	}
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			seg(c.Segment)
		case *ast.FencedCodeBlock:
			open, close := d.fence(c, -1)
			add(open)
			add(close)
		case *ast.HTMLBlock:
			if c.HasClosure() {
				seg(c.ClosureLine)
			}
		}
		if c.Type() == ast.TypeBlock {
			lines := c.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg(lines.At(i))
			}
		}
		return ast.WalkContinue, nil
	})
	if _, heading := n.(*ast.Heading); heading && first >= 0 && !atxLine.MatchString(d.lineText(first)) &&
		underlineLine.MatchString(strings.TrimLeft(d.lineText(last+1), ">")) {
		last++
	}
	return first, last, first >= 0
}

// fence returns the opening and closing lines of a fenced code block, -1 for
// one it cannot place (an unclosed fence; the opening line of an empty fence
// with no info string, unless open names it).
func (d *doc) fence(c *ast.FencedCodeBlock, open int) (int, int) {
	lines := c.Lines()
	close := -1
	var run string
	switch {
	case c.Info != nil:
		open = d.line(c.Info.Segment.Start)
		run = strings.TrimRight(d.s[d.starts[open]:c.Info.Segment.Start], " \t")
	case lines.Len() > 0:
		open = d.line(lines.At(0).Start) - 1
		run = strings.TrimRight(d.lineText(open), " \t")
	case open >= 0:
		run = strings.TrimRight(d.lineText(open), " \t")
	}
	if open < 0 {
		return -1, -1
	}
	n := 0
	if run != "" && (run[len(run)-1] == '`' || run[len(run)-1] == '~') {
		for n < len(run) && run[len(run)-1-n] == run[len(run)-1] {
			n++
		}
	}
	if n < 3 {
		return open, -1
	}
	char := run[len(run)-1]
	next := open + 1
	if lines.Len() > 0 {
		next = d.line(lines.At(lines.Len()-1).Start) + 1
	}
	t := strings.TrimRight(strings.TrimLeft(d.lineText(next), " \t>"), " \t")
	if len(t) >= n && strings.Count(t, string(char)) == len(t) {
		close = next
	}
	return open, close
}

// spans returns the byte ranges of parent's children, which lie in [from, to).
func (d *doc) spans(parent ast.Node, from, to int) []span {
	var out []span
	if parent.FirstChild() == nil || from >= len(d.s) {
		return nil
	}
	prevLast := d.line(from) - 1
	_, list := parent.(*ast.List)
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		first, last, ok := d.extent(c)
		if !ok || first <= prevLast {
			// A node with no segment (a thematic break): the next line with text.
			first = prevLast + 1
			for first < len(d.starts) && d.blank(first) {
				first++
			}
			last = max(last, first)
			if fc, ok := c.(*ast.FencedCodeBlock); ok {
				_, close := d.fence(fc, first)
				last = max(last, close)
			}
		} else if list && first-1 > prevLast && markerOnly.MatchString(d.lineText(first-1)) {
			// An item whose text starts on the line under its marker.
			first--
		}
		start := max(d.lineStart(first), from)
		if n := len(out); n > 0 {
			start = max(start, out[n-1].from)
		}
		out = append(out, span{node: c, from: min(start, to)})
		prevLast = max(prevLast, last)
	}
	for i := range out {
		if i+1 < len(out) {
			out[i].to = out[i+1].from
		} else {
			out[i].to = to
		}
	}
	return out
}

var quotePrefix = regexp.MustCompile(`^ {0,3}>[ \t]?`)

// dequote strips one level of quote markers off each line of s. A lazy line
// (no marker) stays as it is.
func dequote(s string) string {
	lines := strings.SplitAfter(s, "\n")
	for i, l := range lines {
		if m := quotePrefix.FindStringIndex(l); m != nil {
			lines[i] = l[m[1]:]
		}
	}
	return strings.Join(lines, "")
}

var itemHead = regexp.MustCompile(`^([ \t]*)([-+*]|\d{1,9}[.)])([ \t]*)([^\r\n]*)`)

// dedentItem returns a list item's source as the Markdown it holds: the marker
// off its first line, the content indent off the rest.
func dedentItem(s string) string {
	lines := strings.SplitAfter(s, "\n")
	m := itemHead.FindStringSubmatch(lines[0])
	if m == nil {
		return s
	}
	pad := len(m[3])
	if m[4] == "" || pad > 4 {
		pad = min(pad, 1)
	}
	width := len(m[1]) + len(m[2]) + max(pad, 1)
	lines[0] = lines[0][len(m[1])+len(m[2])+pad:]
	for i := 1; i < len(lines); i++ {
		n := 0
		for n < width && n < len(lines[i]) && lines[i][n] == ' ' {
			n++
		}
		lines[i] = lines[i][n:]
	}
	return strings.Join(lines, "")
}

// trimBody drops leading blank lines and trailing white space.
func trimBody(s string) string {
	for {
		nl := strings.IndexByte(s, '\n')
		if nl < 0 || strings.TrimSpace(s[:nl]) != "" {
			break
		}
		s = s[nl+1:]
	}
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return strings.TrimRight(s, " \t\r\n")
}

// labelText is a node's text without its formatting: a bold, linked or coded
// heading still has its name.
func (d *doc) labelText(n ast.Node) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Segment.Value(d.b))
			if c.SoftLineBreak() || c.HardLineBreak() {
				b.WriteByte('\n')
			}
		case *ast.String:
			b.Write(c.Value)
		case *ast.AutoLink:
			b.Write(c.Label(d.b))
		case *ast.RawHTML:
			for i := 0; i < c.Segments.Len(); i++ {
				seg := c.Segments.At(i)
				b.Write(seg.Value(d.b))
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}
