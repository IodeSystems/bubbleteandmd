// Package tablestream renders a growing GFM table block in less time than a
// whole render, with the same bytes.
//
// The rule (internal/spike, N3): when the column widths do not change as rows
// are added, glamour's render of the table with n+1 rows starts with its
// render of n rows minus the last line. The rendered lines of a row depend
// only on the widths and the row (and the numbers of its footer links). So
// Table keeps the lines of the rows before the last one, and renders new rows
// in a mini table through the same Renderer:
//
//	header, carrier rows, forcing rows, new rows [, a 'y' forcing row]
//
// Forcing rows have cells that fill each column exactly ('x' runs), so
// lipgloss keeps the widths (max and median). Carrier rows hold every footer
// link and image of the committed rows, in order, padded to the same exact
// widths, so the links of the new rows get the numbers and the footer list
// they get in the whole table. The mini output must start with the cached
// header, separator, carrier and forcing lines, byte for byte, or the call
// falls back to a whole render.
//
// Whether the widths of the whole table change is computed with lipgloss's own
// resizer from each cell's display width (width model). Cell widths come from
// glamour's own elements (ansi.NewElement), plus "[k]" for each footer link.
// The model is checked against glamour's separator at every whole render; a
// mismatch turns the fast path off for that Table.
package tablestream

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/nest"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	astext "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// Renderer renders one whole Markdown document (*glamour.TermRenderer).
type Renderer interface {
	Render(string) (string, error)
}

// Stats counts how calls were served.
type Stats struct {
	// Whole: calls answered by one whole render.
	Whole int
	// Spliced: calls answered from cached rows and mini renders.
	Spliced int
	// MiniFail: a mini render did not match the cache (whole render).
	MiniFail int
}

// Table renders one growing table block. Use one Table per block: once its
// width model fails, it renders whole for the rest of its life.
// A Table is not safe for concurrent use.
type Table struct {
	// Stats is updated on every call.
	Stats Stats

	opts ansi.Options
	tr   *ansi.ANSIRenderer
	md   goldmark.Markdown

	off      bool   // whole renders only
	failHead string // setup failed for this head: whole renders while it stays
	failAt   int    // ... and src is shorter than this (then setup runs again)

	ok   bool   // the state below is valid
	head string // src up to and including the delimiter row's "\n"
	base string // head + committed rows, each with its "\n"
	// tableWidth: lipgloss table width, sum(widths)+ncol-1, read once.
	tableWidth int
	widths     []int
	headCells  []int // display widths of the header cells

	pre    string // output up to and including the separator line
	fpad   string // an 'x' forcing row's line, not last (with "\n")
	ypad   string // a 'y' forcing row's line, not last (with "\n")
	forceX string
	forceY string

	links, images []linkKey // footer lists of header + committed rows
	carriers      string    // carrier row sources, each with "\n"
	nCarriers     int
	carrierOut    string // their lines

	cells   [][]int // display widths of committed rows' cells
	rowsOut string  // rendered lines of committed rows

	lastKey string // source of the last row in lastRem
	lastRem string // rendered last row and all that follows
}

// New returns a Table for a Renderer made with opts (Styles, and the
// table options; WordWrap is read from the output).
func New(opts ansi.Options) *Table {
	t := &Table{opts: opts, tr: ansi.NewRenderer(opts)}
	t.md = nest.New() // as the Renderer parses (internal/nest)
	if opts.InlineTableLinks || (opts.TableWrap != nil && !*opts.TableWrap) {
		t.off = true
	}
	return t
}

