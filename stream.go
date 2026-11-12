package bubbleteandmd

import (
	"errors"
	"regexp"
	"strings"
	"sync"

	"charm.land/glamour/v2/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/codestream"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
	"github.com/iodesystems/bubbleteandmd/internal/join"
	"github.com/iodesystems/bubbleteandmd/internal/lru"
	"github.com/iodesystems/bubbleteandmd/internal/tablestream"
)

// Renderer renders one whole Markdown document. *glamour.TermRenderer
// satisfies it. A Renderer is used by one goroutine at a time.
//
// Pass a raw *glamour.TermRenderer (or a DirectiveRenderer), not a wrapper
// that changes its output: Stream renders each block alone and joins the
// pieces by the document prefix, suffix and line ends glamour writes. A
// wrapper that trims the newlines makes Frame and Final return ErrTrimmed.
// Do pre-processing on src before Frame; do post-processing on the output
// of Frame and Final.
type Renderer interface {
	Render(string) (string, error)
}

// GlamourOptions is an optional interface of a Renderer: the glamour options
// it renders a plain Markdown table with. ok is false when they are not
// known (for example, glamour options were given that cannot be read back).
// When ok, Stream renders a growing table in the tail from cached rows
// (internal/tablestream), with the same bytes as r.Render; without it, the
// tail renders whole on every call. *DirectiveRenderer implements it.
type GlamourOptions interface {
	GlamourOptions() (opts ansi.Options, ok bool)
}

// ErrTrimmed is returned by Frame and Final when the Renderer ends a
// paragraph without its newline (a wrapper that trims glamour's output):
// the blocks cannot be joined as the whole document renders.
var ErrTrimmed = errors.New("bubbleteandmd: Renderer output ends a paragraph without a newline; pass a raw *glamour.TermRenderer")

// cacheKey: a block renders differently when it is first in the document
// (no Sentinel before it). With a StateRenderer, at (the block's offset:
// mark keys hold it) and state (stateKey of the block) are in the key too.
type cacheKey struct {
	key    string
	source string
	first  bool
	at     int
	state  string
}

// Cache holds rendered stable blocks, keyed by (Stream key, block source).
// One Cache can serve many Streams; it is safe for concurrent use.
type Cache struct {
	mu  sync.Mutex
	lru *lru.Cache[cacheKey, rendered]
	// put, when set (tests), sees every source stored.
	put func(source string)
}

// NewCache returns a cache of at most max rendered blocks.
func NewCache(max int) *Cache {
	return &Cache{lru: lru.New[cacheKey, rendered](max)}
}

func (c *Cache) get(k cacheKey) (rendered, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Get(k)
}

func (c *Cache) store(k cacheKey, v rendered) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.put != nil {
		c.put(k.source)
	}
	c.lru.Put(k, v)
}

// Stream renders one document that grows at its end. Blocks before the
// last two are rendered once and cached; the block before the last is
// rendered when its source changes (the splitter can still reopen it); the
// last block (the tail) is rendered on every call. Both first remove
// terminal controls (Sanitize). Frame heals the tail; Final does not. The
// result equals r.Render of the whole sanitized (and healed, for Frame)
// text, with one exception (Carl B1): while a code fence of more than
// codestream.ChunkLines lines grows in the tail, Frame renders it in chunks,
// and SGR bytes (colours at a chunk seam) can differ; lines and their text
// do not. Final is exact. A growing table tail renders from cached rows,
// exact, when the Renderer tells its options (GlamourOptions).
//
// With a StateRenderer, every block renders in the State given to SetState
// (Final equals RenderState of the whole sanitized text), and Marks returns
// where the interactive parts of the last output are.
//
// A Stream is not safe for concurrent use.
type Stream struct {
	r   Renderer
	key string
	// sr is r when it is a StateRenderer; st is the State it draws with;
	// marks: the marks of the last output; pl: the "\n" count of p.
	sr    StateRenderer
	st    *State
	marks []Mark
	pl    int
	cache *Cache
	split blocks.Splitter
	// p, s: document prefix and suffix, from the render of "".
	p, s  string
	ready bool
	// err: the Renderer cannot be joined (setup); returned by every call.
	err error
	// near: the last render of the block before the last. The splitter can
	// still change that block, so it is kept here, keyed by its source, and
	// not in the shared Cache until it is stable.
	near memo
	// raw, clean: the previous src up to its last "\n", and Sanitize of it.
	// Sanitize(raw+x) == clean+Sanitize(x), so only the new text is scanned.
	raw, clean string
	// gopts: the Renderer's glamour options, when it tells them
	// (GlamourOptions); table: the Table of the tail block at tableAt.
	gopts   ansi.Options
	tables  bool
	table   *tablestream.Table
	tableAt int
	// tableW: the wrap width table renders at (a wide table, WideBlocks), or 0.
	tableW int
	// fence renders a growing code fence tail in Frame; plainOpen and
	// plain: whether the last opening line seen draws as plain glamour
	// (not a form) with a *DirectiveRenderer.
	fence     *codestream.Fence
	plainOpen string
	plain     bool
	// stats counts how tails were served (tests).
	stats streamStats
	// seen, when set (tests), sees every text given to the splitter.
	seen func(src string)
}

