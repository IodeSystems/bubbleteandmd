// Ported from llx-markdown packages/llx-markdown/src/core/remark-answer-containers.ts,
// src/core/remark-answer-blocks.ts and src/dialect.ts (MIT, see NOTICE).

package directive

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
)

var (
	// markerRe is `[!KIND]` at a quote's start, and the blank after it.
	markerRe = regexp.MustCompile(`^\[!([A-Za-z]+)\][ \t]*`)
	// attributionRe is a quote's closing source line: `— Author` or `-- Author`.
	attributionRe = regexp.MustCompile(`^(?:—|--)[ \t]+`)
	// leadSeparator is the text between a lead and what follows it: ` — `,
	// ` · `, `: `, or only a line break.
	leadSeparator = regexp.MustCompile(`^[ \t]*(?:[—–·:|-][ \t]*)?(?:\n[ \t]*)?`)
	hostRe        = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*://(?:[^/?#@]*@)?([^/?#:]+)`)
	fenceTitleRe  = regexp.MustCompile(`(?:^|\s)(?:title|filename|file)=(?:"([^"]*)"|'([^']*)'|(\S+))`)
	fencePathRe   = regexp.MustCompile(`^[\w@.\-/\\\[\]()]*[\w)\]]\.[A-Za-z0-9]+$`)
)

// Parse reads the first directive form in block, a top-level block as
// blocks.Split cuts it. A form whose content does not fit it is no form (it
// stays the quote it was written as): false. A block may hold text before
// Start or after End; Parse(block[End:]) reads a form after the first.
func Parse(block string) (Directive, bool) {
	d := parseDoc(block)
	for i := range d.top {
		if f, ok := d.form(i, true); ok {
			return f, true
		}
	}
	return Directive{}, false
}

// form reads the form top-level node i opens. pair joins a pros and a cons
// written one after the other.
func (d *doc) form(i int, pair bool) (Directive, bool) {
	sp := d.top[i]
	switch n := sp.node.(type) {
	case *ast.FencedCodeBlock:
		return d.fenced(n, sp)
	case *ast.Blockquote:
		inner := parseDoc(dequote(d.s[sp.from:sp.to]))
		if len(inner.top) == 0 {
			return Directive{}, false
		}
		mk, marked := inner.marker(inner.top[0].node)
		if marked && mk.kind == "details" {
			title, body := inner.markedBody(mk)
			return Directive{Kind: KindDetails, Title: title, Start: sp.from, End: sp.to,
				Details: &Details{Title: title, Body: body}}, true
		}
		if marked && mk.single && slices.Contains(ContainerKinds, mk.kind) {
			if f, end, ok := d.container(i, mk, inner); ok {
				f.Start, f.End = sp.from, d.top[end-1].to
				if pair {
					f = d.pairUp(f, end)
				}
				return f, true
			}
		}
		if marked && slices.Contains(CalloutKinds, mk.kind) {
			title, body := inner.markedBody(mk)
			return Directive{Kind: KindCallout, Title: title, Start: sp.from, End: sp.to,
				Callout: &Callout{Kind: mk.kind, Title: title, Body: body}}, true
		}
		if a, ok := inner.attribution(); ok {
			return Directive{Kind: KindAttribution, Start: sp.from, End: sp.to, Attribution: a}, true
		}
	}
	return Directive{}, false
}

// marker is a `[!KIND] title` line opening a quote's first paragraph.
type marker struct {
	kind, title string
	bracket     int  // offset after the `]`
	next        int  // offset of the line after the marker line
	plainLine   bool // the marker line carries no formatting
	single      bool // the paragraph is the marker line alone, without formatting
}