// Render returns r.Render(src), byte-equal. r must render with the options
// given to New.
func (t *Table) Render(src string, r Renderer) (string, error) {
	if t.off || (t.failHead != "" && len(src) < t.failAt && strings.HasPrefix(src, t.failHead)) {
		return t.whole(src, r)
	}
	t.failHead = ""
	if !t.ok || !strings.HasPrefix(src, t.base) {
		return t.rebase(src, r)
	}
	body := src[len(t.base):]
	if body == "" || body == "\n" {
		return t.rebase(src, r)
	}
	nl := strings.HasSuffix(body, "\n")
	newLines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	for _, l := range newLines {
		if !rowLine(l) {
			t.ok = false
			return t.whole(src, r)
		}
	}
	rows, ok := t.parseRows(t.head + strings.Join(newLines, "\n"))
	if !ok || len(rows) != len(newLines) {
		t.ok = false
		return t.whole(src, r)
	}
	links, images := clone(t.links), clone(t.images)
	all := append(make([][]int, 0, len(t.cells)+len(rows)), t.cells...)
	for _, row := range rows {
		all = append(all, row.resolve(&links, &images))
	}
	if !equalInts(modelWidths(t.headCells, all, t.tableWidth), t.widths) {
		return t.rebase(src, r)
	}
	lastLine, lastCells := newLines[len(newLines)-1], all[len(all)-1]
	if n := len(newLines) - 1; n > 0 {
		had := len(t.cells)
		ok, err := t.commit(newLines[:n], all[had:had+n], rows[:n], r)
		if err != nil {
			return "", err
		}
		if !ok {
			t.Stats.MiniFail++
			if len(t.cells) != had {
				t.ok = false
				return t.whole(src, r)
			}
			// Take the rows from the whole render instead.
			return t.commitWhole(src, newLines[:n], all[had:had+n], rows[:n], lastLine, lastCells, nl, r)
		}
	}
	rem, ok, err := t.renderLast(lastLine, lastCells, nl, r)
	if err != nil {
		return "", err
	}
	if !ok {
		t.Stats.MiniFail++
		return t.whole(src, r)
	}
	t.Stats.Spliced++
	return t.pre + t.rowsOut + rem, nil
}

// commitWhole renders src whole and caches the rows before the last one
// from it: what lies between the cached rows and the last row's render.
func (t *Table) commitWhole(src string, lines []string, cells [][]int, rows []row, last string, lastCells []int, nl bool, r Renderer) (string, error) {
	out, err := t.whole(src, r)
	if err != nil {
		return "", err
	}
	t.ok = false
	head := t.pre + t.rowsOut
	if !strings.HasPrefix(out, head) {
		return out, nil
	}
	// The last row's render needs the carriers of the rows before it, and
	// the footer numbers they take: add first, then render the last row.
	ok, err := t.add(lines, cells, rows, "", r)
	if err != nil {
		return "", err
	}
	if !ok {
		return out, nil
	}
	rem, ok, err := t.renderLast(last, lastCells, nl, r)
	if err != nil {
		return "", err
	}
	if !ok || len(out) < len(head)+len(rem) || !strings.HasSuffix(out, rem) {
		return out, nil
	}
	t.rowsOut += out[len(head) : len(out)-len(rem)]
	t.ok = true
	return out, nil
}

func (t *Table) whole(src string, r Renderer) (string, error) {
	t.Stats.Whole++
	return r.Render(src)
}

// rebase renders src whole and, when src is a table block the fast path can
// serve, keeps its rows for the next calls.
func (t *Table) rebase(src string, r Renderer) (string, error) {
	t.ok = false
	out, err := t.whole(src, r)
	if err != nil {
		return "", err
	}
	if err := t.setup(src, out, r); err != nil {
		return "", err
	}
	return out, nil
}