// streamStats counts tail renders served by a fast path.
type streamStats struct {
	// tableSpliced: tail tables served from cached rows.
	tableSpliced int
	// held: Frame tails drawn as the table placeholder; lastHeld: the last
	// call was one (tablehold.go).
	held     int
	lastHeld bool
	// fenceSpliced: Frame tails put together from code chunks; lastFence:
	// the last call was one (its output may differ from the whole render
	// in SGR bytes only, Carl B1).
	fenceSpliced int
	lastFence    bool
}

// memo is one rendered block, keyed by its source, whether it is first,
// its offset and its stateKey (the last two with a StateRenderer only).
type memo struct {
	source, state string
	piece         rendered
	at            int
	first, ok     bool
}

func (m *memo) hit(source, state string, at int, first bool) bool {
	return m.ok && m.first == first && m.at == at && m.state == state && m.source == source
}

// rendered is one block's piece and the marks in it, lines counted from
// the piece's first line.
type rendered struct {
	text  string
	marks []Mark
}

// NewStream returns a Stream that renders with r and caches stable blocks
// in c under key. key must name everything that changes r's output (dun:
// width and style). c may be shared by Streams; nil means no cache.
func NewStream(r Renderer, key string, c *Cache) *Stream {
	s := &Stream{r: r, key: key, cache: c}
	s.sr, _ = r.(StateRenderer)
	if g, ok := r.(GlamourOptions); ok {
		s.gopts, s.tables = g.GlamourOptions()
	}
	return s
}

// Frame renders src while it is still streaming: the tail is healed
// (heal.Close), so half-written syntax does not show raw.
func (s *Stream) Frame(src string) (string, error) { return s.render(src, true) }

// Final renders src as written, with no healing; only Sanitize applies.
func (s *Stream) Final(src string) (string, error) { return s.render(src, false) }

// SetState sets the State the next Frame and Final draw forms in (nil: as
// streamed). It is read on every call, so a change to *st shows on the
// next call; blocks whose part of the State did not change still come from
// the cache. It has an effect only when the Renderer is a StateRenderer.
func (s *Stream) SetState(st *State) { s.st = st }

// Marks returns the marks of the last Frame or Final: where each
// interactive part is drawn, lines counted in that output. nil when the
// Renderer is not a StateRenderer, or the last call failed.
func (s *Stream) Marks() []Mark { return s.marks }

func (s *Stream) render(src string, healed bool) (string, error) {
	if !s.ready {
		if err := s.setup(); err != nil {
			return "", err
		}
		s.ready = true
	}
	s.stats.lastFence, s.stats.lastHeld = false, false
	s.marks = nil
	// Before split, heal and cache, so all three see clean text.
	src = s.sanitize(src)
	if s.seen != nil {
		s.seen(src)
	}
	bs := s.split.Split(src)
	stable := max(0, len(bs)-2)
	pieces := make([]rendered, 0, len(bs))
	for _, b := range bs[:stable] {
		piece, err := s.block(b.Source, b.Offset, b.Offset == 0)
		if err != nil {
			return "", err
		}
		pieces = append(pieces, piece)
	}
	// tailAt: the last block, or the last two when healing the last alone
	// is not the end of healing both (heal drops a callout marker line with
	// the "\n" before it, which is the block before's).
	tailAt, tail := 0, src
	if n := len(bs); n > 0 {
		tailAt, tail = bs[n-1].Offset, src[bs[n-1].Offset:]
		if healed {
			tail = heal.Close(tail)
		}
		if n > 1 {
			near := bs[n-2]
			if healed {
				// heal.Close of the last two blocks equals the tail of
				// heal.Close(src): a block boundary is never inside a
				// construct heal completes (checked by TestInvariant).
				head := src[near.Offset:tailAt]
				if two := heal.Close(src[near.Offset:]); !strings.HasPrefix(two, head) || two[len(head):] != tail {
					tailAt, tail = near.Offset, two
				}
			}
			if tailAt != near.Offset {
				piece, err := s.nearBlock(near.Source, near.Offset, near.Offset == 0)
				if err != nil {
					return "", err
				}
				pieces = append(pieces, piece)
			}
		}
	}
	if tail != "" {
		piece, err := s.tailPiece(tail, tailAt, tailAt == bs[len(bs)-1].Offset, healed)
		if err != nil {
			return "", err
		}
		pieces = append(pieces, piece)
	}
	texts := make([]string, len(pieces))
	var marks []Mark
	line := s.pl
	for i, x := range pieces {
		texts[i] = x.text
		// join drops cells on a shared line, never a "\n".
		for _, m := range x.marks {
			m.Line += line
			marks = append(marks, m)
		}
		line += strings.Count(x.text, "\n")
	}
	s.marks = marks
	return join.Join(s.p, s.s, texts), nil
}