func (d *doc) marker(n ast.Node) (marker, bool) {
	p, ok := n.(*ast.Paragraph)
	if !ok || p.Lines().Len() == 0 {
		return marker{}, false
	}
	seg := p.Lines().At(0)
	if t, ok := p.FirstChild().(*ast.Text); !ok || t.Segment.Start != seg.Start {
		return marker{}, false
	}
	line := strings.TrimRight(string(seg.Value(d.b)), "\r\n")
	m := markerRe.FindStringSubmatch(line)
	if m == nil {
		return marker{}, false
	}
	mk := marker{
		kind:    strings.ToLower(m[1]),
		title:   strings.TrimSpace(line[len(m[0]):]),
		bracket: seg.Start + strings.IndexByte(line, ']') + 1,
		next:    d.lineStart(d.line(seg.Start) + 1),
	}
	// The title is the marker line's own text. When that line carries
	// formatting (`[!TIP] **x**`), the line is body content instead.
	mk.plainLine, mk.single = true, p.Lines().Len() == 1
	ended := false
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			if c.SoftLineBreak() || c.HardLineBreak() {
				ended, mk.single = true, false
			}
		case *ast.String:
		default:
			mk.single = false
			if !ended {
				mk.plainLine = false
			}
		}
	}
	return mk, true
}

// markedBody returns a callout's or a folded section's title and body: the
// marker line's text and what is under it, or no title and the marker line's
// remainder as the body's first line when that line carries formatting.
func (d *doc) markedBody(mk marker) (title, body string) {
	if mk.plainLine {
		return mk.title, trimBody(d.s[mk.next:])
	}
	return "", trimBody(strings.TrimLeft(d.s[mk.bracket:], " \t"))
}

// container reads a list, tabs or verdict form opened by top-level quote i.
// A quote holding only the marker takes the blocks right after it; end is the
// index of the first top-level node it did not take.
func (d *doc) container(i int, mk marker, inner *doc) (f Directive, end int, ok bool) {
	content, cs, end := inner.top[1:], inner, i+1
	if len(content) == 0 {
		content, cs = d.following(mk.kind, i+1), d
		end += len(content)
	}
	if len(content) == 0 {
		return f, 0, false
	}
	f, ok = cs.shape(mk.kind, mk.title, content)
	return f, end, ok
}

// following returns the top-level nodes from index from on that a marker-only
// quote governs, read by grammar: a list form takes the ONE list right after
// it; tabs take the run of sections from the heading right after it up to a
// shallower heading, a `---` or the end.
func (d *doc) following(kind string, from int) []span {
	if from >= len(d.top) {
		return nil
	}
	first := d.top[from]
	if kind != "tabs" {
		if _, ok := first.node.(*ast.List); ok {
			return d.top[from : from+1]
		}
		return nil
	}
	h, ok := first.node.(*ast.Heading)
	if !ok {
		return nil
	}
	end := from
	for ; end < len(d.top); end++ {
		switch n := d.top[end].node.(type) {
		case *ast.ThematicBreak:
			return d.top[from:end]
		case *ast.Heading:
			if n.Level < h.Level {
				return d.top[from:end]
			}
		}
	}
	return d.top[from:end]
}

// shape reads content (spans of d) as the form kind; false when it does not fit.
func (d *doc) shape(kind, title string, content []span) (Directive, bool) {
	f := Directive{Title: title}
	if kind == "tabs" {
		tabs, ok := d.tabs(content)
		if !ok {
			return f, false
		}
		tabs.Title = title
		f.Kind, f.Tabs = KindTabs, tabs
		return f, true
	}
	list, ok := content[0].node.(*ast.List)
	if !ok {
		return f, false
	}
	f.After = trimBody(d.s[content[0].to:content[len(content)-1].to])
	items := d.items(list, content[0])
	switch kind {
	case "steps":
		s := &Steps{Title: title, Ordered: list.IsOrdered(), Start: list.Start}
		for _, it := range items {
			s.Items = append(s.Items, step(it))
		}
		f.Kind, f.Steps = KindSteps, s
	case "stats":
		s := &Stats{Title: title}
		for _, it := range items {
			s.Items = append(s.Items, stat(it))
		}
		f.Kind, f.Stats = KindStats, s
	case "sources":
		s := &Sources{Title: title}
		for _, it := range items {
			s.Items = append(s.Items, source(it))
		}
		f.Kind, f.Sources = KindSources, s
	case "pros", "cons":
		side := make([]Item, len(items))
		for j, it := range items {
			side[j] = Item{Body: trimBody(it)}
		}
		v := &Verdict{}
		if kind == "pros" {
			v.Pros, v.ProsTitle = side, title
		} else {
			v.Cons, v.ConsTitle = side, title
		}
		f.Kind, f.Verdict = KindVerdict, v
	default:
		return f, false
	}
	return f, true
}