// setup reads the table out of src and its whole render out. It leaves t.ok
// false when any check fails.
func (t *Table) setup(src, out string, r Renderer) error {
	head, ncol, ok := split(src)
	if !ok {
		return nil
	}
	// A failed setup costs mini renders: try again only after retryBytes more.
	fail := func() error { t.failHead, t.failAt = head, len(src)+retryBytes; return nil }
	body := src[len(head):]
	nl := strings.HasSuffix(body, "\n")
	lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	for _, l := range lines {
		if !rowLine(l) {
			return fail()
		}
	}
	hrow, ok := t.parseHeader(head)
	if !ok {
		return fail()
	}
	rows, ok := t.parseRows(src)
	if !ok || len(rows) != len(lines) {
		return fail()
	}
	var links, images []linkKey
	hcells := hrow.resolve(&links, &images)
	committed := len(lines) - 1
	cells := make([][]int, len(rows))
	nl0, ni0 := len(links), len(images) // list lengths after the committed rows
	for i, row := range rows {
		cells[i] = row.resolve(&links, &images)
		if i == committed-1 {
			nl0, ni0 = len(links), len(images)
		}
	}
	committedLinks, committedImages := clone(links[:nl0]), clone(images[:ni0])
	// The widths glamour drew: the first line that parses as a separator.
	widths, ok := separatorWidths(out, ncol)
	if !ok {
		return fail()
	}
	if t.tableWidth == 0 {
		t.tableWidth = sum(widths) + ncol - 1
	}
	if !equalInts(modelWidths(hcells, cells, t.tableWidth), widths) {
		t.off = true
		return nil
	}
	*t = Table{Stats: t.Stats, opts: t.opts, tr: t.tr, md: t.md, tableWidth: t.tableWidth,
		head: head, widths: widths, headCells: hcells}
	t.forceX, t.forceY = forcing('x', widths), forcing('y', widths)
	if !t.miniKeepsWidths(nil, 2) {
		return fail()
	}
	bx, err := r.Render(head + t.forceX + "\n" + t.forceX)
	if err != nil {
		return err
	}
	by, err := r.Render(head + t.forceY + "\n" + t.forceY)
	if err != nil {
		return err
	}
	// The first line where they differ is the first forcing row.
	p := 0
	for p < len(bx) && p < len(by) && bx[p] == by[p] {
		p++
	}
	p = strings.LastIndexByte(bx[:p], '\n') + 1
	if p == 0 || !strings.HasPrefix(out, bx[:p]) || !strings.HasPrefix(by, bx[:p]) {
		return fail()
	}
	e := strings.IndexByte(bx[p:], '\n')
	ey := strings.IndexByte(by[p:], '\n')
	if e < 0 || ey < 0 {
		return fail()
	}
	t.pre, t.fpad, t.ypad = bx[:p], bx[p:p+e+1], by[p:p+ey+1]
	t.links, t.images = committedLinks, committedImages
	if ok, err := t.setCarriers(hrow, r); err != nil || !ok {
		if err != nil {
			return err
		}
		return fail()
	}
	rem, ok, err := t.renderLast(lines[committed], cells[committed], nl, r)
	if err != nil {
		return err
	}
	if !ok || len(out) < len(t.pre)+len(rem) || !strings.HasSuffix(out, rem) {
		return fail()
	}
	t.rowsOut = out[len(t.pre) : len(out)-len(rem)]
	t.cells = cells[:committed]
	t.base = strings.Clone(head + joinRows(lines[:committed]))
	t.ok = true
	return nil
}

// commit renders rows that are not last (all but the last line of the body)
// and adds them to the cache.
func (t *Table) commit(lines []string, cells [][]int, rows []row, r Renderer) (bool, error) {
	// The rows end where the 'y' row's line starts; an 'x' row after it keeps
	// it from being last (a last line is padded by what follows the table).
	f := max(1, len(lines)+2-t.nCarriers)
	if !t.miniKeepsWidths(cells, f+2) {
		return false, nil
	}
	var b strings.Builder
	b.WriteString(t.head)
	b.WriteString(t.carriers)
	for range f {
		b.WriteString(t.forceX)
		b.WriteByte('\n')
	}
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString(t.forceY + "\n" + t.forceX)
	out, err := r.Render(b.String())
	if err != nil {
		return false, err
	}
	rest, ok := t.cutHead(out, f)
	if !ok {
		return false, nil
	}
	at := strings.LastIndex("\n"+rest, "\n"+t.ypad)
	if at < 0 {
		return false, nil
	}
	return t.add(lines, cells, rows, rest[:at], r)
}

