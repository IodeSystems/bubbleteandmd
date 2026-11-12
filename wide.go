package bubbleteandmd

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
	"github.com/iodesystems/bubbleteandmd/internal/join"
	"github.com/iodesystems/bubbleteandmd/internal/tablestream"
)

// Wide blocks (ported from dun's internal/tui/wide.go splitWide and
// renderAnswer): a top-level code block or table wider than the word wrap
// width is not wrapped. It renders through a glamour renderer that wraps at
// the block's own width, and each of its lines is cut at the pane width,
// with "… +N cols →" at the cut when the line is longer. Prose wraps at the
// word wrap width as before. A line of code or a table row wrapped breaks
// the columns it is read by; cut, the block has as many lines as its source.
//
// The piece keeps glamour's frame (blank lines, margin), so blocks join as
// they do in a whole render. A line that fits the word wrap width is cut to
// it, padding included: the same bytes as glamour's own render at that width.

// wideCap is the widest a wide block renders at. A code line longer than
// that wraps at it; a table that needs more shrinks its columns to fit.
const wideCap = 512

// wideRenderersMax bounds the wide renderers kept per DirectiveRenderer
// (one per render width); past it the map is dropped.
const wideRenderersMax = 32

// WideBlocks turns wide blocks on: top-level code blocks and tables wider
// than the word wrap width render at their own width and are cut at pane.
// A pane below the word wrap width is read as that width; 0 turns wide
// code blocks off (the default). Tables are wide blocks with or without
// it: with pane 0 a wide table is cut at the word wrap width, so each row
// is one line (Carl 2026-11-11, single-line table rows). Call it before
// the first Render. The Stream key must name pane too.
//
// Not in wide blocks: code blocks and tables inside lists, quotes and form
// bodies; titled code fences and file trees (forms: cut at the width, as
// before); a table with a row the width model does not read (an email
// address, an emoji shortcode) or with glamour options given to NewRenderer.
func (r *DirectiveRenderer) WideBlocks(pane int) {
	if pane > 0 {
		pane = max(pane, r.width)
	}
	r.pane = pane
}

// wideWidth returns the word wrap width block src renders at when it is a
// wide code block (WideBlocks on) or a wide table, else 0. src is one
// top-level block, no Sentinel; it must not hold a form or a `$$` formula
// (the caller checks).
func (r *DirectiveRenderer) wideWidth(src string) int {
	margin := 2 * r.margin()
	if need, ok := codeNeed(src); ok {
		if r.pane == 0 {
			return 0
		}
		indent := 0
		if m := r.style.CodeBlock.Margin; m != nil {
			indent = int(*m)
		}
		need += indent + margin
		if need <= r.width {
			return 0
		}
		// Code is padded to the wrap width and the pad is cut off, so
		// widths are rounded up: one renderer serves many blocks.
		return min((need+8+19)/20*20, wideCap)
	}
	if !strings.HasPrefix(src, "|") || len(r.opts) > 0 {
		return 0
	}
	if r.nat == nil {
		o, _ := r.GlamourOptions()
		r.nat = tablestream.NewNatural(o)
	}
	nat, ok := r.nat.Width(src)
	if !ok || nat+margin <= r.width {
		return 0
	}
	// Exact: glamour stretches a table to its wrap width.
	return min(nat+margin, wideCap)
}

func (r *DirectiveRenderer) margin() int {
	if m := r.style.Document.Margin; m != nil {
		return int(*m)
	}
	return 0
}

// codeNeed reports whether src is one fenced code block (an unclosed fence
// runs to the end; after the closing line only blank lines) and the display
// width of its widest line, a tab read as 4 columns.
func codeNeed(src string) (int, bool) {
	nl := strings.IndexByte(src, '\n')
	if nl < 0 {
		return 0, false
	}
	m := codeFenceRe.FindStringSubmatch(src[:nl])
	if m == nil || (m[1][0] == '`' && strings.Contains(src[len(m[0]):nl], "`")) {
		return 0, false
	}
	fence := m[1]
	need := 0
	rest := src[nl+1:]
	for rest != "" {
		line := rest
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			line, rest = rest[:i], rest[i+1:]
		} else {
			rest = ""
		}
		if c := codeFenceRe.FindStringSubmatch(line); c != nil && c[1][0] == fence[0] && len(c[1]) >= len(fence) && strings.TrimSpace(line[len(c[0]):]) == "" {
			if strings.TrimSpace(rest) != "" {
				return 0, false
			}
			return need, true
		}
		need = max(need, xansi.StringWidth(strings.ReplaceAll(line, "\t", "    ")))
	}
	return need, true
}

// wideTarget is the glamour renderer that wraps at w for wide blocks.
func (r *DirectiveRenderer) wideTarget(w int) (*target, error) {
	if t, ok := r.wides[w]; ok {
		return t, nil
	}
	all := append([]glamour.TermRendererOption{glamour.WithStyles(r.style), glamour.WithWordWrap(w), glamour.WithChromaFormatter(chromaFormatter)}, r.opts...)
	g, err := r.term(r.style, w, all)
	if err != nil {
		return nil, err
	}
	t := &target{g: g, width: w}
	if err := t.affixes(); err != nil {
		return nil, err
	}
	if t.p != r.outer.p || t.s != r.outer.s {
		return nil, join.ErrAffixes
	}
	if r.wides == nil || len(r.wides) >= wideRenderersMax {
		r.wides = map[int]*target{}
	}
	r.wides[w] = t
	return t, nil
}

