// Package blocks cuts a Markdown document into top-level blocks that can be
// rendered, and cached, one by one.
//
// Ported from llx-markdown's core/blocks.ts (MIT, see NOTICE). The text is cut
// only where one parse of the whole document is certain to start a new
// top-level block, and blocks are merged wherever that parse reads them
// together. Merging is always safe; a wrong cut never is.
//
// Every block's Source runs up to the next block's Offset: blank lines between
// blocks belong to the block before them. Joining the Sources gives src back.
package blocks

import (
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// Block is one top-level block of a document.
type Block struct {
	// Source is the block's own text, trailing blank lines included.
	Source string
	// Offset is the byte offset of Source in the document.
	Offset int
	// Line is the 1-based line Source starts on.
	Line int
}

var (
	fenceRe            = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	innerFenceRe       = regexp.MustCompile("^[ \t]*(`{3,}|~{3,})(.*)$")
	itemRe             = regexp.MustCompile(`^( {0,3})([-+*]|\d{1,9}[.)])(.*)$`)
	interruptingItemRe = regexp.MustCompile(`^ {0,3}(?:[-+*]|1[.)]) +\S`)
	ruleRe             = regexp.MustCompile(`^ {0,3}(?:(?:-[ \t]*){3,}|(?:_[ \t]*){3,}|(?:\*[ \t]*){3,})$`)
	headingRe          = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]|$)`)
	underlineRe        = regexp.MustCompile(`\n {0,3}(?:=+|-+)[ \t\r]*(?:\n|$)`)
	quoteRe            = regexp.MustCompile(`^ {0,3}>[ \t]?(.*)$`)
	htmlStartRe        = regexp.MustCompile(`^ {0,3}<[A-Za-z/!?]`)
	htmlLineRe         = regexp.MustCompile(`(?m)^ {0,3}<[A-Za-z]`)
	// The character after the tag name is matched, not looked ahead at (RE2).
	htmlLinesRe   = regexp.MustCompile(`(?m)^ {0,3}<([A-Za-z][\w:-]*)[\s>/]`)
	footnoteDefRe = regexp.MustCompile(`\[\^[^\]\s]{1,200}\]:`)
	linkDefRe     = regexp.MustCompile(`^ {0,3}\[[^\]]+\]:`)
	openLabelRe   = regexp.MustCompile(`^ {0,3}\[[^\]]*$`)
	imageOnlyRe   = regexp.MustCompile(`^!\[[^\]]*\]\([^)]*\)$`)
	captionRe     = regexp.MustCompile(`^[*_]`)
	markerRe      = regexp.MustCompile(`^\[!([A-Za-z]+)\]`)
	// A definition list description (goldmark DefinitionList): `:` then a space.
	descriptionRe = regexp.MustCompile(`^ {0,3}:[ \t]`)
)

// longHTML lists HTML blocks that a blank line does not end: how each opens,
// and what closes it.
var longHTML = []struct{ opens, closes *regexp.Regexp }{
	{regexp.MustCompile(`(?i)^ {0,3}<(?:pre|script|style|textarea)(?:[\s>]|$)`), regexp.MustCompile(`(?i)</(?:pre|script|style|textarea)>`)},
	{regexp.MustCompile(`^ {0,3}<!--`), regexp.MustCompile(`-->`)},
	{regexp.MustCompile(`^ {0,3}<\?`), regexp.MustCompile(`\?>`)},
	{regexp.MustCompile(`^ {0,3}<!\[CDATA\[`), regexp.MustCompile(`\]\]>`)},
	{regexp.MustCompile(`^ {0,3}<![A-Za-z]`), regexp.MustCompile(`>`)},
}

var voidTags = map[string]bool{
	"area": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "source": true, "wbr": true,
}

// containerKinds are llx-markdown's container forms (dialect.ts).
var containerKinds = map[string]bool{
	"steps": true, "tabs": true, "stats": true, "pros": true,
	"cons": true, "details": true, "sources": true,
}

// scan is what the line scan knows after a line: only what decides where a
// top-level block can end.
type scan struct {
	// fence is the opening run of the top-level fence the text is inside.
	fence string
	// html closes the HTML block the text is inside, one blank lines do not end.
	html *regexp.Regexp
	// list is the content column of the open top-level list item; 0 outside a list.
	list int
	// unsure: the list may be over (a line left of the item was lazy or not).
	unsure bool
	// markup: the lines since the last blank one may be an HTML block, where a
	// fence line is not a fence.
	markup bool
	// fresh: the next line starts a block (after a blank line, a closed fence, or none).
	fresh bool
	// odd: an odd number of `$$` so far; a display formula may be open.
	odd bool
	// label: a `[label` that has not closed yet; it may be a link definition.
	label bool
	// indented: the last line with text was indented four or more.
	indented bool
	// closed: the last line with text closed a top-level fence.
	closed bool
	// inner is the opening run of a fence inside the open list item.
	inner string
	// seen: a line with text has been read; leading blank lines belong to the first block.
	seen bool
	// dl: a definition list may still be open or be extended (goldmark only).
	dl bool
	// dlTerm: a paragraph started while dl was open, and no description has
	// followed yet. It may be a term; if the next paragraph starts first, it
	// was not, and the list ended before it.
	dlTerm bool
	// stuck: the scan cannot tell what the next lines are; no more cuts.
	stuck bool
	// whole: a definition that resolves across blocks; the document stays one block.
	whole bool
}

func newScan() scan { return scan{fresh: true} }

// indentOf returns the column a line's text starts at (tabs to the next
// multiple of four), or -1 for a blank line.
func indentOf(line string) int {
	column := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			column++
		case '\t':
			column += 4 - column%4
		default:
			return column
		}
	}
	return -1
}

// itemColumn returns the content column of a list item's line, 0 when the line
// is not one, -1 when a tab after the marker leaves the column unknown.
func itemColumn(line string) int {
	m := itemRe.FindStringSubmatch(line)
	if m == nil {
		return 0
	}
	base := len(m[1]) + len(m[2])
	rest := m[3]
	if rest == "" {
		return base + 1
	}
	if rest[0] == '\t' {
		return -1
	}
	if rest[0] != ' ' {
		return 0
	}
	text := strings.TrimLeft(rest, " \t")
	spaces := len(rest) - len(text)
	if text == "" {
		return base + 1
	}
	if strings.Contains(rest[:spaces], "\t") {
		return -1
	}
	if spaces > 4 {
		return base + 1
	}
	return base + spaces
}

// opensFence reports whether fence submatches m open a fence: a backtick
// fence has no backtick in its info string.
func opensFence(m []string) bool {
	return m != nil && !(m[1][0] == '`' && strings.Contains(m[2], "`"))
}