// add puts rows, rendered as out, in the cache.
func (t *Table) add(lines []string, cells [][]int, rows []row, out string, r Renderer) (bool, error) {
	nLinks, nImages := len(t.links), len(t.images)
	for _, row := range rows {
		row.resolve(&t.links, &t.images)
	}
	t.rowsOut += out
	t.cells = append(t.cells, cells...)
	t.base += joinRows(lines)
	if len(t.links) != nLinks || len(t.images) != nImages {
		hrow, ok := t.parseHeader(t.head)
		if !ok {
			return false, nil
		}
		return t.setCarriers(hrow, r)
	}
	return true, nil
}

// renderLast renders the last row and what follows the table.
func (t *Table) renderLast(last string, cells []int, nl bool, r Renderer) (string, bool, error) {
	key := last
	if nl {
		key += "\n"
	}
	if t.lastRem != "" && key == t.lastKey {
		return t.lastRem, true, nil
	}
	f := max(1, 2-t.nCarriers)
	if !t.miniKeepsWidths([][]int{cells}, f) {
		return "", false, nil
	}
	var b strings.Builder
	b.WriteString(t.head)
	b.WriteString(t.carriers)
	for range f {
		b.WriteString(t.forceX)
		b.WriteByte('\n')
	}
	b.WriteString(key)
	out, err := r.Render(b.String())
	if err != nil {
		return "", false, err
	}
	rest, ok := t.cutHead(out, f)
	if !ok {
		return "", false, nil
	}
	t.lastKey, t.lastRem = key, rest
	return rest, true, nil
}

// miniKeepsWidths: the model gives the cached widths to a mini table of the
// header, the carriers and f forcing rows (all cells exact), and rows. Its
// header and separator lines cannot tell: lipgloss cuts lines at the table
// width, so a mini table drawn wider shows the same separator.
func (t *Table) miniKeepsWidths(rows [][]int, f int) bool {
	force := make([]int, len(t.widths))
	for j, w := range t.widths {
		force[j] = max(w-2, 0)
	}
	all := make([][]int, 0, t.nCarriers+f+len(rows))
	for range t.nCarriers + f {
		all = append(all, force)
	}
	all = append(all, rows...)
	return equalInts(modelWidths(t.headCells, all, t.tableWidth), t.widths)
}

// cutHead checks that out starts with the cached header, carrier lines and n
// forcing rows, and returns what follows them.
func (t *Table) cutHead(out string, n int) (string, bool) {
	if !strings.HasPrefix(out, t.pre) {
		return "", false
	}
	rest := out[len(t.pre):]
	if !strings.HasPrefix(rest, t.carrierOut) {
		return "", false
	}
	rest = rest[len(t.carrierOut):]
	for range n {
		if !strings.HasPrefix(rest, t.fpad) {
			return "", false
		}
		rest = rest[len(t.fpad):]
	}
	return rest, true
}