// pairUp sets a pros and a cons written one after the other side by side:
// the form at top-level index next joins f when it is the other side.
func (d *doc) pairUp(f Directive, next int) Directive {
	// A side with content after its list is not a bare verdict: it pairs with nothing.
	if f.Kind != KindVerdict || f.After != "" || next >= len(d.top) {
		return f
	}
	g, ok := d.form(next, false)
	if !ok || g.Kind != KindVerdict || (f.Verdict.Pros != nil) == (g.Verdict.Pros != nil) {
		return f
	}
	v := *f.Verdict
	if g.Verdict.Pros != nil {
		v.Pros, v.ProsTitle, v.ConsFirst = g.Verdict.Pros, g.Verdict.ProsTitle, true
	} else {
		v.Cons, v.ConsTitle = g.Verdict.Cons, g.Verdict.ConsTitle
	}
	f.Verdict, f.End, f.After = &v, g.End, g.After
	return f
}

// items returns the Markdown each item of list (at sp) holds.
func (d *doc) items(list *ast.List, sp span) []string {
	var out []string
	for _, it := range d.spans(list, sp.from, sp.to) {
		out = append(out, dedentItem(d.s[it.from:it.to]))
	}
	return out
}

// tabs reads content as one tab per heading of the first heading's depth,
// each holding everything up to the next.
func (d *doc) tabs(content []span) (*Tabs, bool) {
	first, ok := content[0].node.(*ast.Heading)
	if !ok {
		return nil, false
	}
	t := &Tabs{}
	from := -1
	end := content[len(content)-1].to
	for _, sp := range content {
		h, ok := sp.node.(*ast.Heading)
		if !ok || h.Level != first.Level {
			continue
		}
		if n := len(t.Panels); n > 0 {
			t.Panels[n-1].Body = trimBody(d.s[from:sp.from])
		}
		label := strings.TrimSpace(d.labelText(h))
		if label == "" {
			label = strconv.Itoa(len(t.Panels) + 1)
		}
		t.Panels = append(t.Panels, Panel{Label: label})
		from = sp.to
	}
	t.Panels[len(t.Panels)-1].Body = trimBody(d.s[from:end])
	return t, true
}

// run is a paragraph's inline nodes. goldmark places only text: a node
// between two texts spans the bytes between them.
type run struct {
	d          *doc
	kids       []ast.Node
	begin, end int
}

func (d *doc) paragraphRun(n ast.Node) (run, bool) {
	switch n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
	default:
		return run{}, false
	}
	lines := n.Lines()
	if lines.Len() == 0 {
		return run{}, false
	}
	r := run{d: d, begin: lines.At(0).Start, end: lines.At(lines.Len() - 1).Stop}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.kids = append(r.kids, c)
	}
	return r, true
}

// start and stop return the byte range of kid k, -1 when it cannot be placed.
func (r run) start(k int) int {
	if t, ok := r.kids[k].(*ast.Text); ok {
		return t.Segment.Start
	}
	if k == 0 {
		return r.begin
	}
	if t, ok := r.kids[k-1].(*ast.Text); ok {
		return t.Segment.Stop
	}
	return -1
}

func (r run) stop(k int) int {
	if t, ok := r.kids[k].(*ast.Text); ok {
		return t.Segment.Stop
	}
	if k+1 == len(r.kids) {
		return r.end
	}
	if t, ok := r.kids[k+1].(*ast.Text); ok {
		return t.Segment.Start
	}
	return -1
}