// closesFence reports whether fence submatches m close a fence opened by run.
func closesFence(m []string, run string) bool {
	return m != nil && m[1][0] == run[0] && len(m[1]) >= len(run) && strings.TrimSpace(m[2]) == ""
}

// countDoubleDollars counts `$$` not preceded by a backslash, left to right,
// without overlap.
func countDoubleDollars(line string) int {
	n := 0
	for i := 0; i+1 < len(line); i++ {
		if line[i] == '$' && line[i+1] == '$' && (i == 0 || line[i-1] != '\\') {
			n++
			i++
		}
	}
	return n
}

// step reads one line (without its line ending). It returns true when the line
// starts a new top-level block for certain.
func (s *scan) step(line string) bool {
	seen := s.seen
	if !seen && strings.TrimSpace(line) != "" {
		s.seen = true
	}
	return s.advance(line) && seen
}

func (s *scan) advance(line string) bool {
	if s.stuck || s.whole {
		return false
	}
	if s.fence != "" {
		if closesFence(fenceRe.FindStringSubmatch(line), s.fence) {
			s.fence = ""
			s.fresh = true
			s.closed = true
		}
		return false
	}
	if s.html != nil {
		if s.html.MatchString(line) {
			s.html = nil
		}
		return false
	}
	indent := indentOf(line)
	if indent == -1 {
		s.fresh = true
		s.markup = false
		s.label = false
		return false
	}
	fresh, closed := s.fresh, s.closed
	s.fresh, s.closed = false, false

	if footnoteDefRe.MatchString(line) || linkDefRe.MatchString(line) {
		s.whole = true
		return false
	}
	if s.label {
		if i := strings.IndexByte(line, ']'); i != -1 {
			if i+1 < len(line) && line[i+1] == ':' {
				s.whole = true
				return false
			}
			s.label = false
		}
	} else if openLabelRe.MatchString(line) {
		s.label = true
	}

	odd := s.odd
	if countDoubleDollars(line)%2 == 1 {
		s.odd = !odd
	}

	fence := fenceRe.FindStringSubmatch(line)
	opens := opensFence(fence)
	// Inside a formula or an HTML block a fence line is text, and which of the
	// two it is cannot be told from the lines alone.
	if opens && (odd || s.markup) {
		return s.stick()
	}
	// An indented fence may be inside a definition description.
	if opens && s.dl && indent > 0 {
		return s.stick()
	}

	if s.list > 0 {
		if indent >= s.list {
			// In the item if the list is still open; a top-level fence if not.
			if opens && s.unsure {
				return s.stick()
			}
			inner := innerFenceRe.FindStringSubmatch(line)
			if s.inner != "" {
				if closesFence(inner, s.inner) {
					s.inner = ""
				}
			} else if opensFence(inner) {
				s.inner = inner[1]
			}
			return false
		}
		// A fence in the item that a line left of the item ends: where its last
		// blank lines go depends on what follows.
		if s.inner != "" {
			return s.stick()
		}
		column := 0
		if !ruleRe.MatchString(line) {
			column = itemColumn(line)
		}
		if column == -1 {
			return s.stick()
		}
		if column > 0 {
			// The next item of the list (or of a new one right after it).
			s.list = column
			if fresh {
				s.unsure = false
			}
			return false
		}
		if fresh || opens || headingRe.MatchString(line) || ruleRe.MatchString(line) ||
			quoteRe.MatchString(line) || opensLongHTML(line) {
			// The list is over; the line is read as a top-level one below.
			s.list = 0
			s.unsure = false
		} else {
			// A lazy continuation of the item's paragraph, or the first line
			// after the list.
			s.unsure = true
			if htmlStartRe.MatchString(line) {
				s.markup = true
			}
			return false
		}
	}

	cut := s.topLevel(line, indent, fresh, closed, odd, opens, fence)
	if descriptionRe.MatchString(line) {
		// A description joins the paragraph (term) or list above it.
		s.dl = true
		s.dlTerm = false
		return false
	}
	if s.dl && cut {
		// Indented text may continue a description, and a paragraph may be
		// the next term of the same list.
		if indent > 0 {
			return false
		}
		if startsParagraph(line) {
			if s.dlTerm {
				// The paragraph before had no description: the list ended
				// before it, and this paragraph starts a new block.
				s.dl, s.dlTerm = false, false
				return true
			}
			s.dlTerm = true
			return false
		}
		s.dl, s.dlTerm = false, false
	}
	return cut
}