// setCarriers builds carrier rows for the footer links and images of the
// committed rows (the lists past the header's own), checks that they parse
// back to the same links at the exact widths, and renders them once.
func (t *Table) setCarriers(hrow row, r Renderer) (bool, error) {
	var hl, hi []linkKey
	hrow.resolve(&hl, &hi)
	var items []string
	var keys []linkKey
	for _, k := range t.links[len(hl):] {
		items, keys = append(items, k.source()), append(keys, k)
	}
	for _, k := range t.images[len(hi):] {
		items, keys = append(items, k.source()), append(keys, k)
	}
	t.carriers, t.nCarriers, t.carrierOut, t.lastRem = "", 0, "", ""
	if len(items) == 0 {
		return true, nil
	}
	// Measure each item alone: one row per item, item in the first cell.
	var m strings.Builder
	m.WriteString(t.head)
	for _, it := range items {
		m.WriteString("| " + it + " |\n")
	}
	mrows, ok := t.parseRows(m.String())
	if !ok || len(mrows) != len(items) {
		return false, nil
	}
	// Numbers: links after the header's, images after the header's.
	links, images := clone(hl), clone(hi)
	capOf := func(j int) int { return max(t.widths[j]-2, 0) }
	widest := 0
	for j := range t.widths {
		widest = max(widest, capOf(j))
	}
	var b strings.Builder
	var cells []string
	flush := func() {
		for len(cells) < len(t.widths) {
			cells = append(cells, strings.Repeat("x", capOf(len(cells))))
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		t.nCarriers++
		cells = nil
	}
	// Items go left to right, top to bottom, in list order; a column too
	// narrow for the next item is filled with 'x'.
	for i, it := range items {
		mr := mrows[i]
		if len(mr.refs) != 1 || mr.refs[0].key != keys[i] {
			return false, nil
		}
		w := mr.resolve(&links, &images)[0]
		if w > widest {
			return false, nil
		}
		for {
			if len(cells) == len(t.widths) {
				flush()
			}
			c := capOf(len(cells))
			if w > c {
				cells = append(cells, strings.Repeat("x", c))
				continue
			}
			switch rest := c - w; {
			case rest == 1:
				it += "x"
			case rest > 1:
				it += " " + strings.Repeat("x", rest-1)
			}
			cells = append(cells, it)
			break
		}
	}
	if len(cells) > 0 {
		flush()
	}
	t.carriers = b.String()
	// The carriers must parse to the same links, at the exact widths.
	crows, ok := t.parseRows(t.head + strings.TrimSuffix(t.carriers, "\n"))
	if !ok || len(crows) != t.nCarriers {
		return false, nil
	}
	links, images = clone(hl), clone(hi)
	for _, cr := range crows {
		ws := cr.resolve(&links, &images)
		for j, w := range ws {
			if w != max(t.widths[j]-2, 0) {
				return false, nil
			}
		}
	}
	if !equalKeys(links, t.links) || !equalKeys(images, t.images) {
		return false, nil
	}
	// Their lines: render header + carriers + x + y; the carrier lines are
	// the nCarriers lines after the header.
	out, err := r.Render(t.head + t.carriers + t.forceX + "\n" + t.forceY + "\n" + t.forceX)
	if err != nil {
		return false, err
	}
	if !strings.HasPrefix(out, t.pre) {
		return false, nil
	}
	rest := out[len(t.pre):]
	at := 0
	for range t.nCarriers {
		e := strings.IndexByte(rest[at:], '\n')
		if e < 0 {
			return false, nil
		}
		at += e + 1
	}
	if !strings.HasPrefix(rest[at:], t.fpad+t.ypad) {
		return false, nil
	}
	t.carrierOut = rest[:at]
	return true, nil
}

func joinRows(rows []string) string {
	if len(rows) == 0 {
		return ""
	}
	return strings.Join(rows, "\n") + "\n"
}

// forcing returns a row whose cells fill each column of widths exactly
// (a cell has one cell of margin on each side).
func forcing(ch byte, widths []int) string {
	var b strings.Builder
	b.WriteByte('|')
	for _, w := range widths {
		b.WriteByte(' ')
		b.WriteString(strings.Repeat(string(ch), max(w-2, 0)))
		b.WriteString(" |")
	}
	return b.String()
}

// split finds the table: src is a prefix that ends with a blank line (or
// nothing), a header row, a delimiter row with as many cells, and rows.
func split(src string) (head string, ncol int, ok bool) {
	at := 0
	for {
		line, next, found := lineAt(src, at)
		if !found {
			return "", 0, false
		}
		if strings.HasPrefix(line, "|") && (at == 0 || strings.HasSuffix(src[:at], "\n\n")) {
			delim, after, found := lineAt(src, next)
			if found && strings.HasSuffix(src[:after], "\n") {
				if n, isDelim := delimiterCells(delim); isDelim && n == headerCells(line) {
					return src[:after], n, after < len(src)
				}
			}
			return "", 0, false
		}
		at = next
		if at >= len(src) {
			return "", 0, false
		}
	}
}

// lineAt returns the line at offset at and the offset after its "\n".
func lineAt(src string, at int) (line string, next int, ok bool) {
	if at >= len(src) {
		return "", at, false
	}
	e := strings.IndexByte(src[at:], '\n')
	if e < 0 {
		return src[at:], len(src), true
	}
	return src[at : at+e], at + e + 1, true
}

// emojiCode: a goldmark-emoji shortcode; a Renderer made WithEmoji would
// draw it, the width model does not know.
var emojiCode = regexp.MustCompile(`:[a-z0-9_+-]+:`)

// rowLine: a line the fast path takes as a table row. Email autolinks are
// refused: glamour numbers them by a key it does not list (mailto: href).
func rowLine(l string) bool {
	return strings.HasPrefix(l, "|") && !strings.Contains(l, "@") && !emojiCode.MatchString(l)
}

// headerCells counts the cells of a header row as GFM does when the row has
// no escaped pipe or code span (anything else fails the delimiter count
// check or the model check later).
func headerCells(l string) int {
	if strings.ContainsAny(l, "\\`") {
		return -1
	}
	s := strings.TrimSpace(l)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "|"), "|")
	return len(strings.Split(s, "|"))
}

