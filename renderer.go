package bubbleteandmd

import (
	"regexp"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
	"github.com/iodesystems/bubbleteandmd/internal/join"
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
}

// target is one glamour renderer and the frame its directive pieces get.
type target struct {
	g    *glamour.TermRenderer
	p, s string // document prefix and suffix (join.Affixes)
	// width: content width of a directive piece, frame excluded.
	width int
	frame directive.Style
}

// NewRenderer returns a renderer for style at word wrap width. opts go to
// glamour after the style and the word wrap; they must not change either.
func NewRenderer(style ansi.StyleConfig, width int, opts ...glamour.TermRendererOption) (*DirectiveRenderer, error) {
	plain := directive.PlainStyle(style)
	r := &DirectiveRenderer{style: style, opts: opts, theme: directive.ThemeFrom(style, plain), width: width, inners: map[int]*target{}}
	all := append([]glamour.TermRendererOption{glamour.WithStyles(style), glamour.WithWordWrap(width)}, opts...)
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
// plain Markdown at the top level: the style and the word wrap. ok is false
// when NewRenderer was given glamour options: they cannot be read back.
// Stream uses it to render a growing table from cached rows (GlamourOptions).
func (r *DirectiveRenderer) GlamourOptions() (ansi.Options, bool) {
	return ansi.Options{Styles: r.style, WordWrap: r.width}, len(r.opts) == 0
}

// Render renders md. An md that starts with join.Sentinel (as Stream sends
// every block after the first) renders its first block in not-first place.
func (r *DirectiveRenderer) Render(md string) (string, error) {
	return r.render(&r.outer, Sanitize(md))
}

// parsed is one top-level block: its source with `$$` formulas made math
// fences, and the first form in it.
type parsed struct {
	src  string
	d    directive.Directive
	form bool
}

func (r *DirectiveRenderer) render(t *target, md string) (string, error) {
	body, first := md, true
	if strings.HasPrefix(md, join.Sentinel) {
		body, first = md[len(join.Sentinel):], false
	}
	bs := blocks.Split(body)
	ps := make([]parsed, len(bs))
	plain := true
	for i, b := range bs {
		src, math := mathFences(b.Source)
		d, ok := directive.Parse(src)
		ps[i] = parsed{src, d, ok}
		if ok || math {
			plain = false
		}
	}
	if plain {
		return t.g.Render(md)
	}
	var pieces []string
	for i, p := range ps {
		var err error
		if pieces, err = r.block(t, pieces, p, first && i == 0); err != nil {
			return "", err
		}
	}
	return join.Join(t.p, t.s, pieces), nil
}

// block appends the pieces of one top-level block: Markdown around a form
// renders through glamour, a form draws as a framed piece.
func (r *DirectiveRenderer) block(t *target, pieces []string, p parsed, first bool) ([]string, error) {
	src, d, ok := p.src, p.d, p.form
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
		lines, err := r.form(d, t.width)
		if err != nil {
			return nil, err
		}
		if len(lines) > 0 {
			pieces = append(pieces, directive.Frame(lines, t.width, t.frame, first))
			first = false
		}
		src = src[d.End:]
		d, ok = directive.Parse(src)
	}
}

