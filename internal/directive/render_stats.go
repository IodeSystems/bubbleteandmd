package directive

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/join"
)

// A stats form draws as tiles (value over label, side by side) or as a
// list (one stat per line: the value right-aligned in a column, two
// spaces, the label wrapped under its own column). See docs/decisions.md
// "STATS list layout".
const (
	statsGap      = 3  // cells between two tiles
	statsListGap  = 2  // cells between the value column and the label
	statsMaxValue = 12 // the list layout needs every value at most this wide
	statsMinLabel = 10 // and a label column at least this wide
)

type statsLayout int

const (
	statsTiles statsLayout = iota
	statsList
)

// statCell is one stat, rendered once.
type statCell struct {
	val, lab string // styled, one line each (tiles)
	vw, w    int    // value width; tile width
	label    string // label Markdown, wrapped again per column (list)
	wrapped  map[int][]string
}

type statsDraw struct {
	c    Ctx
	head []string
}

func renderStats(d Directive, c Ctx) ([]string, error) {
	s := d.Stats
	if s == nil {
		return nil, nil
	}
	th := c.Theme
	var head []string
	if s.Title != "" {
		t, err := listLines(c, s.Title, c.Width)
		if err != nil {
			return nil, err
		}
		head = listStyle(th.Heading, t)
	}
	if len(s.Items) == 0 {
		return listClamp(head, c.Width), nil
	}
	cells := make([]*statCell, len(s.Items))
	for i, it := range s.Items {
		v, err := statInline(c, it.Value)
		if err != nil {
			return nil, err
		}
		l, err := statInline(c, it.Label)
		if err != nil {
			return nil, err
		}
		ce := &statCell{vw: ansi.StringWidth(v), w: ansi.StringWidth(l), label: it.Label}
		if ce.vw > ce.w {
			ce.w = ce.vw
		}
		if v != "" {
			ce.val = th.Heading.Render(v)
		}
		if l != "" {
			ce.lab = th.Muted.Render(l)
		}
		cells[i] = ce
	}
	sd := statsDraw{c: c, head: head}
	out, err := sd.pick(cells)
	return listClamp(out, c.Width), err
}

// statInline is listInline with blank cells trimmed off each line's ends
// (statTrim), cut to the width.
func statInline(c Ctx, md string) (string, error) {
	lines, err := listLines(c, md, c.Width)
	if err != nil {
		return "", err
	}
	for i, l := range lines {
		lines[i] = statTrim(l)
	}
	return ansi.Truncate(statTrim(strings.Join(lines, " ")), c.Width, ""), nil
}