// delimiterCells reports whether l is a GFM delimiter row and its cell count.
func delimiterCells(l string) (int, bool) {
	s := strings.TrimSpace(l)
	if !strings.HasPrefix(s, "|") {
		return 0, false
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "|"), "|")
	parts := strings.Split(s, "|")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, ":")
		p = strings.TrimSuffix(p, ":")
		if p == "" || strings.Trim(p, "-") != "" {
			return 0, false
		}
	}
	return len(parts), true
}

// linkKey is glamour's tableLink: footer entries are unique by it.
type linkKey struct {
	href, title, content string
	kind                 int // 1 auto, 2 image, 3 regular (glamour's linkType)
}

// source is Markdown that parses back to k (checked by setCarriers).
func (k linkKey) source() string {
	title := ""
	if k.title != "" {
		title = ` "` + k.title + `"`
	}
	switch k.kind {
	case 1:
		return "<" + k.href + ">"
	case 2:
		return "![" + k.content + "](<" + k.href + ">" + title + ")"
	default:
		return "[" + k.content + "](<" + k.href + ">" + title + ")"
	}
}

// ref is one footer link or image in a cell, in walk order.
type ref struct {
	cell  int
	key   linkKey
	image bool
}

// row is one parsed table row: each cell's display width without the "[k]"
// suffixes, and its footer links.
type row struct {
	widths []int
	refs   []ref
}

// resolve adds the row's links to the footer lists and returns each cell's
// display width, "[k]" suffixes included.
func (rw row) resolve(links, images *[]linkKey) []int {
	ws := append([]int(nil), rw.widths...)
	for _, f := range rw.refs {
		list := links
		if f.image {
			list = images
		}
		k := index(*list, f.key)
		if k < 0 {
			*list = append(*list, f.key)
			k = len(*list) - 1
		}
		ws[f.cell] += len(fmt.Sprintf("[%d]", k+1))
	}
	return ws
}

func index(list []linkKey, k linkKey) int {
	for i, x := range list {
		if x == k {
			return i
		}
	}
	return -1
}

func clone(l []linkKey) []linkKey { return append([]linkKey(nil), l...) }

