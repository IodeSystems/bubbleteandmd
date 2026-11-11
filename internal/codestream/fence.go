// Package codestream renders one growing fenced code block in chunks, so a
// frame does not re-render (and re-lex) the whole block.
//
// Carl 2026-11-11 (B1): while the fence streams, Frame is approximate;
// Final is the whole render, exact. Frame has the same lines, and the same
// visible text on each line, as the whole render; only SGR bytes can differ,
// at the first line of a chunk (chroma lexes each chunk alone, so a token
// that spans a chunk seam can get another colour there until Final).
//
// How a frame is built (spike: internal/spike/code_splice_spike_test.go in
// the N2 worktree). glamour renders a code block as head lines, one or more
// lines per source line (wrapped lines), and tail lines; each line is padded
// to the block width by itself. The code lines are cut into chunks of
// ChunkLines source lines. Each completed chunk is rendered once, as a fence
// of its own, and its body lines are kept. The last (open) chunk is rendered
// each frame with the document text before the fence and the fence's own
// end, and gives the head and tail lines. The whole render starts every line
// after the first with the SGR pen of the line before it; a chunk rendered
// alone lacks it on its first line, so penCarry puts it back.
package codestream

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
)

// ChunkLines is how many source lines a completed chunk holds.
const ChunkLines = 40

// Renderer renders one whole Markdown document (*glamour.TermRenderer).
type Renderer interface {
	Render(string) (string, error)
}

// Fence renders a document whose last block is a fenced code block that
// grows at its end. Use one Fence per Renderer and per growing fence; a src
// that does not start as the last one did resets the chunk cache. A Fence
// is not safe for concurrent use.
type Fence struct {
	size  int
	split blocks.Splitter
	// fence: where the fence opens in the last src, and how it is read.
	at      int    // offset of the fence's opening line
	opening string // opening line, no "\n"
	closer  string // the fence run: a line that closes it
	head    string // src[:at] + opening + "\n"
	// cal: line counts read from renders of a one-line fence.
	calKey string
	cal    calibration
	chunks []chunk
	// spliced: the last Frame was put together from chunks (tests).
	spliced bool
}

type calibration struct {
	ok bool
	// alone: lines before the body in a render of the fence alone.
	alone int
	// withPre: lines before the body with the text before the fence.
	withPre int
	// tail: lines after the body (same in both).
	tail int
}

type chunk struct {
	src  string
	body []string
}

// New returns a Fence with ChunkLines-line chunks.
func New() *Fence { return &Fence{size: ChunkLines} }

// Spliced reports whether the last Frame was put together from chunks (and
// so may differ from the whole render in SGR bytes).
func (f *Fence) Spliced() bool { return f.spliced }

// Final renders src whole: exact.
func (f *Fence) Final(src string, r Renderer) (string, error) { return r.Render(src) }