// trimLead skips separator text (and a line break) from kid k, at byte pos
// on; it returns the first kid with content and where that content starts.
func (r run) trimLead(k, pos int) (int, int) {
	for ; k < len(r.kids); k++ {
		t, ok := r.kids[k].(*ast.Text)
		if !ok {
			return k, r.start(k)
		}
		from := max(pos, t.Segment.Start)
		v := r.d.s[min(from, t.Segment.Stop):t.Segment.Stop]
		if t.SoftLineBreak() || t.HardLineBreak() {
			v += "\n"
		}
		if cut := len(leadSeparator.FindString(v)); cut < len(v) {
			return k, from + cut
		}
	}
	return k, r.end
}

// delimited returns the inside of kid k's source when it is wrapped in n
// emphasis delimiters (`**…**`, `_…_`).
func (r run) delimited(k, n int) (string, bool) {
	a, b := r.start(k), r.stop(k)
	if a < 0 || b < 0 {
		return "", false
	}
	s := strings.TrimSpace(r.d.s[a:b])
	if len(s) < 2*n || (s[0] != '*' && s[0] != '_') {
		return "", false
	}
	open, close := s[:n], s[len(s)-n:]
	if strings.Count(open, s[:1]) != n || open != close {
		return "", false
	}
	return s[n : len(s)-n], true
}

// emphasis returns the inside of kid k when it is an emphasis of level n.
func (r run) emphasis(k, n int) (string, bool) {
	if k >= len(r.kids) {
		return "", false
	}
	e, ok := r.kids[k].(*ast.Emphasis)
	if !ok || e.Level != n {
		return "", false
	}
	return r.delimited(k, n)
}

// lead splits an item at its bold lead: the lead's inside, the run, the kid
// after the lead and where it starts. False when the item does not open with bold.
func lead(item string) (string, run, int, int, bool) {
	d := parseDoc(item)
	if len(d.top) == 0 {
		return "", run{}, 0, 0, false
	}
	r, ok := d.paragraphRun(d.top[0].node)
	if !ok {
		return "", run{}, 0, 0, false
	}
	k, _ := r.trimLead(0, r.begin)
	title, ok := r.emphasis(k, 2)
	if !ok {
		return "", run{}, 0, 0, false
	}
	k, pos := r.trimLead(k+1, r.stop(k))
	return title, r, k, pos, true
}

// step splits an item into its head (bold title, then an emphasis "when")
// and its body.
func step(item string) Step {
	title, r, k, pos, ok := lead(item)
	if !ok {
		return Step{Body: trimBody(item)}
	}
	s := Step{Title: title}
	if when, ok := r.emphasis(k, 1); ok {
		s.When = when
		_, pos = r.trimLead(k+1, r.stop(k))
	}
	s.Body = trimBody(r.d.s[pos:])
	return s
}

// stat splits an item into its bold value and its label.
func stat(item string) Stat {
	value, r, _, pos, ok := lead(item)
	if !ok {
		return Stat{Label: trimBody(item)}
	}
	return Stat{Value: value, Label: trimBody(r.d.s[pos:])}
}

// source reads an item that opens with a link as a reference.
func source(item string) Source {
	d := parseDoc(item)
	plain := Source{Note: trimBody(item)}
	if len(d.top) == 0 {
		return plain
	}
	r, ok := d.paragraphRun(d.top[0].node)
	if !ok {
		return plain
	}
	k, _ := r.trimLead(0, r.begin)
	if k >= len(r.kids) {
		return plain
	}
	var label, url string
	bare := false
	switch n := r.kids[k].(type) {
	case *ast.Link:
		a, b := r.start(k), r.stop(k)
		if a < 0 || b < 0 {
			return plain
		}
		label, url = linkLabel(d, n, a, b), string(n.Destination)
		bare = n.ChildCount() == 1 && d.labelText(n) == url
	case *ast.AutoLink:
		label, url = string(n.Label(d.b)), string(n.URL(d.b))
		bare = label == url
	default:
		return plain
	}
	s := Source{Title: label, URL: url, Host: hostOf(url)}
	// A bare URL is named by its site; the address is not printed twice.
	if bare && s.Host != "" {
		s.Title, s.Host = s.Host, ""
	}
	_, pos := r.trimLead(k+1, r.stop(k))
	s.Note = trimBody(d.s[pos:])
	return s
}