// setup reads the document prefix and suffix and checks that a paragraph
// piece ends with its newline. Errors that are the Renderer's (not glamour's
// render error) are kept and returned by every later call.
func (s *Stream) setup() error {
	if s.err != nil {
		return s.err
	}
	empty, err := s.r.Render("")
	if err != nil {
		return err
	}
	if s.p, s.s, err = join.Affixes(empty); err != nil {
		s.err = err
		return err
	}
	s.pl = strings.Count(s.p, "\n")
	// glamour ends every paragraph line with "\n"; with no "\n" each piece
	// would join as an open line (join.Join drops cells), and the output
	// would be wrong with no error.
	probe, err := s.piece("x\n", 0, false)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(probe.text, "\n") {
		s.err = ErrTrimmed
		return s.err
	}
	return nil
}

// sanitize is Sanitize(src). When src extends the previous src past its
// last "\n", only the text after that "\n" is scanned.
func (s *Stream) sanitize(src string) string {
	var out string
	if s.raw != "" && strings.HasPrefix(src, s.raw) {
		rest := Sanitize(src[len(s.raw):])
		if s.clean == s.raw && rest == src[len(s.raw):] {
			out = src // nothing removed: no copy
		} else {
			out = s.clean + rest
		}
	} else {
		out = Sanitize(src)
	}
	// Keep src[:last "\n"+1]. out has the same "\n" bytes as src (Sanitize
	// keeps every "\n"), so the same cut of out is its sanitized form.
	if i := strings.LastIndexByte(src, '\n'); i >= 0 {
		j := strings.LastIndexByte(out, '\n')
		s.raw, s.clean = src[:i+1], out[:j+1]
	} else {
		s.raw, s.clean = "", ""
	}
	return out
}

// nearBlock renders the block before the last. It is not cached: appended
// text can still change it, so it is memoized by its source (and, with a
// StateRenderer, its offset and state) alone.
func (s *Stream) nearBlock(source string, at int, first bool) (rendered, error) {
	kat, state := s.stateOf(source, at)
	if s.near.hit(source, state, kat, first) {
		return s.near.piece, nil
	}
	v, err := s.piece(source, at, first)
	if err != nil {
		return rendered{}, err
	}
	s.near = memo{source: source, state: state, piece: v, at: kat, first: first, ok: true}
	return v, nil
}

// stateOf returns the offset and stateKey a block of source at offset at
// is keyed by: 0 and "" when the Renderer is not a StateRenderer (its
// output does not depend on them).
func (s *Stream) stateOf(source string, at int) (int, string) {
	if s.sr == nil {
		return 0, ""
	}
	return at, stateKey(s.st, at, at+len(source))
}

// block renders a stable block at offset at, from the cache when it is
// there.
func (s *Stream) block(source string, at int, first bool) (rendered, error) {
	if s.cache == nil {
		return s.piece(source, at, first)
	}
	k := cacheKey{key: s.key, source: source, first: first}
	k.at, k.state = s.stateOf(source, at)
	if v, ok := s.cache.get(k); ok {
		return v, nil
	}
	var v rendered
	if s.near.hit(source, k.state, k.at, first) {
		v = s.near.piece // rendered while it was the block before the last
	} else {
		var err error
		if v, err = s.piece(source, at, first); err != nil {
			return rendered{}, err
		}
	}
	// source is a substring of src: a copy, so the key does not keep the
	// whole text of this frame alive.
	k.source = strings.Clone(source)
	s.cache.store(k, v)
	return v, nil
}