// Frame renders src while the fence grows. When src's last block is not a
// fenced code block, or the fence has one chunk only, it is r.Render(src).
func (f *Fence) Frame(src string, r Renderer) (string, error) {
	f.spliced = false
	at, opening, closer, ok := f.locate(src)
	if !ok {
		return r.Render(src)
	}
	bodyAt := at + len(opening) + 1
	content, ok := codeContent(src[bodyAt:], closer)
	if !ok {
		// Text after the closing line: the fence is not the last block.
		f.head = ""
		return r.Render(src)
	}
	// starts: byte offset in content of each source line.
	starts := lineStarts(content)
	if len(starts) <= f.size {
		return r.Render(src)
	}
	// done: completed chunks; the open chunk keeps at least one line.
	done := (len(starts) - 1) / f.size
	pre := src[:at]
	if key := pre + "\x00" + opening; key != f.calKey {
		cal, err := calibrate(r, pre, opening, closer)
		if err != nil {
			return "", err
		}
		f.calKey, f.cal, f.chunks = key, cal, nil
	}
	if !f.cal.ok {
		return r.Render(src)
	}
	if len(f.chunks) > done {
		f.chunks = f.chunks[:done]
	}
	for i := 0; i < done; i++ {
		csrc := content[starts[i*f.size]:starts[(i+1)*f.size]]
		if i < len(f.chunks) && f.chunks[i].src == csrc {
			continue
		}
		out, err := r.Render(opening + "\n" + csrc + closer + "\n")
		if err != nil {
			return "", err
		}
		ls := strings.Split(out, "\n")
		if len(ls) < f.cal.alone+f.cal.tail {
			return r.Render(src)
		}
		c := chunk{strings.Clone(csrc), ls[f.cal.alone : len(ls)-f.cal.tail]}
		f.chunks = append(f.chunks[:i], c)
	}
	openAt := bodyAt + starts[done*f.size]
	out, err := r.Render(pre + opening + "\n" + src[openAt:])
	if err != nil {
		return "", err
	}
	ls := strings.Split(out, "\n")
	if len(ls) < f.cal.withPre+f.cal.tail {
		return r.Render(src)
	}
	head, body, tail := ls[:f.cal.withPre], ls[f.cal.withPre:len(ls)-f.cal.tail], ls[len(ls)-f.cal.tail:]
	n := len(head) + len(body) + len(tail)
	for _, c := range f.chunks {
		n += len(c.body)
	}
	lines := make([]string, 0, n)
	lines = append(lines, head...)
	for i, c := range f.chunks {
		lines = appendBody(lines, c.body, i > 0)
	}
	lines = appendBody(lines, body, true)
	lines = append(lines, tail...)
	f.spliced = true
	return strings.Join(lines, "\n"), nil
}

// appendBody appends body to lines; carry gives body's first line the pen
// of the line before it (every chunk but the first).
func appendBody(lines, body []string, carry bool) []string {
	if len(body) == 0 {
		return lines
	}
	first := body[0]
	if carry && len(lines) > 0 {
		first = penCarry(first, lines[len(lines)-1])
	}
	lines = append(lines, first)
	return append(lines, body[1:]...)
}

// fenceRe is blocks' fence line: up to three spaces, a run of three or
// more backticks or tildes, the info string.
var fenceRe = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")

// locate returns the offset, opening line and fence run of the fenced code
// block that is src's last block.
func (f *Fence) locate(src string) (at int, opening, closer string, ok bool) {
	// The fence opened in an earlier src: blocks before it are final, as
	// long as src starts as that src did up to the fence's first line.
	if f.head != "" && strings.HasPrefix(src, f.head) {
		return f.at, f.opening, f.closer, true
	}
	f.head = ""
	bs := f.split.Split(src)
	if len(bs) == 0 {
		return 0, "", "", false
	}
	last := bs[len(bs)-1]
	nl := strings.IndexByte(last.Source, '\n')
	if nl < 0 {
		return 0, "", "", false
	}
	line := last.Source[:nl]
	m := fenceRe.FindStringSubmatch(line)
	if m == nil || (m[1][0] == '`' && strings.Contains(m[2], "`")) {
		return 0, "", "", false
	}
	f.at, f.opening, f.closer = last.Offset, line, m[1]
	f.head = src[:last.Offset+nl+1]
	return f.at, f.opening, f.closer, true
}

// codeContent returns the code lines of a fence, given rest, the text after
// its opening line. ok is false when text other than blank lines follows
// the closing line (the fence is not the last block).
func codeContent(rest, run string) (content string, ok bool) {
	for i := 0; i < len(rest); {
		j := strings.IndexByte(rest[i:], '\n')
		end := len(rest)
		if j >= 0 {
			end = i + j
		}
		if closes(rest[i:end], run) {
			return rest[:i], strings.TrimSpace(rest[end:]) == ""
		}
		if j < 0 {
			break
		}
		i = end + 1
	}
	return rest, true
}

// closes reports whether line closes a fence opened by run.
func closes(line, run string) bool {
	body := strings.TrimLeft(line, " ")
	if len(line)-len(body) > 3 {
		return false
	}
	n := 0
	for n < len(body) && body[n] == run[0] {
		n++
	}
	return n >= len(run) && strings.TrimSpace(body[n:]) == ""
}