func equalKeys(a, b []linkKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// parseHeader parses the header row of head.
func (t *Table) parseHeader(head string) (row, bool) {
	tb := t.parse(head)
	if tb == nil {
		return row{}, false
	}
	h, ok := tb.FirstChild().(*astext.TableHeader)
	if !ok {
		return row{}, false
	}
	return t.cellRow(h, []byte(head)), true
}

// parseRows parses src (head and rows) and returns its body rows.
func (t *Table) parseRows(src string) ([]row, bool) {
	tb := t.parse(src)
	if tb == nil {
		return nil, false
	}
	source := []byte(src)
	var rows []row
	for n := tb.FirstChild(); n != nil; n = n.NextSibling() {
		if tr, ok := n.(*astext.TableRow); ok {
			rows = append(rows, t.cellRow(tr, source))
		}
	}
	return rows, true
}

// parse returns the table that is the last node of src, or nil.
func (t *Table) parse(src string) *astext.Table {
	source := []byte(src)
	doc := t.md.Parser().Parse(text.NewReader(source))
	tb, ok := doc.LastChild().(*astext.Table)
	if !ok {
		return nil
	}
	return tb
}

// cellRow measures each cell as glamour's TableCellElement renders it.
func (t *Table) cellRow(n ast.Node, source []byte) row {
	ctx := ansi.NewRenderContext(t.opts)
	doc := ast.NewDocument()
	if e := t.tr.NewElement(doc, source); e.Renderer != nil {
		_ = e.Renderer.Render(&bytes.Buffer{}, ctx)
	}
	style := t.opts.Styles.Table.StylePrimitive
	var rw row
	ci := 0
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		var b bytes.Buffer
		for k := c.FirstChild(); k != nil; k = k.NextSibling() {
			el := t.tr.NewElement(k, source).Renderer
			if el == nil {
				continue
			}
			if so, ok := el.(ansi.StyleOverriderElementRenderer); ok {
				_ = so.StyleOverrideRender(&b, ctx, style)
			} else {
				var bb bytes.Buffer
				_ = el.Render(&bb, ctx)
				_ = (&ansi.BaseElement{Token: bb.String(), Style: style}).Render(&b, ctx)
			}
		}
		rw.widths = append(rw.widths, lipgloss.Width(b.String()))
		cell := ci
		_ = ast.Walk(c, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			switch x := node.(type) {
			case *ast.AutoLink:
				u := string(x.URL(source))
				rw.refs = append(rw.refs, ref{cell: cell, key: linkKey{href: u, content: autoContent(t.tr.NewElement(x, source).Renderer, u), kind: 1}})
			case *ast.Image:
				content := string(nodeContent(x, source))
				if content == "" {
					content = linkDomain(string(x.Destination))
				}
				rw.refs = append(rw.refs, ref{cell: cell, image: true, key: linkKey{href: string(x.Destination), title: string(x.Title), content: content, kind: 2}})
			case *ast.Link:
				rw.refs = append(rw.refs, ref{cell: cell, key: linkKey{href: string(x.Destination), title: string(x.Title), content: string(nodeContent(x, source)), kind: 3}})
			}
			return ast.WalkContinue, nil
		})
		ci++
	}
	return rw
}

// autoContent is the footer text glamour gives an autolink (its shortened
// form or domain): the token of its element, built with no footer list.
func autoContent(el ansi.ElementRenderer, u string) string {
	if le, ok := el.(*ansi.LinkElement); ok && len(le.Children) == 1 {
		if be, ok := le.Children[0].(*ansi.BaseElement); ok {
			return be.Token
		}
	}
	return linkDomain(u)
}

// nodeContent is glamour's (ansi/table_links.go): the text of node's Text
// descendants.
func nodeContent(node ast.Node, source []byte) []byte {
	var b bytes.Buffer
	var walk func(ast.Node)
	walk = func(node ast.Node) {
		for n := node.FirstChild(); n != nil; n = n.NextSibling() {
			if tx, ok := n.(*ast.Text); ok {
				b.Write(tx.Segment.Value(source))
			} else {
				walk(n)
			}
		}
	}
	walk(node)
	return b.Bytes()
}

// linkDomain is glamour's (ansi/table_links.go).
func linkDomain(href string) string {
	if uri, err := url.Parse(href); err == nil {
		return uri.Hostname()
	}
	return "link"
}

// separatorWidths finds glamour's header separator in out: the first line
// that, without escapes and outer spaces, is ncol runs of one character
// joined by another.
func separatorWidths(out string, ncol int) ([]int, bool) {
	for _, line := range strings.Split(out, "\n") {
		rs := []rune(strings.TrimSpace(xansi.Strip(line)))
		if len(rs) < ncol {
			continue
		}
		top, mid := rs[0], rune(0)
		ws := []int{0}
		good := true
		for _, c := range rs {
			switch {
			case c == top:
				ws[len(ws)-1]++
			case mid == 0 || c == mid:
				mid = c
				ws = append(ws, 0)
			default:
				good = false
			}
			if !good {
				break
			}
		}
		if good && len(ws) == ncol && mid != 0 && mid != ' ' && top != ' ' {
			return ws, true
		}
	}
	return nil, false
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

// retryBytes: after a failed setup, how much the block grows before the
// next try.
const retryBytes = 1024
