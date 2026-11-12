package bubbleteandmd

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"charm.land/glamour/v2/ansi"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/diffview"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
	"github.com/iodesystems/bubbleteandmd/internal/join"
)

// Diff fences (Carl 2026-11-11, level 2). A ```diff (or ```patch) fence
// draws one line per source line, never wrapped: added and removed lines on
// a background tint the width of the code block (Theme.Diff), hunk and
// file headers faint (Theme.Muted), as llx-markdown's styles/code.css
// draws them. A titled fence (`title="x.go"`) colours the code of each line
// with the lexer of the title's file name, the line lexed alone without its
// marker; with no title, or a name chroma does not know, the code is plain.
// A removed run followed directly by an added run pairs index-wise, and
// the words a pair changed get a stronger tint (diffview.Words). Only
// colours depend on the lines around a line, never its cells: a Frame
// differs from the Final only in SGR (Carl B1).
//
// The piece keeps glamour's code block frame (blank line, margin, indent,
// pad cells), read once per renderer from a render of a one-line fence. A
// line wider than the code block is cut at it with "…"; with WideBlocks it
// is cut at the pane with the edge note, as other wide code. notty: no SGR,
// the +/- marker is the only cue.
//
// Scope: a top-level fence at indent 0 (also in a form body); titled
// fences draw in the titled code form. Fences in lists and quotes render
// through glamour (chroma's diff lexer colours), as before.

// diffCal is how glamour draws one code line in a target: the lines before
// and after the code, the left margin and indent (lead), the pad cell, and
// the cells the code area has.
type diffCal struct {
	head, tail []string
	lead       string
	cell       string
	area       int
	// bad: the render had no such line; the fence renders through glamour.
	bad bool
}

// diffCacheMax bounds the per-renderer line memos; past it a map is dropped.
const diffCacheMax = 1 << 14

// diffState is a DirectiveRenderer's diff state: its chroma escape table,
// and memos of lexed and of drawn lines.
type diffState struct {
	tab   map[chroma.TokenType]string
	built bool
	toks  map[tokKey][]tok
	lines map[lineKey]string
}

type tokKey struct {
	lexer chroma.Lexer
	line  string
}

// tok is one token of a lexed line: its type and length in bytes.
type tok struct {
	typ chroma.TokenType
	n   int
}

// diffFence reports whether src (one top-level block) is one fenced code
// block at indent 0 whose language is a diff, and returns its code lines.
// An unclosed fence runs to the end; after a closing line only blank lines.
func diffFence(src string) (body string, ok bool) {
	nl := strings.IndexByte(src, '\n')
	if nl < 0 {
		return "", false
	}
	m := fenceOpenRe.FindStringSubmatch(src[:nl])
	if m == nil || len(m[0]) != len(m[1])+len(m[2]) || (m[1][0] == '`' && strings.Contains(m[2], "`")) {
		return "", false
	}
	lang := strings.TrimSpace(m[2])
	if i := strings.IndexAny(lang, " \t"); i >= 0 {
		lang = lang[:i]
	}
	if !directive.IsDiffLang(lang) {
		return "", false
	}
	rest := src[nl+1:]
	for i := 0; i < len(rest); {
		j := strings.IndexByte(rest[i:], '\n')
		end := len(rest)
		if j >= 0 {
			end = i + j
		}
		if c := codeFenceRe.FindStringSubmatch(rest[i:end]); c != nil && c[1][0] == m[1][0] && len(c[1]) >= len(m[1]) && strings.TrimSpace(rest[i+len(c[0]):end]) == "" {
			if strings.TrimSpace(rest[end:]) != "" {
				return "", false
			}
			return rest[:i], true
		}
		if j < 0 {
			break
		}
		i = end + 1
	}
	return rest, true
}

// diffPiece renders a diff fence block (src, its code body) in target t.
// A fence with no code line renders through glamour.
func (r *DirectiveRenderer) diffPiece(t *target, pieces []piece, src, body string, first bool) ([]piece, error) {
	if body == "" {
		return r.glamourPiece(t, pieces, src, first)
	}
	cal, err := r.diffCalibrate(t, first)
	if err != nil {
		return nil, err
	}
	if cal == nil {
		return r.glamourPiece(t, pieces, src, first)
	}
	pane := 0
	if t == &r.outer {
		pane = r.pane
	}
	lines, err := r.diffLines(cal, body, "", pane, true)
	if err != nil {
		return nil, err
	}
	all := make([]string, 0, len(cal.head)+len(lines)+len(cal.tail))
	all = append(all, cal.head...)
	all = append(all, lines...)
	all = append(all, cal.tail...)
	return append(pieces, piece{text: strings.Join(all, "\n")}), nil
}