// lineStarts returns the offset of each line of s; a final "\n" starts no
// line.
func lineStarts(s string) []int {
	if s == "" {
		return nil
	}
	starts := []int{0}
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// calibrate reads where the body starts and ends from renders of a
// one-line fence, alone and after pre. cal.ok is false when the two renders
// do not have the shape Frame needs.
func calibrate(r Renderer, pre, opening, run string) (calibration, error) {
	md := opening + "\nx\n" + run + "\n"
	alone, err := r.Render(md)
	if err != nil {
		return calibration{}, err
	}
	withPre, err := r.Render(pre + md)
	if err != nil {
		return calibration{}, err
	}
	a, w := strings.Split(alone, "\n"), strings.Split(withPre, "\n")
	at := -1
	for i, l := range a {
		if strings.TrimSpace(ansi.Strip(l)) != "" {
			if at >= 0 {
				return calibration{}, nil
			}
			at = i
		}
	}
	if at < 0 {
		return calibration{}, nil
	}
	tail := len(a) - at - 1
	wat := len(w) - tail - 1
	if wat < 0 || w[wat] != a[at] || strings.Join(w[wat:], "\n") != strings.Join(a[at:], "\n") {
		return calibration{}, nil
	}
	return calibration{ok: true, alone: at, withPre: wat, tail: tail}, nil
}

var (
	// padRe: the pad cells glamour writes after a line's text.
	padRe = regexp.MustCompile(`(?:\x1b\[[0-9;]*m \x1b\[m)+$`)
	// leadPadRe: the indent cells glamour writes before a line's text.
	leadPadRe = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m \x1b\[m)*`)
	// emptyRunRe: an empty styled run, where a blank line's text would be.
	emptyRunRe = regexp.MustCompile(`^\x1b\[[0-9;]*m\x1b\[m`)
	sgrRe      = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

// penCarry gives line, the first line of a chunk rendered alone, the pen
// the whole render starts it with: the SGR run open at the end of prev.
// After the margin the whole render writes pen ESC[m; after the indent
// cells, on a line with text, pen ESC[0m (the end of the token that held
// the newline). Exact when the chunk seam is a token boundary and the lexer
// is in its root state there; else only SGR bytes differ.
func penCarry(line, prev string) string {
	pen := penOf(prev)
	if pen == "" {
		return line
	}
	margin := len(line) - len(strings.TrimLeft(line, " "))
	rest := line[margin:]
	pads := leadPadRe.FindString(rest)
	rest = rest[len(pads):]
	var b strings.Builder
	b.Grow(len(line) + 2*len(pen) + 7)
	b.WriteString(line[:margin])
	b.WriteString(pen)
	b.WriteString("\x1b[m")
	b.WriteString(pads)
	if rest != "" && !emptyRunRe.MatchString(rest) && !continuesPen(rest, pen) {
		b.WriteString(pen)
		b.WriteString("\x1b[0m")
	}
	b.WriteString(rest)
	return b.String()
}

// continuesPen reports whether rest starts with whitespace in pen: chroma
// coalesces the newline token with following whitespace of the same type,
// so the whole render has the newline's token go on into this line and
// writes no pen ESC[0m.
func continuesPen(rest, pen string) bool {
	if !strings.HasPrefix(rest, pen) || len(rest) == len(pen) {
		return false
	}
	c := rest[len(pen)]
	return c == ' ' || c == '\t'
}

// penOf returns the last run of SGR sequences in line that is not a reset,
// pad cells at the end left out; "" when there is none.
func penOf(line string) string {
	line = padRe.ReplaceAllString(line, "")
	idx := sgrRe.FindAllStringIndex(line, -1)
	isReset := func(i int) bool {
		s := line[idx[i][0]:idx[i][1]]
		return s == "\x1b[0m" || s == "\x1b[m"
	}
	last := -1
	for i := len(idx) - 1; i >= 0; i-- {
		if !isReset(i) {
			last = i
			break
		}
	}
	if last < 0 {
		return ""
	}
	first := last
	for first > 0 && !isReset(first-1) && idx[first-1][1] == idx[first][0] {
		first--
	}
	return line[idx[first][0]:idx[last][1]]
}
