package bubbleteandmd

import (
	"strings"
	"sync"

	"github.com/AlexanderGrooff/mermaid-ascii/pkg/diagram"
	mermaid "github.com/AlexanderGrooff/mermaid-ascii/pkg/render"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/lru"
)

// Mermaid diagrams (Carl 2026-11-18). A closed top-level ```mermaid fence
// (info string `mermaid` alone, at indent 0) draws as a terminal diagram
// (github.com/AlexanderGrooff/mermaid-ascii: flowchart/graph, sequence, ER),
// in glamour's code block frame: head and tail lines, margin, the code
// text's colour, pad cells. A fence still being written draws as code:
// heal adds heal.OpenMermaid to the opener of a fence it closes, so its info
// string is not `mermaid` alone. The diagram is never fewer lines than the
// code block it replaces (blank code lines under it), so a frame does not
// shrink when the closer arrives.
//
// Code, as before, when: the library returns an error or panics, a line is
// wider than the code block (cells, x/ansi), the output is empty, the type
// is not one it draws (it returns an error), or the body has more than
// mermaidMaxLines lines (layout time grows faster than the node count).
// Colours (classDef) are dropped in every style. Fences in lists, quotes
// and form bodies, and titled fences, stay code.

// mermaidMaxLines: a body with more lines draws as code. Measured
// (dev box): 20 edges 36 ms, 50 edges 103 ms, 100 edges 369 ms.
const mermaidMaxLines = 100

// mermaidCacheMax bounds the process-wide diagram cache (entries).
const mermaidCacheMax = 128

// Mermaid turns mermaid diagrams on (the default) or off. Off: every
// ```mermaid fence draws as code, as before diagrams. Call it before the
// first Render. The Stream key must name it.
func (r *DirectiveRenderer) Mermaid(on bool) { r.noMermaid = !on }

// mermaidFence reports whether src (one top-level block) is a closed
// ```mermaid fence at indent 0 with no other info, and returns its body.
func mermaidFence(src string) (body string, ok bool) {
	info, body, closed, ok := topFence(src)
	if !ok || !closed || strings.TrimSpace(info) != "mermaid" {
		return "", false
	}
	return body, true
}

// mermaidPiece appends the diagram piece of a mermaid fence block (src, its
// body) in target t. drawn is false when the fence must draw as code (the
// caller renders it as before).
func (r *DirectiveRenderer) mermaidPiece(t *target, pieces []piece, src, body string, first bool) (_ []piece, drawn bool, err error) {
	cal, err := r.diffCalibrate(t, first)
	if err != nil || cal == nil {
		return pieces, false, err
	}
	lines, ok := mermaids.get(body, cal.area)
	if !ok {
		return pieces, false, nil
	}
	// The code block it replaces: a frame drew it while the fence streamed.
	code, err := r.glamourPiece(t, nil, src, first)
	if err != nil {
		return pieces, false, err
	}
	codeLines := 0
	if len(code) > 0 {
		codeLines = strings.Count(code[0].text, "\n") + 1
	}
	all := make([]string, 0, max(codeLines, len(cal.head)+len(lines)+len(cal.tail)))
	all = append(all, cal.head...)
	for _, l := range lines {
		w := xansi.StringWidth(l)
		all = append(all, cal.lead+cal.pre+l+cal.post+strings.Repeat(cal.cell, cal.area-w))
	}
	blank := cal.lead + strings.Repeat(cal.cell, cal.area)
	for len(all)+len(cal.tail) < codeLines {
		all = append(all, blank)
	}
	all = append(all, cal.tail...)
	return append(pieces, piece{text: strings.Join(all, "\n")}), true, nil
}

// mermaidCache is the process-wide cache of drawn diagrams, by body and
// width: a diagram is laid out once, not per frame or per Stream. A
// failure is kept too (ok false). The value does not depend on the style:
// colours are dropped, and the frame is put on per render.
type mermaidCache struct {
	mu sync.Mutex
	c  *lru.Cache[mermaidKey, mermaidDrawn]
	// renders: library calls (tests).
	renders int
}

type mermaidKey struct {
	src   string
	width int
}

type mermaidDrawn struct {
	lines []string
	ok    bool
}

var mermaids = &mermaidCache{c: lru.New[mermaidKey, mermaidDrawn](mermaidCacheMax)}

// get returns the diagram of src at most w cells wide; ok is false when it
// must draw as code. The lock is not held while the library draws: two
// callers that miss at once both draw, and one result is kept.
func (c *mermaidCache) get(src string, w int) ([]string, bool) {
	k := mermaidKey{src, w}
	c.mu.Lock()
	v, hit := c.c.Get(k)
	c.mu.Unlock()
	if hit {
		return v.lines, v.ok
	}
	lines, ok := drawMermaid(src, w)
	k.src = strings.Clone(src)
	c.mu.Lock()
	c.renders++
	c.c.Put(k, mermaidDrawn{lines, ok})
	c.mu.Unlock()
	return lines, ok
}

// mermaidRender is the library call (a variable for tests).
var mermaidRender = func(src string, w int) (string, error) {
	cfg := diagram.DefaultConfig()
	cfg.MaxWidth = w // graphs only; the caller measures every type
	return mermaid.RenderDiagram(src, cfg)
}

// drawMermaid draws src with mermaid-ascii (Unicode box drawing), at most w
// cells wide. ok is false on an error, a panic, an empty or too wide output,
// or too long a body.
func drawMermaid(src string, w int) (lines []string, ok bool) {
	if strings.Count(src, "\n") > mermaidMaxLines {
		return nil, false
	}
	defer func() {
		if recover() != nil {
			lines, ok = nil, false
		}
	}()
	out, err := mermaidRender(src, w)
	if err != nil {
		return nil, false
	}
	// classDef colours: gookit/color picks the codes from the environment.
	out = xansi.Strip(out)
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimRight(l, " \t")
		if xansi.StringWidth(l) > w || strings.ContainsAny(l, "\t") {
			return nil, false
		}
		lines = append(lines, l)
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	if len(lines) == 0 {
		return nil, false
	}
	return lines, true
}