// diffTitled draws the code lines of a titled diff fence at content width
// w (directive.Ctx.Diff): glamour's code indent, no pad but the tint's.
func (r *DirectiveRenderer) diffTitled(body, title string, w int) ([]string, error) {
	if body == "" {
		return nil, nil
	}
	t, err := r.innerTarget(w)
	if err != nil {
		return nil, err
	}
	cal, err := r.diffCalibrate(t, true)
	if err != nil {
		return nil, err
	}
	if cal == nil {
		// No code line shape to read (a style glamour draws code
		// without one): no lead, the whole width.
		if t.dfall == nil {
			t.dfall = &diffCal{area: w}
		}
		cal = t.dfall
	}
	return r.diffLines(cal, body, title, 0, false)
}

// diffCalibrate reads cal from glamour's render of a one-line fence in t;
// nil when the render does not have one line of code in it.
func (r *DirectiveRenderer) diffCalibrate(t *target, first bool) (*diffCal, error) {
	k := 0
	if first {
		k = 1
	}
	if c := t.dcal[k]; c != nil {
		if c.bad {
			return nil, nil
		}
		return c, nil
	}
	out, err := t.g.Render(join.Input("```"+plainLang+"\nx\n```\n", first))
	if err != nil {
		return nil, err
	}
	p, err := join.Strip(out, t.p, t.s)
	if err != nil {
		return nil, err
	}
	c := readCal(p)
	if c == nil {
		t.dcal[k] = &diffCal{bad: true}
		return nil, nil
	}
	t.dcal[k] = c
	return c, nil
}

// readCal reads a diffCal from piece p of a one-line fence; nil when p
// does not have that shape.
func readCal(p string) *diffCal {
	lines := strings.Split(p, "\n")
	at := -1
	for i, l := range lines {
		if strings.TrimSpace(xansi.Strip(l)) != "" {
			if at >= 0 {
				return nil
			}
			at = i
		}
	}
	if at < 0 {
		return nil
	}
	l := lines[at]
	lead := padRunAt(l, true)
	trail := padRunAt(l[len(lead):], false)
	mid := l[len(lead) : len(l)-len(trail)]
	if xansi.Strip(mid) != "x" || trail == "" {
		return nil
	}
	cell := trail[len(trail)-padUnit(trail, false):]
	return &diffCal{head: lines[:at], tail: lines[at+1:], lead: lead, cell: cell, area: xansi.StringWidth(l) - xansi.StringWidth(lead)}
}

// padRunAt returns the run of blank cells (plain spaces, or glamour's
// styled pad cells "ESC[..m ESC[m") at the start of s (front) or its end.
func padRunAt(s string, front bool) string {
	n := 0
	for {
		rest := s[:len(s)-n]
		if front {
			rest = s[n:]
		}
		k := padUnit(rest, front)
		if k == 0 {
			break
		}
		n += k
	}
	if front {
		return s[:n]
	}
	return s[len(s)-n:]
}

// padUnit is the byte length of one blank cell at the start (front) or end
// of s, 0 when there is none.
func padUnit(s string, front bool) int {
	if front {
		if strings.HasPrefix(s, " ") {
			return 1
		}
		if m := styledCellRe.FindString(s); m != "" {
			return len(m)
		}
		return 0
	}
	if strings.HasSuffix(s, " ") {
		return 1
	}
	if m := styledCellEndRe.FindString(s); m != "" {
		return len(m)
	}
	return 0
}

var (
	styledCellRe    = regexp.MustCompile(`^\x1b\[[0-9;]*m \x1b\[m`)
	styledCellEndRe = regexp.MustCompile(`\x1b\[[0-9;]*m \x1b\[m$`)
)

