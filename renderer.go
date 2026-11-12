package bubbleteandmd

import (
	"regexp"
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
	"github.com/iodesystems/bubbleteandmd/internal/join"
	"github.com/iodesystems/bubbleteandmd/internal/tablestream"
)

// DirectiveRenderer renders Markdown as glamour does, and draws
// llx-markdown's directive forms (callouts, steps, tabs, stats, pros and
// cons, details, sources, quotes with a source, file trees, titled code
// fences) and `$$` formulas. It satisfies Renderer, so NewStream can stream
// it. A document with no form and no `$$` formula renders exactly as glamour
// renders it.
//
// A DirectiveRenderer is used by one goroutine at a time.
type DirectiveRenderer struct {
	style ansi.StyleConfig
	opts  []glamour.TermRendererOption
	theme directive.Theme
	width int
	outer target
	// inners: one glamour renderer per inner content width, margin 0.
	inners map[int]*target
	// pane: wide blocks are cut here (WideBlocks; 0: off). wides: their
	// renderers by wrap width; nat measures wide tables.
	pane  int
	wides map[int]*target
	nat   *tablestream.Natural
	// diff: escape table and memos of diff fences (diff.go).
	diff diffState
}

// target is one glamour renderer and the frame its directive pieces get.
type target struct {
	g    *glamour.TermRenderer
	p, s string // document prefix and suffix (join.Affixes)
	// width: content width of a directive piece, frame excluded.
	width int
	frame directive.Style
	// dcal: how a code line is drawn here, not first and first; dfall:
	// a titled diff's lines when dcal could not be read (diff.go).
	dcal  [2]*diffCal
	dfall *diffCal
}

// NewRenderer returns a renderer for style at word wrap width. opts go to
// glamour after the style, the word wrap and the chroma formatter; they must
// not change the style or the word wrap.
//
// Code blocks use chroma formatter "bubbleteandmd-terminal256" (same bytes as
// "terminal256", style tables cached; chromafmt.go). The first NewRenderer
// in a process registers it in chroma's formatter registry, which has no
// lock: make it before other goroutines render with chroma.
func NewRenderer(style ansi.StyleConfig, width int, opts ...glamour.TermRendererOption) (*DirectiveRenderer, error) {
	registerChromaFormatter()
	plain := directive.PlainStyle(style)
	r := &DirectiveRenderer{style: style, opts: opts, theme: directive.ThemeFrom(style, plain), width: width, inners: map[int]*target{}}
	all := append([]glamour.TermRendererOption{glamour.WithStyles(style), glamour.WithWordWrap(width), glamour.WithChromaFormatter(chromaFormatter)}, opts...)
	g, err := glamour.NewTermRenderer(all...)
	if err != nil {
		return nil, err
	}
	margin := 0
	if style.Document.Margin != nil {
		margin = int(*style.Document.Margin)
	}
	// glamour keeps the margin on both sides (internal/spike/frame_spike_test.go).
	r.outer = target{g: g, width: max(width-2*margin, 1), frame: r.theme.Frame}
	if err := r.outer.affixes(); err != nil {
		return nil, err
	}
	return r, nil
}

func (t *target) affixes() error {
	empty, err := t.g.Render("")
	if err != nil {
		return err
	}
	t.p, t.s, err = join.Affixes(empty)
	return err
}

// GlamourOptions returns the options of the glamour renderer that draws
// plain Markdown at the top level: the style, the word wrap and the chroma
// formatter. ok is false
// when NewRenderer was given glamour options: they cannot be read back.
// Stream uses it to render a growing table from cached rows (GlamourOptions).
func (r *DirectiveRenderer) GlamourOptions() (ansi.Options, bool) {
	return ansi.Options{Styles: r.style, WordWrap: r.width, ChromaFormatter: chromaFormatter}, len(r.opts) == 0
}

// Render renders md. An md that starts with join.Sentinel (as Stream sends
// every block after the first) renders its first block in not-first place.
func (r *DirectiveRenderer) Render(md string) (string, error) {
	out, _, err := r.render(&r.outer, Sanitize(md), scope{})
	return out, err
}