// topLevel reads a line that is outside every list item.
func (s *scan) topLevel(line string, indent int, fresh, closed, odd, opens bool, fence []string) bool {
	// Indented four or more after a blank line: indented code, which goes on
	// over blank lines. Right after a fence, any line starts a block.
	indented := s.indented
	s.indented = indent >= 4
	cut := closed || (fresh && indent < 4 && !odd && !indented)
	for _, h := range longHTML {
		loc := h.opens.FindStringIndex(line)
		if loc == nil {
			continue
		}
		if !h.closes.MatchString(line[:loc[0]] + line[loc[1]:]) {
			s.html = h.closes
		}
		return cut
	}
	if opens {
		s.fence = fence[1]
		// A fence starts a block wherever it is written.
		return !odd
	}
	column := 0
	if indent < 4 && !ruleRe.MatchString(line) {
		column = itemColumn(line)
	}
	if column == -1 {
		return s.stick()
	}
	// After a line indented four (code or the rest of a paragraph), only the
	// lines before it could say what a list item that cannot interrupt is.
	if column > 0 && indented && !interruptingItemRe.MatchString(line) {
		return s.stick()
	}
	if column > 0 {
		s.list = column
		s.unsure = s.markup || (!fresh && !interruptingItemRe.MatchString(line))
		return cut
	}
	if htmlStartRe.MatchString(line) {
		s.markup = true
	}
	return cut
}

