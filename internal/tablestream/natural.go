package tablestream

import (
	"strings"

	"charm.land/glamour/v2/ansi"
)

// Natural measures the width a growing table block takes when no column is
// shrunk or widened: each column as wide as its widest cell plus glamour's
// cell frame, with a column separator between columns. It uses the width
// model's cell widths (glamour's own elements, "[k]" for footer links), so
// a Renderer that wraps at this width plus the document margins draws the
// table at its natural column widths.
//
// Rows that end in "\n" are parsed once and kept while src starts with the
// same head and rows. A Natural is not safe for concurrent use.
type Natural struct {
	t    *Table
	head string
	hrow row
	body string // complete row lines after head, each with "\n"
	rows []row
}

// NewNatural returns a Natural for a Renderer made with opts.
func NewNatural(opts ansi.Options) *Natural { return &Natural{t: New(opts)} }

// Width returns the natural table width of the table block src (trailing
// blank lines allowed). ok is false when src is not one table that starts
// with a pipe row, or a row has something the width model does not read
// (rowLine).
func (n *Natural) Width(src string) (int, bool) {
	src = strings.TrimRight(src, "\n")
	if n.head == "" || !strings.HasPrefix(src, n.head) {
		head, _, _ := split(src + "\n")
		if head == "" {
			return 0, false
		}
		hrow, ok := n.t.parseHeader(head)
		if !ok {
			return 0, false
		}
		*n = Natural{t: n.t, head: head, hrow: hrow}
	}
	body := src[min(len(n.head), len(src)):]
	if !strings.HasPrefix(body, n.body) {
		n.body, n.rows = "", nil
	}
	rest := body[len(n.body):]
	// done: the complete lines of rest (a "\n" after them).
	done := rest[:strings.LastIndexByte(rest, '\n')+1]
	if done != "" {
		rows, ok := n.parse(done)
		if !ok {
			return 0, false
		}
		n.body += done
		n.rows = append(n.rows, rows...)
	}
	rows := n.rows
	if last := rest[len(done):]; last != "" {
		lr, ok := n.parse(last + "\n")
		if !ok {
			return 0, false
		}
		rows = append(rows[:len(rows):len(rows)], lr...)
	}
	var links, images []linkKey
	cells := [][]int{n.hrow.resolve(&links, &images)}
	for _, r := range rows {
		cells = append(cells, r.resolve(&links, &images))
	}
	return naturalWidth(cells), true
}

// parse reads lines (each ending in "\n") as rows of the table at n.head.
func (n *Natural) parse(lines string) ([]row, bool) {
	ls := strings.Split(strings.TrimSuffix(lines, "\n"), "\n")
	for _, l := range ls {
		if !rowLine(l) {
			return nil, false
		}
	}
	rows, ok := n.t.parseRows(n.head + strings.TrimSuffix(lines, "\n"))
	if !ok || len(rows) != len(ls) {
		return nil, false
	}
	return rows, true
}

// naturalWidth is the table width modelWidths starts from: the header and
// every row, each column its widest cell plus the frame, and the borders.
// Columns are the data's, as in modelWidths (the header is padded to them).
func naturalWidth(cells [][]int) int {
	const frame = 2
	cols := 0
	for _, r := range cells[1:] {
		cols = max(cols, len(r))
	}
	cols = max(cols, len(cells[0]))
	ws := make([]int, cols)
	for _, r := range cells {
		for i, w := range r {
			ws[i] = max(ws[i], w)
		}
	}
	total := cols - 1
	for _, w := range ws {
		total += w + frame
	}
	return total
}