// RenderState renders md as Render does, with each form drawn in its state
// in st (nil: as streamed), and returns the marks of the interactive parts,
// lines counted in out (StateRenderer). offset is the document offset of
// md's text after the sentinel: form keys are document keys (form offsets
// in the source as written, before `$$` formulas are rewritten). md should
// be sanitized already, else offsets move.
func (r *DirectiveRenderer) RenderState(md string, offset int, st *State) (string, []Mark, error) {
	return r.render(&r.outer, Sanitize(md), scope{on: true, base: offset, st: st})
}

// scope is where the forms of one render sit in the document: their keys
// and their state. The zero scope draws forms as streamed and gives no
// marks.
type scope struct {
	on bool
	// parent is the key of the body ("<form key>/<slot>"), "" at top level.
	parent string
	// base is the document offset of the rendered text, top level only.
	base int
	st   *directive.State
}

// key is the FormID.Key of a form at byte rel of the rendered text.
func (sc scope) key(rel int) string {
	if sc.parent == "" {
		return strconv.Itoa(sc.base + rel)
	}
	return sc.parent + "@" + strconv.Itoa(rel)
}

// parsed is one top-level block: its source with `$$` formulas made math
// fences, and the first form in it. wide: the wrap width of a wide block
// (wide.go), else 0. shifts: where the rewrite moved the text after a
// formula.
type parsed struct {
	src    string
	d      directive.Directive
	form   bool
	wide   int
	shifts []directive.MathShift
	// diff: an untitled diff fence (diff.go); body is its code.
	diff bool
	body string
}

// piece is a rendered piece and the marks in it, lines counted from its
// first line.
type piece struct {
	text  string
	marks []directive.Mark
}

func (r *DirectiveRenderer) render(t *target, md string, sc scope) (string, []directive.Mark, error) {
	body, first := md, true
	if strings.HasPrefix(md, join.Sentinel) {
		body, first = md[len(join.Sentinel):], false
	}
	bs := blocks.Split(body)
	ps := make([]parsed, len(bs))
	plain := true
	for i, b := range bs {
		src, math, shifts := directive.MathFences(b.Source)
		d, ok := directive.Parse(src)
		ps[i] = parsed{src: src, d: d, form: ok, shifts: shifts}
		if !ok && !math {
			ps[i].body, ps[i].diff = diffFence(src)
		}
		if !ok && !math && !ps[i].diff && t == &r.outer {
			ps[i].wide = r.wideWidth(src)
		}
		if ok || math || ps[i].wide > 0 || ps[i].diff {
			plain = false
		}
	}
	if plain {
		out, err := t.g.Render(langs.plainUnknownLangs(md))
		return out, nil, err
	}
	var pieces []piece
	for i, p := range ps {
		var err error
		if p.diff {
			pieces, err = r.diffPiece(t, pieces, p.src, p.body, first && i == 0)
		} else if p.wide > 0 {
			var text string
			if text, err = r.widePiece(p.src, first && i == 0, p.wide); err == nil && text != "" {
				pieces = append(pieces, piece{text: text})
			}
		} else {
			pieces, err = r.block(t, pieces, p, bs[i].Offset, first && i == 0, sc)
		}
		if err != nil {
			return "", nil, err
		}
	}
	texts := make([]string, len(pieces))
	for i, x := range pieces {
		texts[i] = x.text
	}
	// Piece k starts at line count(P) + the "\n" in the pieces before it:
	// join drops cells on a shared line, never a "\n"
	// (internal/spike/linemap_spike_test.go).
	var marks []directive.Mark
	line := strings.Count(t.p, "\n")
	for _, x := range pieces {
		for _, m := range x.marks {
			m.Line += line
			marks = append(marks, m)
		}
		line += strings.Count(x.text, "\n")
	}
	return join.Join(t.p, t.s, texts), marks, nil
}