// form draws d at content width w, with the Markdown quoted after a list
// form (Directive.After) below it.
func (r *DirectiveRenderer) form(d directive.Directive, w int) ([]string, error) {
	c := directive.Ctx{Width: w, Theme: r.theme, Inner: r.inner}
	lines, err := directive.Render(d, c)
	if err != nil || strings.TrimSpace(d.After) == "" {
		return lines, err
	}
	after, err := r.inner(d.After, w)
	if err != nil {
		return nil, err
	}
	after = strings.Trim(after, "\n")
	if after == "" {
		return lines, nil
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return append(lines, strings.Split(after, "\n")...), nil
}

func (r *DirectiveRenderer) glamourPiece(t *target, pieces []string, src string, first bool) ([]string, error) {
	if strings.TrimSpace(src) == "" {
		return pieces, nil
	}
	out, err := t.g.Render(join.Input(src, first))
	if err != nil {
		return nil, err
	}
	piece, err := join.Strip(out, t.p, t.s)
	if err != nil {
		return nil, err
	}
	if piece == "" {
		return pieces, nil
	}
	return append(pieces, piece), nil
}

// inner renders a form body at content width w: margin 0, no pad cells.
// Forms in it draw too (Carl B2: forms nest inside form bodies).
func (r *DirectiveRenderer) inner(md string, w int) (string, error) {
	w = max(w, 1)
	t, ok := r.inners[w]
	if !ok {
		st := r.style
		zero := uint(0)
		st.Document.Margin, st.Document.Indent = &zero, &zero
		all := append(append([]glamour.TermRendererOption{}, r.opts...), glamour.WithStyles(st), glamour.WithWordWrap(w))
		g, err := glamour.NewTermRenderer(all...)
		if err != nil {
			return "", err
		}
		t = &target{g: g, width: w}
		if err := t.affixes(); err != nil {
			return "", err
		}
		r.inners[w] = t
	}
	out, err := r.render(t, md)
	if err != nil {
		return "", err
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = padRe.ReplaceAllString(l, "")
	}
	return strings.Join(lines, "\n"), nil
}

// padRe is a line's trailing pad cells: plain or styled spaces.
var padRe = regexp.MustCompile(`(?: |\x1b\[[0-9;]*m +\x1b\[m)+$`)

// mathFenceOpen is a `$$` formula's first line (remark-math: up to three
// spaces of indent).
var (
	mathFenceOpen = regexp.MustCompile(`^ {0,3}\$\$`)
	codeFenceRe   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
)

// mathFences rewrites each top-level `$$ … $$` formula in src as a ```math
// fence (Carl B7: a formula shows as its TeX source, as code). Lines inside a
// code fence are left alone. A formula not closed runs to the end of src.
// ok reports whether src held a formula.
func mathFences(src string) (string, bool) {
	if !strings.Contains(src, "$$") {
		return src, false
	}
	lines := strings.SplitAfter(src, "\n")
	var b strings.Builder
	found := false
	fence := ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		text := strings.TrimRight(line, "\r\n")
		if fence != "" {
			if m := codeFenceRe.FindStringSubmatch(text); m != nil && strings.HasPrefix(m[1], fence) && strings.TrimSpace(text[len(m[0]):]) == "" {
				fence = ""
			}
			b.WriteString(line)
			continue
		}
		if m := codeFenceRe.FindStringSubmatch(text); m != nil && !(m[1][0] == '`' && strings.Contains(text[len(m[0]):], "`")) {
			fence = m[1]
			b.WriteString(line)
			continue
		}
		loc := mathFenceOpen.FindStringIndex(text)
		if loc == nil {
			b.WriteString(line)
			continue
		}
		found = true
		eol := line[len(text):]
		var body []string
		rest := text[loc[1]:]
		closed := false
		if j := closeDollars(rest); j >= 0 {
			// one line: $$ x $$
			body = append(body, strings.TrimSpace(rest[:j]))
			closed = true
		} else if strings.TrimSpace(rest) != "" {
			body = append(body, strings.TrimSpace(rest))
		}
		for !closed && i+1 < len(lines) {
			i++
			l := strings.TrimRight(lines[i], "\r\n")
			eol = lines[i][len(l):]
			if j := closeDollars(l); j >= 0 {
				if strings.TrimSpace(l[:j]) != "" {
					body = append(body, l[:j])
				}
				closed = true
				break
			}
			body = append(body, l)
		}
		code := strings.Join(body, "\n")
		f := longFence(code)
		b.WriteString(f + "math\n")
		if code != "" {
			b.WriteString(code + "\n")
		}
		b.WriteString(f)
		if eol == "" {
			eol = "\n"
		}
		b.WriteString(eol)
	}
	return b.String(), found
}

// closeDollars returns where the first unescaped `$$` in s starts, or -1.
func closeDollars(s string) int {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '$' && s[i+1] == '$' {
			return i
		}
	}
	return -1
}

// longFence is a backtick fence longer than any backtick run at a line start of code.
func longFence(code string) string {
	n := 3
	for _, l := range strings.Split(code, "\n") {
		t := strings.TrimLeft(l, " ")
		k := 0
		for k < len(t) && t[k] == '`' {
			k++
		}
		if k >= n {
			n = k + 1
		}
	}
	return strings.Repeat("`", n)
}