// diffLines draws the code lines of a diff body. pane > 0 (WideBlocks, top
// level): a line wider than the code block keeps its cells up to the pane
// and gets the edge note there; else it is cut at the code block with "…".
// pad: pad context and header lines to the code block with glamour's pad
// cells (a titled fence leaves that to the form frame).
func (r *DirectiveRenderer) diffLines(cal *diffCal, body, title string, pane int, pad bool) ([]string, error) {
	src := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	kinds := make([]diffview.Kind, len(src))
	for i, l := range src {
		kinds[i] = diffview.Classify(l)
	}
	pairs := diffview.Pairs(kinds)
	th := r.theme
	var lexer chroma.Lexer
	if title != "" && !th.Plain {
		lexer = diffLexers.get(title)
	}
	out := make([]string, len(src))
	for i, l := range src {
		// A drawn line depends on its source, its pair's source, the
		// lexer and the target: memoized on those, so a frame costs a map
		// lookup per line that did not change.
		k := lineKey{cal: cal, lexer: lexer, pane: pane, pad: pad, line: l}
		if j := pairs[i]; j >= 0 && !th.Plain {
			k.paired, k.partner = true, src[j]
		}
		if s, ok := r.diff.lines[k]; ok {
			out[i] = s
			continue
		}
		marker, code := splitMarker(kinds[i], l)
		var marks []diffview.Range
		if k.paired {
			_, other := splitMarker(kinds[pairs[i]], k.partner)
			if kinds[i] == diffview.Removed {
				marks, _ = diffview.Words(code, other)
			} else {
				_, marks = diffview.Words(other, code)
			}
		}
		line, err := r.diffLine(cal, r.diffTable(), lexer, kinds[i], marker, code, marks, pane, pad)
		if err != nil {
			return nil, err
		}
		if r.diff.lines == nil || len(r.diff.lines) >= diffCacheMax {
			r.diff.lines = map[lineKey]string{}
		}
		k.line, k.partner = strings.Clone(k.line), strings.Clone(k.partner)
		r.diff.lines[k] = line
		out[i] = line
	}
	return out, nil
}

// lineKey is what one drawn diff line depends on.
type lineKey struct {
	cal           *diffCal
	lexer         chroma.Lexer
	pane          int
	pad, paired   bool
	line, partner string
}

// splitMarker cuts a line into its one-character marker (added, removed,
// or a context line's space) and its code, tabs expanded.
func splitMarker(k diffview.Kind, l string) (marker, code string) {
	if k == diffview.Added || k == diffview.Removed || strings.HasPrefix(l, " ") {
		return l[:1], expandTabs(l[1:])
	}
	return "", expandTabs(l)
}

// diffLine draws one line: lead, marker and code (tinted for added and
// removed lines, faint for headers), pad.
func (r *DirectiveRenderer) diffLine(cal *diffCal, tab map[chroma.TokenType]string, lexer chroma.Lexer, k diffview.Kind, marker, code string, marks []diffview.Range, pane int, pad bool) (string, error) {
	th := r.theme
	var bg, strong uint8
	switch k {
	case diffview.Added:
		bg, strong = th.Diff.Added, th.Diff.AddedWord
	case diffview.Removed:
		bg, strong = th.Diff.Removed, th.Diff.RemovedWord
	}
	mw := xansi.StringWidth(marker)
	cells := mw + xansi.StringWidth(code)
	cut := pane == 0 && cells > cal.area
	if cut {
		code = cutCells(code, max(cal.area-mw-1, 0))
		cells = mw + xansi.StringWidth(code) + 1
	}
	var b strings.Builder
	b.WriteString(cal.lead)
	switch {
	case th.Plain:
		b.WriteString(marker)
		b.WriteString(code)
		if cut {
			b.WriteString("…")
		}
	case !k.Code():
		text := code
		if cut {
			text += "…"
		}
		b.WriteString(th.Muted.Render(text))
	default:
		bgEsc := ""
		if bg != 0 {
			bgEsc = "\x1b[48;5;" + strconv.Itoa(int(bg)) + "m"
		}
		if marker != "" {
			mt := chroma.Text
			switch k {
			case diffview.Added:
				mt = chroma.GenericInserted
			case diffview.Removed:
				mt = chroma.GenericDeleted
			}
			seg(&b, escapeOf(tab, mt)+bgEsc, marker)
		}
		toks, err := r.lexLine(lexer, code)
		if err != nil {
			return "", err
		}
		strongEsc := "\x1b[48;5;" + strconv.Itoa(int(strong)) + "m"
		at, mi := 0, 0
		for _, tk := range toks {
			end := at + tk.n
			fg := escapeOf(tab, tk.typ)
			for at < end {
				// Split the token at word mark edges.
				for mi < len(marks) && marks[mi].To <= at {
					mi++
				}
				stop, esc := end, bgEsc
				if mi < len(marks) && marks[mi].From <= at {
					stop, esc = min(end, marks[mi].To), strongEsc
				} else if mi < len(marks) && marks[mi].From < end {
					stop = marks[mi].From
				}
				seg(&b, fg+esc, code[at:stop])
				at = stop
			}
		}
		if cut {
			seg(&b, escapeOf(tab, chroma.Text)+bgEsc, "…")
		}
		if bg != 0 && cells < cal.area {
			// "ESC[0m", not "ESC[m": inner's pad strip (padRe) keeps the
			// tint in a form body.
			b.WriteString(bgEsc + strings.Repeat(" ", cal.area-cells) + "\x1b[0m")
			cells = cal.area
		}
	}
	if pad && cells < cal.area {
		b.WriteString(strings.Repeat(cal.cell, cal.area-cells))
	}
	line := b.String()
	if pane > 0 && cells > cal.area {
		line = r.cutLine(line)
	}
	return line, nil
}