// linkLabel returns the source inside a link's brackets, the link at [a, b).
func linkLabel(d *doc, n *ast.Link, a, b int) string {
	from := a + strings.IndexByte(d.s[a:b], '[') + 1
	at := from
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering {
			at = max(at, t.Segment.Stop)
		}
		return ast.WalkContinue, nil
	})
	if close := strings.IndexByte(d.s[at:b], ']'); close >= 0 {
		return d.s[from : at+close]
	}
	return ""
}

// hostOf returns the site a link points at, as a reader names it; "" for a relative link.
func hostOf(url string) string {
	m := hostRe.FindStringSubmatch(url)
	if m == nil {
		return ""
	}
	return strings.TrimPrefix(m[1], "www.")
}

// attribution reads a quote whose last paragraph has a line starting with a
// dash: the quote above it and its source.
func (d *doc) attribution() (*Attribution, bool) {
	last := d.top[len(d.top)-1]
	p, ok := last.node.(*ast.Paragraph)
	if !ok {
		return nil, false
	}
	lines := p.Lines()
	for j := lines.Len() - 1; j >= 0; j-- {
		seg := lines.At(j)
		v := string(seg.Value(d.b))
		at := len(v) - len(strings.TrimLeft(v, " \t"))
		m := attributionRe.FindStringIndex(v[at:])
		if m == nil {
			continue
		}
		// A quote that is nothing but its source line has no quotation to attribute.
		if j == 0 && len(d.top) == 1 {
			return nil, false
		}
		end := lines.At(lines.Len() - 1).Stop
		return &Attribution{
			Quote:  trimBody(d.s[:d.lineStart(d.line(seg.Start))]),
			Source: trimBody(d.s[seg.Start+at+m[1] : end]),
		}, true
	}
	return nil, false
}

// fenced reads a ```tree fence, or a fence that names its file.
func (d *doc) fenced(n *ast.FencedCodeBlock, sp span) (Directive, bool) {
	if n.Info == nil {
		return Directive{}, false
	}
	info := strings.TrimSpace(string(n.Info.Segment.Value(d.b)))
	lang, meta := info, ""
	if i := strings.IndexAny(info, " \t"); i >= 0 {
		lang, meta = info[:i], strings.TrimSpace(info[i:])
	}
	var body strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		body.WriteString(strings.Repeat(" ", seg.Padding))
		body.Write(seg.Value(d.b))
	}
	f := Directive{Start: sp.from, End: sp.to}
	if lang == "tree" {
		f.Kind, f.Tree = KindTree, &Tree{Rows: ParseTree(body.String())}
		return f, true
	}
	title := FenceTitle(meta)
	if title == "" {
		return Directive{}, false
	}
	f.Kind, f.Title = KindCodeTitle, title
	f.CodeTitle = &CodeTitle{Title: title, Lang: lang, Body: body.String()}
	return f, true
}

// FenceTitle returns the file a code fence names after its language:
// `title="src/app.ts"` (also `filename=`, `file=`), or the path alone. ""
// when meta names none; any other word there (`{1,3}`) is not one.
func FenceTitle(meta string) string {
	if meta == "" {
		return ""
	}
	if m := fenceTitleRe.FindStringSubmatch(meta); m != nil {
		return strings.TrimSpace(m[1] + m[2] + m[3])
	}
	if bare := strings.TrimSpace(meta); fencePathRe.MatchString(bare) {
		return bare
	}
	return ""
}