func (s *scan) stick() bool {
	s.stuck = true
	return false
}

func opensLongHTML(line string) bool {
	for _, h := range longHTML {
		if h.opens.MatchString(line) {
			return true
		}
	}
	return false
}

// startsParagraph reports whether a line with text may start a paragraph (a
// definition term), as opposed to a block that cannot be one.
func startsParagraph(line string) bool {
	if headingRe.MatchString(line) || ruleRe.MatchString(line) || quoteRe.MatchString(line) ||
		htmlStartRe.MatchString(line) || opensFence(fenceRe.FindStringSubmatch(line)) {
		return false
	}
	return itemColumn(line) == 0
}

// lineAt returns the line of text that starts at at, without its line ending.
func lineAt(text string, at int) string {
	line := text[at:]
	if end := strings.IndexByte(line, '\n'); end != -1 {
		line = line[:end]
	}
	return strings.TrimSuffix(line, "\r")
}

type tagRes struct{ open, close *regexp.Regexp }

var tagCache sync.Map // lowercase tag name -> tagRes

// tagRegexps returns the open and close tag patterns of tag, compiled once.
func tagRegexps(tag string) tagRes {
	if v, ok := tagCache.Load(tag); ok {
		return v.(tagRes)
	}
	q := regexp.QuoteMeta(tag)
	v := tagRes{
		open:  regexp.MustCompile(`(?i)<` + q + `(?:>|[\s/][^>]*>)`),
		close: regexp.MustCompile(`(?i)</` + q + `\s*>`),
	}
	tagCache.Store(tag, v)
	return v
}

// htmlOpen reports whether raw has a line that opens an HTML tag still open at
// its end.
func htmlOpen(raw string) bool {
	// A comment left open takes whatever follows it.
	if strings.Contains(raw, "<!--") && strings.Count(raw, "<!--") > strings.Count(raw, "-->") {
		return true
	}
	if !htmlLineRe.MatchString(raw) {
		return false
	}
	tried := map[string]bool{}
	for _, m := range htmlLinesRe.FindAllStringSubmatchIndex(raw, -1) {
		tag := strings.ToLower(raw[m[2]:m[3]])
		if tried[tag] || voidTags[tag] {
			continue
		}
		tried[tag] = true
		rest := raw[m[0]:]
		re := tagRegexps(tag)
		opened := 0
		for _, found := range re.open.FindAllString(rest, -1) {
			if !strings.HasSuffix(found, "/>") {
				opened++
			}
		}
		closed := len(re.close.FindAllStringIndex(rest, -1))
		if opened > closed {
			return true
		}
	}
	return false
}

// containerKind returns the container form a quote line opens, or "".
func containerKind(line string) string {
	q := quoteRe.FindStringSubmatch(line)
	if q == nil {
		return ""
	}
	m := markerRe.FindStringSubmatch(strings.TrimLeftFunc(q[1], unicode.IsSpace))
	if m == nil {
		return ""
	}
	if kind := strings.ToLower(m[1]); containerKinds[kind] {
		return kind
	}
	return ""
}