// seg writes text in esc, closed; nothing for empty text. An empty esc
// writes the text bare.
func seg(b *strings.Builder, esc, text string) {
	if text == "" {
		return
	}
	if esc == "" {
		b.WriteString(text)
		return
	}
	b.WriteString(esc)
	b.WriteString(text)
	b.WriteString("\x1b[0m")
}

// lexLine returns the tokens of one line of code (no "\n"): one Text token
// with no lexer. Memoized per renderer.
func (r *DirectiveRenderer) lexLine(lexer chroma.Lexer, code string) (toks []tok, err error) {
	if code == "" {
		return nil, nil
	}
	if lexer == nil {
		return []tok{{chroma.Text, len(code)}}, nil
	}
	k := tokKey{lexer, code}
	if ts, ok := r.diff.toks[k]; ok {
		return ts, nil
	}
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("bubbleteandmd: chroma panic: %v", p)
		}
	}()
	it, err := lexer.Tokenise(nil, code+"\n")
	if err != nil {
		return nil, err
	}
	n := 0
	for t := it(); t != chroma.EOF; t = it() {
		v := strings.TrimRight(t.Value, "\n")
		if n+len(v) > len(code) {
			v = v[:len(code)-n]
		}
		if v == "" {
			continue
		}
		if c := len(toks); c > 0 && toks[c-1].typ == t.Type {
			toks[c-1].n += len(v)
		} else {
			toks = append(toks, tok{t.Type, len(v)})
		}
		n += len(v)
		if n == len(code) {
			break
		}
	}
	if n < len(code) {
		// The lexer dropped or changed bytes: the rest as text.
		toks = append(toks, tok{chroma.Text, len(code) - n})
	}
	if r.diff.toks == nil || len(r.diff.toks) >= diffCacheMax {
		r.diff.toks = map[tokKey][]tok{}
	}
	r.diff.toks[tokKey{lexer, strings.Clone(code)}] = toks
	return toks, nil
}

// diffTable is the escape table of the renderer's code block colours: the
// chroma style glamour builds from the style's CodeBlock (or its named
// theme), per renderer, not the one glamour registers once per process.
func (r *DirectiveRenderer) diffTable() map[chroma.TokenType]string {
	if !r.diff.built {
		r.diff.built = true
		if st := codeChromaStyle(r.style.CodeBlock); st != nil && !r.theme.Plain {
			r.diff.tab = escapeTable(st)
		}
	}
	return r.diff.tab
}

