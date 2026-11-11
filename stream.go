package bubbleteandmd

import (
	"errors"
	"strings"
	"sync"

	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
	"github.com/iodesystems/bubbleteandmd/internal/join"
	"github.com/iodesystems/bubbleteandmd/internal/lru"
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
// result equals r.Render of the whole sanitized (and healed, for Frame) text.
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
	// seen, when set (tests), sees every text given to the splitter.
	seen func(src string)
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
	return &Stream{r: r, key: key, cache: c}
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
	// Before split, heal and cache, so all three see clean text.
	src = Sanitize(src)
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
		piece, err := s.piece(tail, tailAt == 0)
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

func (s *Stream) piece(source string, first bool) (string, error) {
	out, err := s.r.Render(join.Input(source, first))
	if err != nil {
		return "", err
	}
	return join.Strip(out, s.p, s.s)
}