// group joins the pieces the scan cut (starts, offsets into text) into the
// blocks that must render together, and returns where each block starts.
func group(text string, starts []int) []int {
	count := len(starts)
	endOf := func(piece int) int {
		if piece+1 < count {
			return starts[piece+1]
		}
		return len(text)
	}
	first := func(piece int) string {
		if piece < count {
			return lineAt(text, starts[piece])
		}
		return ""
	}
	isItem := func(line string) bool { return !ruleRe.MatchString(line) && itemColumn(line) != 0 }
	// tabsEnd is the piece after the tabs whose first heading is in holder, on
	// line or underlined.
	tabsEnd := func(holder int, line string, fallback int) int {
		if holder >= count {
			return fallback
		}
		opening := headingRe.FindStringSubmatch(line)
		// A heading underlined with `===` has a depth this scan does not read:
		// it ends at a `---`.
		if opening == nil && !underlineRe.MatchString(text[starts[holder]:endOf(holder)]) {
			return fallback
		}
		depth := 1
		if opening != nil {
			depth = len(opening[1])
		}
		end := holder + 1
		for ; end < count; end++ {
			next := first(end)
			if ruleRe.MatchString(next) {
				break
			}
			if h := headingRe.FindStringSubmatch(next); h != nil && len(h[1]) < depth {
				break
			}
		}
		return end
	}
	// form returns the container form a piece holds a marker line for, and the
	// piece after everything it governs. A list form takes the one list right
	// after it; tabs take the sections from the heading after them up to a
	// shallower heading, a `---` or the end. Of several markers, the one that
	// reaches furthest decides.
	form := func(piece int) (string, int) {
		if piece >= count {
			return "", 0
		}
		body := strings.TrimRightFunc(text[starts[piece]:endOf(piece)], unicode.IsSpace)
		if !strings.Contains(body, "[!") {
			return "", 0
		}
		lines := strings.Split(body, "\n")
		for i := range lines {
			lines[i] = strings.TrimSuffix(lines[i], "\r")
		}
		found, reach := "", 0
		for i, line := range lines {
			kind := containerKind(line)
			// The marker opens its quote: the line above is not of the same quote.
			if kind == "" || (i > 0 && quoteRe.MatchString(lines[i-1])) {
				continue
			}
			end := piece + 1
			if i+1 == len(lines) {
				if kind == "tabs" {
					end = tabsEnd(piece+1, first(piece+1), piece+1)
				} else if isItem(first(piece + 1)) {
					end = piece + 2
				}
			} else if kind == "tabs" && !quoteRe.MatchString(lines[i+1]) {
				end = tabsEnd(piece, lines[i+1], piece+1)
			}
			found, reach = kind, max(end, reach)
		}
		return found, reach
	}

	var blocks []int
	for piece := 0; piece < count; {
		from := piece
		kind, end := form(piece)
		if kind != "" {
			piece = end
		} else {
			piece++
		}
		// A pros right before a cons (or the reverse) is set as one pair.
		if kind == "pros" || kind == "cons" {
			other, otherEnd := form(piece)
			if (kind == "pros" && other == "cons") || (kind == "cons" && other == "pros") {
				piece = otherEnd
			}
		}
		// An image alone in its paragraph takes the emphasis line under it as
		// its caption.
		if piece == from+1 && piece < count &&
			imageOnlyRe.MatchString(strings.TrimSpace(text[starts[from]:endOf(from)])) &&
			captionRe.MatchString(strings.TrimLeftFunc(first(piece), unicode.IsSpace)) {
			piece++
		}
		// An HTML block whose tag closes further down.
		for piece < count && htmlOpen(text[starts[from]:starts[piece]]) {
			piece++
		}
		blocks = append(blocks, starts[from])
	}
	return blocks
}

// marksKept is how many complete lines back a scan can resume from (MARKS).
const marksKept = 3

// mark is the scan after a complete line.
type mark struct {
	at    int // offset after that line
	scan  scan
	count int // pieces cut by then
}

// progress is where a scan can be resumed.
type progress struct {
	from   int    // where the scan started: the end of the blocks kept as final
	starts []int  // pieces cut up to the last mark, from first
	marks  []mark // the scan after each of the last complete lines, oldest first
}