// codeChromaStyle is the chroma style glamour v2.0.1 highlights code
// blocks with (ansi/codeblock.go: the Chroma entries, else the named
// Theme); nil when there is none.
func codeChromaStyle(cb ansi.StyleCodeBlock) *chroma.Style {
	c := cb.Chroma
	if c == nil {
		if cb.Theme == "" {
			return nil
		}
		return chromastyles.Get(cb.Theme)
	}
	entries := chroma.StyleEntries{}
	for tt, p := range map[chroma.TokenType]ansi.StylePrimitive{
		chroma.Text: c.Text, chroma.Error: c.Error, chroma.Comment: c.Comment,
		chroma.CommentPreproc: c.CommentPreproc, chroma.Keyword: c.Keyword,
		chroma.KeywordReserved: c.KeywordReserved, chroma.KeywordNamespace: c.KeywordNamespace,
		chroma.KeywordType: c.KeywordType, chroma.Operator: c.Operator,
		chroma.Punctuation: c.Punctuation, chroma.Name: c.Name,
		chroma.NameBuiltin: c.NameBuiltin, chroma.NameTag: c.NameTag,
		chroma.NameAttribute: c.NameAttribute, chroma.NameClass: c.NameClass,
		chroma.NameConstant: c.NameConstant, chroma.NameDecorator: c.NameDecorator,
		chroma.NameException: c.NameException, chroma.NameFunction: c.NameFunction,
		chroma.NameOther: c.NameOther, chroma.Literal: c.Literal,
		chroma.LiteralNumber: c.LiteralNumber, chroma.LiteralDate: c.LiteralDate,
		chroma.LiteralString: c.LiteralString, chroma.LiteralStringEscape: c.LiteralStringEscape,
		chroma.GenericDeleted: c.GenericDeleted, chroma.GenericEmph: c.GenericEmph,
		chroma.GenericInserted: c.GenericInserted, chroma.GenericStrong: c.GenericStrong,
		chroma.GenericSubheading: c.GenericSubheading, chroma.Background: c.Background,
	} {
		entries[tt] = chromaEntry(p)
	}
	st, err := chroma.NewStyle("bubbleteandmd-diff", entries)
	if err != nil {
		return nil
	}
	return st
}

// chromaEntry is glamour's chromaStyle: a chroma style entry string.
func chromaEntry(p ansi.StylePrimitive) string {
	var parts []string
	if p.Color != nil {
		parts = append(parts, *p.Color)
	}
	if p.BackgroundColor != nil {
		parts = append(parts, "bg:"+*p.BackgroundColor)
	}
	if p.Italic != nil && *p.Italic {
		parts = append(parts, "italic")
	}
	if p.Bold != nil && *p.Bold {
		parts = append(parts, "bold")
	}
	if p.Underline != nil && *p.Underline {
		parts = append(parts, "underline")
	}
	return strings.Join(parts, " ")
}

// lexerCache maps a file name's extension (or the name, when it has none)
// to chroma's lexer for it, nil for none: lexers.Match scans every lexer's
// file globs.
type lexerCache struct {
	mu sync.Mutex
	m  map[string]chroma.Lexer
}

var diffLexers = &lexerCache{m: map[string]chroma.Lexer{}}

func (c *lexerCache) get(title string) chroma.Lexer {
	base := path.Base(strings.ReplaceAll(title, `\`, "/"))
	key := strings.ToLower(path.Ext(base))
	name := "x" + key
	if key == "" {
		key, name = "name:"+base, base
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.m[key]
	if !ok {
		if l = lexers.Match(name); l != nil {
			if cfg := l.Config(); cfg != nil && strings.EqualFold(cfg.Name, "plaintext") {
				l = nil
			} else {
				l = chroma.Coalesce(l)
			}
		}
		if len(c.m) < 1024 {
			c.m[key] = l
		}
	}
	return l
}

// expandTabs replaces each tab with spaces to the next multiple of 4
// columns, counted in s.
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	if ascii(s) {
		for i := 0; i < len(s); i++ {
			if s[i] == '\t' {
				n := 4 - col%4
				b.WriteString("    "[:n])
				col += n
				continue
			}
			b.WriteByte(s[i])
			col++
		}
		return b.String()
	}
	for _, g := range graphemes(s) {
		if g == "\t" {
			n := 4 - col%4
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteString(g)
		col += xansi.StringWidth(g)
	}
	return b.String()
}

func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func graphemes(s string) []string {
	var out []string
	for s != "" {
		g, _ := xansi.FirstGraphemeCluster(s, xansi.GraphemeWidth)
		if g == "" {
			g = s[:1]
		}
		out = append(out, g)
		s = s[len(g):]
	}
	return out
}

// cutCells is the longest prefix of s (plain text) that is at most n cells.
func cutCells(s string, n int) string {
	cells, i := 0, 0
	for i < len(s) {
		g, w := xansi.FirstGraphemeCluster(s[i:], xansi.GraphemeWidth)
		if g == "" {
			g = s[i : i+1]
		}
		if cells+w > n {
			break
		}
		cells += w
		i += len(g)
	}
	return s[:i]
}