// statTrim drops blank cells (space, no-break space) at both ends of a
// rendered line. A code span at an edge loses its outer pad cell (glamour
// draws it as a styled no-break space), not its colours: the sequences
// before the first cell left are kept (less pairs that style nothing),
// other escape sequences (OSC 8) are kept, and the styles after the last
// cell left end in one reset. An incomplete sequence at the end is dropped.
func statTrim(s string) string {
	type tok struct {
		text     string
		esc, sgr bool
	}
	var toks []tok
	first, last := -1, -1
	for i := 0; i < len(s); {
		if j, sgr, ok := join.EscapeEnd(s, i); j > 0 {
			if ok {
				toks = append(toks, tok{s[i:j], true, sgr})
			}
			i = j
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		if r != ' ' && r != '\u00a0' {
			if first < 0 {
				first = len(toks)
			}
			last = len(toks)
		}
		toks = append(toks, tok{text: s[i : i+n]})
		i += n
	}
	reset := func(t tok) bool { return t.text == "\x1b[m" || t.text == "\x1b[0m" }
	var lead []tok
	for k := 0; k < len(toks) && (first < 0 || k < first); k++ {
		t := toks[k]
		if !t.esc {
			continue
		}
		if n := len(lead); n > 0 && reset(t) && lead[n-1].sgr && !reset(lead[n-1]) {
			lead = lead[:n-1] // a style over blank cells only
			continue
		}
		lead = append(lead, t)
	}
	var b strings.Builder
	for _, t := range lead {
		b.WriteString(t.text)
	}
	if first < 0 {
		return b.String()
	}
	for _, t := range toks[first : last+1] {
		b.WriteString(t.text)
	}
	styled := false
	for _, t := range toks[last+1:] {
		switch {
		case t.sgr:
			styled = true
		case t.esc:
			b.WriteString(t.text)
		}
	}
	if styled {
		b.WriteString("\x1b[m")
	}
	return b.String()
}

// pick chooses the layout. The spec: tiles when every row holds two tiles
// or more; else the list when it is allowed (listOK); else tiles. While a
// form streams, items arrive one after the other and the last one grows,
// and a layout switch must not make the frame shorter. pick walks the
// frames a stream shows (each item empty, then whole) from the source, so
// it needs no memory of past frames:
//   - item k empty: the spec, unless it is shorter than item k-1 whole;
//     then the other layout when that is not shorter, else the taller.
//   - item k whole: the layout picked for k empty, while the spec agrees.
//     When the spec turned from tiles to list, the list when it was
//     already as tall at k empty (it only grows), else the taller of the
//     two; any other turn: the taller of the two.
func (sd statsDraw) pick(cells []*statCell) ([]string, error) {
	h := 0
	var out []string
	for k := 1; k <= len(cells); k++ {
		empty := append(cells[:k-1:k-1], &statCell{})
		a, ea, err := sd.floor(empty, h)
		if err != nil {
			return nil, err
		}
		h = len(ea)
		full := cells[:k]
		b := sd.spec(full)
		switch {
		case b == a:
			out, err = sd.draw(full, b)
		case a == statsTiles && b == statsList:
			var el []string
			if el, err = sd.draw(empty, statsList); err == nil && len(el) >= len(ea) {
				out, err = sd.draw(full, statsList)
			} else if err == nil {
				out, err = sd.taller(full, b)
			}
		default:
			out, err = sd.taller(full, b)
		}
		if err != nil {
			return nil, err
		}
		h = len(out)
	}
	return out, nil
}

// floor is the spec for cells unless it is shorter than h lines.
func (sd statsDraw) floor(cells []*statCell, h int) (statsLayout, []string, error) {
	s := sd.spec(cells)
	ls, err := sd.draw(cells, s)
	if err != nil || len(ls) >= h {
		return s, ls, err
	}
	o, ok := sd.other(cells, s)
	if !ok {
		return s, ls, nil
	}
	lo, err := sd.draw(cells, o)
	if err != nil {
		return 0, nil, err
	}
	if len(lo) >= h || len(lo) > len(ls) {
		return o, lo, nil
	}
	return s, ls, nil
}

// taller draws cells in layout s and in the other layout, and keeps the
// taller (s on a tie).
func (sd statsDraw) taller(cells []*statCell, s statsLayout) ([]string, error) {
	ls, err := sd.draw(cells, s)
	if err != nil {
		return nil, err
	}
	o, ok := sd.other(cells, s)
	if !ok {
		return ls, nil
	}
	lo, err := sd.draw(cells, o)
	if err != nil {
		return nil, err
	}
	if len(lo) > len(ls) {
		return lo, nil
	}
	return ls, nil
}

// other is the layout that is not s, when cells can be drawn in it. The
// list as a fallback needs only a label column of one cell or more: a
// value that grew past statsMaxValue while the list was drawn keeps the
// list, so the frame does not shrink.
func (sd statsDraw) other(cells []*statCell, s statsLayout) (statsLayout, bool) {
	if s == statsList {
		return statsTiles, true
	}
	return statsList, sd.c.Width-statsValueWidth(cells)-statsListGap >= 1
}

func (sd statsDraw) spec(cells []*statCell) statsLayout {
	for _, r := range sd.rows(cells) {
		if r[1]-r[0] == 1 {
			if sd.listOK(cells) {
				return statsList
			}
			break
		}
	}
	return statsTiles
}

func statsValueWidth(cells []*statCell) int {
	vw := 0
	for _, ce := range cells {
		vw = max(vw, ce.vw)
	}
	return vw
}

// listOK: the spec allows the list.
func (sd statsDraw) listOK(cells []*statCell) bool {
	vw := statsValueWidth(cells)
	return vw <= statsMaxValue && sd.c.Width-vw-statsListGap >= statsMinLabel
}

// rows packs tiles left to right: [from, to) per row.
func (sd statsDraw) rows(cells []*statCell) [][2]int {
	var rs [][2]int
	for i := 0; i < len(cells); {
		j, used := i, 0
		for j < len(cells) {
			need := cells[j].w
			if j > i {
				need += statsGap
			}
			if j > i && used+need > sd.c.Width {
				break
			}
			used += need
			j++
		}
		rs = append(rs, [2]int{i, j})
		i = j
	}
	return rs
}

func (sd statsDraw) draw(cells []*statCell, s statsLayout) ([]string, error) {
	if s == statsList {
		return sd.list(cells)
	}
	return sd.tiles(cells), nil
}

func (sd statsDraw) tiles(cells []*statCell) []string {
	out := append([]string(nil), sd.head...)
	pad := func(s string, to int) string {
		return s + strings.Repeat(" ", to-ansi.StringWidth(s))
	}
	for _, r := range sd.rows(cells) {
		if len(out) > 0 {
			out = append(out, "")
		}
		var vl, ll strings.Builder
		for k := r[0]; k < r[1]; k++ {
			ce := cells[k]
			if k == r[1]-1 {
				vl.WriteString(ce.val)
				ll.WriteString(ce.lab)
				continue
			}
			vl.WriteString(pad(ce.val, ce.w+statsGap))
			ll.WriteString(pad(ce.lab, ce.w+statsGap))
		}
		out = append(out, strings.TrimRight(vl.String(), " "), strings.TrimRight(ll.String(), " "))
	}
	return out
}

func (sd statsDraw) list(cells []*statCell) ([]string, error) {
	out := append([]string(nil), sd.head...)
	if len(out) > 0 {
		out = append(out, "")
	}
	vw := statsValueWidth(cells)
	gap := statsListGap
	if vw == 0 {
		gap = 0
	}
	cw := sd.c.Width - vw - gap
	indent := strings.Repeat(" ", vw+gap)
	for _, ce := range cells {
		lab, err := ce.lines(sd.c, cw)
		if err != nil {
			return nil, err
		}
		first := strings.Repeat(" ", vw-ce.vw) + ce.val
		if len(lab) == 0 {
			out = append(out, strings.TrimRight(first, " "))
			continue
		}
		for j, l := range lab {
			if j == 0 {
				out = append(out, first+strings.Repeat(" ", gap)+l)
			} else {
				out = append(out, indent+l)
			}
		}
	}
	return out, nil
}

// lines is the label wrapped to width cw, each line trimmed and muted.
func (ce *statCell) lines(c Ctx, cw int) ([]string, error) {
	if ls, ok := ce.wrapped[cw]; ok {
		return ls, nil
	}
	ls, err := listLines(c, ce.label, cw)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range ls {
		if l = statTrim(l); ansi.Strip(l) != "" || len(out) > 0 {
			out = append(out, l)
		}
	}
	for len(out) > 0 && ansi.Strip(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	for i, l := range out {
		if ansi.Strip(l) != "" {
			out[i] = c.Theme.Muted.Render(l)
		} else {
			out[i] = ""
		}
	}
	if ce.wrapped == nil {
		ce.wrapped = map[int][]string{}
	}
	ce.wrapped[cw] = out
	return out, nil
}
