package main

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"github.com/iodesystems/bubbleteandmd"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// tokens returns the byte offset where each token of src ends; the last is
// len(src). Token sizes are internal/corpus's Token pattern (1 to 7
// characters, 5 more one time in ten, mulberry32 from seed), counted in
// runes, not UTF-16 units: offsets in place of corpus.Prefixes' strings
// keep a long --file from holding every prefix in memory.
func tokens(src string, seed uint32) []int {
	r := corpus.Seeded(seed)
	var ends []int
	for at := 0; at < len(src); {
		step := r.Int(1, 7)
		if r.Next() < 0.1 {
			step += 5
		}
		for i := 0; i < step && at < len(src); i++ {
			_, n := utf8.DecodeRuneInString(src[at:])
			at += n
		}
		ends = append(ends, at)
	}
	return ends
}

// engine renders one pane. render returns the lines to show (at most the
// pane height) and the line count of the whole output. It runs off the UI
// goroutine, one call at a time per engine.
type engine interface {
	render(src string, final bool) (view string, lines int, err error)
}

// docEngine is bubbleteandmd: a Doc that owns its viewport.
type docEngine struct {
	r *bubbleteandmd.DirectiveRenderer
	d *bubbleteandmd.Doc
}

// newDocEngine makes a Renderer and Doc for width w and height h. wide
// turns WideBlocks on at the pane width. old, when not nil, gives its
// State (folds, tabs, focus) to the new Doc, as on a width change.
func newDocEngine(style ansi.StyleConfig, name string, w, h int, wide bool, c *bubbleteandmd.Cache, old *bubbleteandmd.Doc) (*docEngine, error) {
	r, err := bubbleteandmd.NewRenderer(style, w)
	if err != nil {
		return nil, err
	}
	pane := 0
	if wide {
		pane = w
		r.WideBlocks(pane)
	}
	d := bubbleteandmd.NewDoc(r, fmt.Sprintf("%s/%d/%d", name, w, pane), c)
	if old != nil {
		*d.State() = *old.State()
	}
	d.SetHeight(h)
	return &docEngine{r: r, d: d}, nil
}

func (e *docEngine) render(src string, final bool) (string, int, error) {
	var out string
	var err error
	if final {
		out, err = e.d.Final(src)
	} else {
		out, err = e.d.Frame(src)
	}
	return out, e.d.Lines(), err
}

// naiveEngine is what a host with no streaming layer does: glamour renders
// the whole reply on every token, with no heal and no cache. The view
// follows the bottom.
type naiveEngine struct {
	g *glamour.TermRenderer
	h int
}

func newNaiveEngine(style ansi.StyleConfig, w, h int) (*naiveEngine, error) {
	g, err := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(w))
	if err != nil {
		return nil, err
	}
	return &naiveEngine{g: g, h: h}, nil
}

func (e *naiveEngine) render(src string, _ bool) (string, int, error) {
	out, err := e.g.Render(src)
	if err != nil {
		return "", 0, err
	}
	ls := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	n := len(ls)
	if n > e.h {
		ls = ls[n-e.h:]
	}
	return strings.Join(ls, "\n") + "\n", n, nil
}

// stats are one pane's render numbers.
type stats struct {
	frames  int
	dropped int // stream steps (ticks that released tokens) never drawn: the pane was still rendering
	shrank  int // renders after new tokens with fewer lines than the one before
	lines   int // lines in the last output
	last    time.Duration
	total   time.Duration
	max     time.Duration
}

func (s stats) avg() time.Duration {
	if s.frames == 0 {
		return 0
	}
	return s.total / time.Duration(s.frames)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