// block appends the pieces of one top-level block, at byte at of the
// rendered text: Markdown around a form renders through glamour, a form
// draws as a framed piece.
func (r *DirectiveRenderer) block(t *target, pieces []piece, p parsed, at int, first bool, sc scope) ([]piece, error) {
	src, d, ok := p.src, p.d, p.form
	pos := 0
	for {
		if !ok {
			return r.glamourPiece(t, pieces, src, first)
		}
		var err error
		if pieces, err = r.glamourPiece(t, pieces, src[:d.Start], first); err != nil {
			return nil, err
		}
		if len(pieces) > 0 {
			first = false
		}
		// Keys are offsets in the source as written (directive.IDs).
		lines, marks, err := r.form(d, t.width, sc, at+directive.RawOffset(p.shifts, pos+d.Start))
		if err != nil {
			return nil, err
		}
		if len(lines) > 0 {
			if !first {
				// Frame's leading blank line
				for i := range marks {
					marks[i].Line++
				}
			}
			pieces = append(pieces, piece{directive.Frame(lines, t.width, t.frame, first), marks})
			first = false
		}
		src = src[d.End:]
		pos += d.End
		d, ok = directive.Parse(src)
	}
}

// form draws d at content width w; rel is its byte offset in the rendered
// text, for its key.
func (r *DirectiveRenderer) form(d directive.Directive, w int, sc scope, rel int) ([]string, []directive.Mark, error) {
	c := directive.Ctx{Width: w, Theme: r.theme, Inner: func(md string, w int) (string, error) {
		out, _, err := r.inner(md, w, scope{})
		return out, err
	}, Diff: r.diffTitled}
	if !sc.on {
		lines, _, err := directive.Render(d, c)
		return lines, nil, err
	}
	c.Key, c.State = sc.key(rel), sc.st
	c.Body = func(md string, w int, slot string) (string, []directive.Mark, error) {
		return r.inner(md, w, scope{on: true, parent: c.Key + "/" + slot, st: sc.st})
	}
	return directive.Render(d, c)
}

func (r *DirectiveRenderer) glamourPiece(t *target, pieces []piece, src string, first bool) ([]piece, error) {
	if strings.TrimSpace(src) == "" {
		return pieces, nil
	}
	out, err := t.g.Render(join.Input(langs.plainUnknownLangs(src), first))
	if err != nil {
		return nil, err
	}
	p, err := join.Strip(out, t.p, t.s)
	if err != nil {
		return nil, err
	}
	if p == "" {
		return pieces, nil
	}
	return append(pieces, piece{text: p}), nil
}

// inner renders a form body at content width w: margin 0, no pad cells.
// Forms in it draw too (Carl B2: forms nest inside form bodies).
func (r *DirectiveRenderer) inner(md string, w int, sc scope) (string, []directive.Mark, error) {
	t, err := r.innerTarget(w)
	if err != nil {
		return "", nil, err
	}
	out, marks, err := r.render(t, md, sc)
	if err != nil {
		return "", nil, err
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = padRe.ReplaceAllString(l, "")
	}
	return strings.Join(lines, "\n"), marks, nil
}

// innerTarget is the glamour renderer of form bodies at content width w:
// margin 0.
func (r *DirectiveRenderer) innerTarget(w int) (*target, error) {
	w = max(w, 1)
	if t, ok := r.inners[w]; ok {
		return t, nil
	}
	st := r.style
	zero := uint(0)
	st.Document.Margin, st.Document.Indent = &zero, &zero
	all := append([]glamour.TermRendererOption{glamour.WithChromaFormatter(chromaFormatter)}, r.opts...)
	all = append(all, glamour.WithStyles(st), glamour.WithWordWrap(w))
	g, err := glamour.NewTermRenderer(all...)
	if err != nil {
		return nil, err
	}
	t := &target{g: g, width: w}
	if err := t.affixes(); err != nil {
		return nil, err
	}
	r.inners[w] = t
	return t, nil
}

// padRe is a line's trailing pad cells: plain or styled spaces.
var padRe = regexp.MustCompile(`(?: |\x1b\[[0-9;]*m +\x1b\[m)+$`)

// codeFenceRe is a code fence line: up to three spaces, the fence run.
var codeFenceRe = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")

// mathFences is directive.MathFences without the shifts.
func mathFences(src string) (string, bool) {
	out, ok, _ := directive.MathFences(src)
	return out, ok
}