// pieces scans text from from on and returns the offsets where it cut, or
// ok=false when a definition keeps the document whole. prog and m are where an
// earlier scan from the same start can go on from.
func pieces(text string, from int, prog *progress, m *mark) (starts []int, next *progress, ok bool) {
	sc := newScan()
	at := from
	var marks []mark
	if m != nil {
		sc = m.scan
		at = m.at
		starts = append([]int(nil), prog.starts[:m.count]...)
		for _, k := range prog.marks {
			if k.at <= m.at {
				marks = append(marks, k)
			}
		}
	} else {
		starts = []int{from}
	}
	// Marks are left after the last complete lines only.
	marked := len(text)
	for back := 0; back < marksKept && marked > at; back++ {
		marked = strings.LastIndexByte(text[:marked], '\n')
	}
	for {
		end := strings.IndexByte(text[at:], '\n')
		if end == -1 {
			break
		}
		end += at
		if sc.step(strings.TrimSuffix(text[at:end], "\r")) && at > from {
			starts = append(starts, at)
		}
		at = end + 1
		if end >= marked {
			marks = append(marks, mark{at: at, scan: sc, count: len(starts)})
		}
	}
	if len(marks) > marksKept {
		marks = marks[len(marks)-marksKept:]
	}
	next = &progress{from: from, starts: append([]int(nil), starts...), marks: marks}
	// The last line is still being written: read, but not remembered.
	if at < len(text) && sc.step(strings.TrimSuffix(text[at:], "\r")) && at > from {
		starts = append(starts, at)
	}
	if sc.whole {
		return nil, nil, false
	}
	return starts, next, true
}

// build makes the blocks of text that the pieces at cuts form, the first on
// line line.
func build(text string, cuts []int, line int) []Block {
	starts := group(text, cuts)
	blocks := make([]Block, len(starts))
	for i, offset := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		blocks[i] = Block{Source: text[offset:end], Offset: offset, Line: line}
		line += strings.Count(blocks[i].Source, "\n")
	}
	return blocks
}

// Split returns the top-level blocks of src, cut only where one parse of the
// whole document would cut. A document with link reference or footnote
// definitions is one block. Split("") is nil.
func Split(src string) []Block {
	if src == "" {
		return nil
	}
	starts, _, ok := pieces(src, 0, nil, nil)
	if !ok {
		return []Block{{Source: src, Offset: 0, Line: 1}}
	}
	return build(src, starts, 1)
}

// reopened is how many blocks at the end are read again. Appended text can
// change only the group it is appended to, and that group can join the one
// before it (a `[!CONS]` under a `[!PROS]`); every group above is final.
const reopened = 2

// Splitter is Split for a document that grows at its end, as a streaming reply
// does. When the new src still starts with all blocks of the last call but the
// last two, it keeps those and scans only the lines that are new. Any other
// src is split afresh. The zero value is ready to use.
type Splitter struct {
	text   string
	blocks []Block
	prog   *progress
	lexed  int // bytes scanned so far
}

// Split returns the same blocks as Split(src). The caller may modify the
// returned slice.
func (s *Splitter) Split(src string) []Block {
	if src == "" {
		*s = Splitter{lexed: s.lexed}
		return nil
	}
	kept := s.blocks[:max(0, len(s.blocks)-reopened)]
	from, line := 0, 1
	if n := len(kept); n > 0 {
		last := kept[n-1]
		from = last.Offset + len(last.Source)
		line = last.Line + strings.Count(last.Source, "\n")
	}
	if from > 0 && !strings.HasPrefix(src, s.text[:from]) {
		kept, from, line = nil, 0, 1
	}
	// The scan goes on from the last line it read that is still there, when
	// it started at the same place.
	var m *mark
	if s.prog != nil && s.prog.from == from {
		for i := len(s.prog.marks) - 1; i >= 0; i-- {
			tried := &s.prog.marks[i]
			if len(src) >= tried.at && src[from:tried.at] == s.text[from:tried.at] {
				m = tried
				break
			}
		}
	}
	if m != nil {
		s.lexed += len(src) - m.at
	} else {
		s.lexed += len(src) - from
	}
	starts, next, ok := pieces(src, from, s.prog, m)
	s.text = src
	if !ok {
		// A definition arrived: everything is one block with it now.
		s.blocks, s.prog = nil, nil
		return []Block{{Source: src, Offset: 0, Line: 1}}
	}
	s.prog = next
	blocks := append(append([]Block(nil), kept...), build(src, starts, line)...)
	s.blocks = blocks
	return append([]Block(nil), blocks...)
}