// widePiece renders block src at wrap width w and cuts its lines.
func (r *DirectiveRenderer) widePiece(src string, first bool, w int) (string, error) {
	t, err := r.wideTarget(w)
	if err != nil {
		return "", err
	}
	out, err := t.g.Render(join.Input(langs.plainUnknownLangs(src), first))
	if err != nil {
		return "", err
	}
	piece, err := join.Strip(out, t.p, t.s)
	if err != nil {
		return "", err
	}
	return r.cutLines(piece), nil
}

// cutLines cuts each line of a wide piece: a line that fits the word wrap
// width keeps that many cells (glamour's padding at that width); a longer
// one loses its trailing blank cells, and one longer than the pane is cut
// there with the edge note.
func (r *DirectiveRenderer) cutLines(piece string) string {
	var b strings.Builder
	b.Grow(len(piece))
	for len(piece) > 0 {
		line := piece
		nl := strings.IndexByte(piece, '\n')
		if nl >= 0 {
			line, piece = piece[:nl], piece[nl+1:]
		} else {
			piece = ""
		}
		b.WriteString(r.cutLine(line))
		if nl >= 0 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (r *DirectiveRenderer) cutLine(line string) string {
	cw := xansi.StringWidth(strings.TrimRight(xansi.Strip(line), " "))
	// pane 0 (no WideBlocks): a wide table is cut at the word wrap width.
	pane := r.pane
	if pane == 0 {
		pane = r.width
	}
	// glamour's lines end at the wrap width less the right margin.
	switch fit := r.width - r.margin(); {
	case cw <= fit:
		return keepCells(line, fit)
	case cw <= pane:
		return keepCells(line, cw)
	}
	room, ind := edgeNote(cw, pane)
	if room == 0 {
		return keepCells(line, pane)
	}
	return keepCells(line, pane-room) + r.theme.Muted.Render(ind)
}

// edgeNote is dun's cutRow note for a line w cells wide cut at pane: the
// note, right-aligned in room cells, says how many cells are not shown.
// room is 0 when the pane is too narrow for a note.
func edgeNote(w, pane int) (room int, note string) {
	for room = 12; room < pane/2; room++ {
		note = fmt.Sprintf(" … +%d cols →", w-(pane-room))
		if n := xansi.StringWidth(note); n <= room {
			return room, strings.Repeat(" ", room-n) + note
		}
	}
	return 0, ""
}

// keepCells returns the first n cells of s with the escape sequences before
// and among them. A reset right after the nth cell is kept (glamour closes
// each pad cell with one). When the cut leaves a style or an OSC 8 link
// open, a reset (or the link's end) is added.
func keepCells(s string, n int) string {
	i, cells := 0, 0
	open, link := false, false
	for i < len(s) {
		if s[i] == 0x1b {
			end, sgr, _ := join.EscapeEnd(s, i)
			seq := s[i:end]
			if cells == n {
				if sgr && isReset(seq) {
					open, i = false, end
				}
				break
			}
			switch {
			case sgr:
				open = !isReset(seq)
			case strings.HasPrefix(seq, "\x1b]8;"):
				link = oscLinkOpens(seq)
			}
			i = end
			continue
		}
		g, w := xansi.FirstGraphemeCluster(s[i:], xansi.GraphemeWidth)
		if cells+w > n {
			break
		}
		cells += w
		i += len(g)
	}
	if i == len(s) {
		return s
	}
	out := s[:i]
	if open {
		out += "\x1b[m"
	}
	if link {
		out += "\x1b]8;;\x07"
	}
	return out
}

func isReset(sgr string) bool { return sgr == "\x1b[m" || sgr == "\x1b[0m" }

// oscLinkOpens reports whether OSC 8 sequence seq starts a link (has a URI)
// rather than ending one.
func oscLinkOpens(seq string) bool {
	body := strings.TrimPrefix(seq, "\x1b]8;")
	body = strings.TrimSuffix(strings.TrimSuffix(body, "\x07"), "\x1b\\")
	_, uri, _ := strings.Cut(body, ";")
	return uri != ""
}

// wideTable returns the wrap width and options a Stream renders tail at
// when tail (one block, no Sentinel) is a wide table, so it can grow from
// cached rows (tablestream) through the wide renderer, with or without
// WideBlocks. cut makes the
// renderer's piece the piece Render gives.
func (r *DirectiveRenderer) wideTable(tail string) (t *target, opts ansi.Options, ok bool, err error) {
	if !strings.HasPrefix(tail, "|") || len(r.opts) > 0 {
		return nil, opts, false, nil
	}
	if bs := blocks.Split(tail); len(bs) != 1 {
		return nil, opts, false, nil
	}
	if _, math := mathFences(tail); math {
		return nil, opts, false, nil
	}
	if _, form := directive.Parse(tail); form {
		return nil, opts, false, nil
	}
	w := r.wideWidth(tail)
	if w == 0 {
		return nil, opts, false, nil
	}
	if t, err = r.wideTarget(w); err != nil {
		return nil, opts, false, err
	}
	opts, _ = r.GlamourOptions()
	opts.WordWrap = w
	return t, opts, true, nil
}