// tailPiece renders the tail at offset at. last: the tail is the last block
// alone (not the last two). When it is the last block: a table renders
// through a Table kept for its offset (exact); in Frame (healed), a code
// fence renders in chunks (codestream: may differ in SGR bytes only, Carl
// B1). In Frame, a table with fewer than holdRows body rows is held: one
// placeholder line (tablehold.go). Any other tail renders whole.
//
// With a StateRenderer, a fast path renders through a tailRenderer: when it
// falls back to the whole render, that render is in the State, with marks.
// A held table, a chunked fence and spliced table rows have no forms.
func (s *Stream) tailPiece(tail string, at int, last, healed bool) (rendered, error) {
	first := at == 0
	if last && healed && heldTable(tail) {
		s.stats.held++
		s.stats.lastHeld = true
		text, err := s.holdPiece(first)
		return rendered{text: text}, err
	}
	fast := last && healed && s.plainFence(tail)
	table := !fast && s.tables && last && strings.HasPrefix(tail, "|")
	if !fast && !table {
		return s.piece(tail, at, first)
	}
	in := join.Input(tail, first)
	var r Renderer = s.r
	var tr *tailRenderer
	if s.sr != nil {
		tr = &tailRenderer{s: s, at: at, whole: in}
		r = tr
	}
	marks := func() []Mark {
		if tr == nil {
			return nil
		}
		return s.pieceMarks(tr.marks)
	}
	if fast {
		if s.fence == nil {
			s.fence = codestream.New()
		}
		text, err := s.strip(s.fence.Frame(in, r))
		if err != nil {
			return rendered{}, err
		}
		if s.fence.Spliced() {
			s.stats.fenceSpliced++
			s.stats.lastFence = true
		}
		return rendered{text, marks()}, nil
	}
	// A wide table (DirectiveRenderer.WideBlocks) grows through the wide
	// renderer at its own width; its lines are cut as Render cuts them. It
	// has no forms (wideTable).
	opts, wide, cut := s.gopts, 0, func(p string) string { return p }
	if dr, ok := s.r.(*DirectiveRenderer); ok {
		t, o, ok, err := dr.wideTable(tail)
		if err != nil {
			return rendered{}, err
		}
		if ok {
			r, opts, wide, cut = t.g, o, o.WordWrap, dr.cutLines
		}
	}
	if s.table == nil || s.tableAt != at || s.tableW != wide {
		s.table, s.tableAt, s.tableW = tablestream.New(opts), at, wide
	}
	before := s.table.Stats.Spliced
	text, err := s.strip(s.table.Render(in, r))
	if err != nil {
		return rendered{}, err
	}
	s.stats.tableSpliced += s.table.Stats.Spliced - before
	return rendered{cut(text), marks()}, nil
}

// fenceOpenRe: a fenced code block's opening line (CommonMark: up to three
// spaces, three or more backticks or tildes, the info string).
var fenceOpenRe = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")

// plainFence reports whether tail opens with a code fence that draws as
// glamour's code block: with a *DirectiveRenderer, a titled fence or a file
// tree is a form, drawn whole.
func (s *Stream) plainFence(tail string) bool {
	line := tail
	if i := strings.IndexByte(tail, '\n'); i >= 0 {
		line = tail[:i]
	} else {
		return false
	}
	m := fenceOpenRe.FindStringSubmatch(line)
	if m == nil || (m[1][0] == '`' && strings.Contains(m[2], "`")) {
		return false
	}
	if _, ok := s.r.(*DirectiveRenderer); !ok {
		return true
	}
	if line != s.plainOpen {
		_, form := directive.Parse(line + "\n" + m[1] + "\n")
		// A diff fence draws line by line (diff.go), not through glamour:
		// its whole render is the cheap one.
		_, diff := diffFence(line + "\n")
		s.plainOpen, s.plain = line, !form && !diff
	}
	return s.plain
}

// piece renders the block source at offset at, alone: with a StateRenderer
// in the Stream's State, with its marks.
func (s *Stream) piece(source string, at int, first bool) (rendered, error) {
	if s.sr == nil {
		text, err := s.strip(s.r.Render(join.Input(source, first)))
		return rendered{text: text}, err
	}
	out, marks, err := s.sr.RenderState(join.Input(source, first), at, s.st)
	text, err := s.strip(out, err)
	if err != nil {
		return rendered{}, err
	}
	return rendered{text, s.pieceMarks(marks)}, nil
}

func (s *Stream) strip(out string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return join.Strip(out, s.p, s.s)
}

// pieceMarks moves marks counted in a render to count in its piece (P
// stripped).
func (s *Stream) pieceMarks(marks []Mark) []Mark {
	for i := range marks {
		marks[i].Line -= s.pl
	}
	return marks
}

// tailRenderer is the Renderer a fast path renders the tail through, with
// a StateRenderer: every render is RenderState at the tail's offset in the
// Stream's State, and the marks of a render of the whole tail (a fast path
// falls back to it) are kept. Its other renders are parts of a code fence
// or a table: no forms, no marks.
type tailRenderer struct {
	s     *Stream
	at    int
	whole string
	marks []Mark
}

func (t *tailRenderer) Render(md string) (string, error) {
	out, marks, err := t.s.sr.RenderState(md, t.at, t.s.st)
	if md == t.whole {
		t.marks = marks
	}
	return out, err
}
