package bubbleteandmd

import (
	"sync"

	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
	"github.com/iodesystems/bubbleteandmd/internal/join"
	"github.com/iodesystems/bubbleteandmd/internal/lru"
)

// Renderer renders one whole Markdown document. *glamour.TermRenderer
// satisfies it. A Renderer is used by one goroutine at a time.
type Renderer interface {
	Render(string) (string, error)
}

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
// last two are rendered once and cached; the last two (the tail) are
// rendered on every call. Frame heals the tail; Final does not. The result
// equals r.Render of the whole (healed, for Frame) text.
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
	// seen, when set (tests), sees every text given to the splitter.
	seen func(src string)
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

// Final renders src as written, with no healing.
func (s *Stream) Final(src string) (string, error) { return s.render(src, false) }

func (s *Stream) render(src string, healed bool) (string, error) {
	if !s.ready {
		empty, err := s.r.Render("")
		if err != nil {
			return "", err
		}
		if s.p, s.s, err = join.Affixes(empty); err != nil {
			return "", err
		}
		s.ready = true
	}
	if s.seen != nil {
		s.seen(src)
	}
	bs := s.split.Split(src)
	stable := max(0, len(bs)-2)
	pieces := make([]string, 0, stable+1)
	for _, b := range bs[:stable] {
		piece, err := s.block(b.Source, b.Offset == 0)
		if err != nil {
			return "", err
		}
		pieces = append(pieces, piece)
	}
	tailAt := 0
	if stable > 0 {
		tailAt = bs[stable].Offset
	}
	tail := src[tailAt:]
	if healed {
		// heal.Close of the tail alone equals the tail of heal.Close(src):
		// a block boundary is never inside a construct heal completes
		// (checked by TestInvariant).
		tail = heal.Close(tail)
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

// block renders a stable block, from the cache when it is there.
func (s *Stream) block(source string, first bool) (string, error) {
	if s.cache == nil {
		return s.piece(source, first)
	}
	k := cacheKey{s.key, source, first}
	if v, ok := s.cache.get(k); ok {
		return v, nil
	}
	v, err := s.piece(source, first)
	if err != nil {
		return "", err
	}
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
