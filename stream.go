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
// (no Sentinel before it).
type cacheKey struct {
	key    string
	source string
	first  bool
}

// Cache holds rendered stable blocks, keyed by (Stream key, block source).
// One Cache can serve many Streams; it is safe for concurrent use.
type Cache struct {
	mu  sync.Mutex
	lru *lru.Cache[cacheKey, string]
	// put, when set (tests), sees every source stored.
	put func(source string)
}

// NewCache returns a cache of at most max rendered blocks.
func NewCache(max int) *Cache {
	return &Cache{lru: lru.New[cacheKey, string](max)}
}

func (c *Cache) get(k cacheKey) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Get(k)
}

func (c *Cache) store(k cacheKey, v string) {
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
// A Stream is not safe for concurrent use.
type Stream struct {
	r     Renderer
	key   string
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

// memo is one rendered block, keyed by its source and whether it is first.
type memo struct {
	source, piece string
	first, ok     bool
}

// NewStream returns a Stream that renders with r and caches stable blocks
// in c under key. key must name everything that changes r's output (dun:
// width and style). c may be shared by Streams; nil means no cache.
func NewStream(r Renderer, key string, c *Cache) *Stream {
	s := &Stream{r: r, key: key, cache: c}
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

func (s *Stream) render(src string, healed bool) (string, error) {
	if !s.ready {
		if err := s.setup(); err != nil {
			return "", err
		}
		s.ready = true
	}
	s.stats.lastFence, s.stats.lastHeld = false, false
	// Before split, heal and cache, so all three see clean text.
	src = s.sanitize(src)
	if s.seen != nil {
		s.seen(src)
	}
	bs := s.split.Split(src)
	stable := max(0, len(bs)-2)
	pieces := make([]string, 0, len(bs))
	for _, b := range bs[:stable] {
		piece, err := s.block(b.Source, b.Offset == 0)
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
				piece, err := s.nearBlock(near.Source, near.Offset == 0)
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
	return join.Join(s.p, s.s, pieces), nil
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
	// glamour ends every paragraph line with "\n"; with no "\n" each piece
	// would join as an open line (join.Join drops cells), and the output
	// would be wrong with no error.
	probe, err := s.piece("x\n", false)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(probe, "\n") {
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
// text can still change it, so it is memoized by its source alone.
func (s *Stream) nearBlock(source string, first bool) (string, error) {
	if s.near.ok && s.near.first == first && s.near.source == source {
		return s.near.piece, nil
	}
	v, err := s.piece(source, first)
	if err != nil {
		return "", err
	}
	s.near = memo{source: source, piece: v, first: first, ok: true}
	return v, nil
}

// block renders a stable block, from the cache when it is there.
func (s *Stream) block(source string, first bool) (string, error) {
	if s.cache == nil {
		return s.piece(source, first)
	}
	k := cacheKey{s.key, source, first}
	if v, ok := s.cache.get(k); ok {
		return v, nil
	}
	var v string
	if s.near.ok && s.near.first == first && s.near.source == source {
		v = s.near.piece // rendered while it was the block before the last
	} else {
		var err error
		if v, err = s.piece(source, first); err != nil {
			return "", err
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
func (s *Stream) tailPiece(tail string, at int, last, healed bool) (string, error) {
	first := at == 0
	if last && healed && heldTable(tail) {
		s.stats.held++
		s.stats.lastHeld = true
		return s.holdPiece(first)
	}
	if last && healed && s.plainFence(tail) {
		if s.fence == nil {
			s.fence = codestream.New()
		}
		out, err := s.fence.Frame(join.Input(tail, first), s.r)
		if err != nil {
			return "", err
		}
		if s.fence.Spliced() {
			s.stats.fenceSpliced++
			s.stats.lastFence = true
		}
		return join.Strip(out, s.p, s.s)
	}
	if !s.tables || !last || !strings.HasPrefix(tail, "|") {
		return s.piece(tail, first)
	}
	// A wide table (DirectiveRenderer.WideBlocks) grows through the wide
	// renderer at its own width; its lines are cut as Render cuts them.
	var r Renderer = s.r
	opts, wide, cut := s.gopts, 0, func(p string) string { return p }
	if dr, ok := s.r.(*DirectiveRenderer); ok {
		t, o, ok, err := dr.wideTable(tail)
		if err != nil {
			return "", err
		}
		if ok {
			r, opts, wide, cut = t.g, o, o.WordWrap, dr.cutLines
		}
	}
	if s.table == nil || s.tableAt != at || s.tableW != wide {
		s.table, s.tableAt, s.tableW = tablestream.New(opts), at, wide
	}
	before := s.table.Stats.Spliced
	out, err := s.table.Render(join.Input(tail, first), r)
	if err != nil {
		return "", err
	}
	s.stats.tableSpliced += s.table.Stats.Spliced - before
	piece, err := join.Strip(out, s.p, s.s)
	return cut(piece), err
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
		s.plainOpen, s.plain = line, !form
	}
	return s.plain
}

func (s *Stream) piece(source string, first bool) (string, error) {
	out, err := s.r.Render(join.Input(source, first))
	if err != nil {
		return "", err
	}
	return join.Strip(out, s.p, s.s)
}
