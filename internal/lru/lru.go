// Package lru is a small bounded least-recently-used cache.
//
// Port of llx-markdown's core/bounded-cache.ts, count bound only. The TS
// weight bound (maxWeight/weigh) is not ported; add it when a caller needs it.
//
// Not goroutine-safe. The TS original is not either (a plain Map), so callers
// that share a Cache across goroutines must hold their own lock.
//
// Recency: Get moves a hit to the front. Put of a new key pushes it to the
// front; Put of an existing key updates the value and moves it to the front.
// When Len exceeds max, the entry at the back (least recent) is evicted.
package lru

import "container/list"

// Cache is a bounded LRU map from K to V.
type Cache[K comparable, V any] struct {
	max   int
	order *list.List // front = most recent; elements hold *entry[K, V]
	index map[K]*list.Element
}

type entry[K comparable, V any] struct {
	key   K
	value V
}

// New returns an empty cache holding at most max entries.
func New[K comparable, V any](max int) *Cache[K, V] {
	return &Cache[K, V]{
		max:   max,
		order: list.New(),
		index: make(map[K]*list.Element),
	}
}

// Get returns the value for k and marks k as most recently used.
func (c *Cache[K, V]) Get(k K) (V, bool) {
	el, ok := c.index[k]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*entry[K, V]).value, true
}

// Put stores v under k as most recently used, evicting the least recently
// used entries while Len exceeds max.
func (c *Cache[K, V]) Put(k K, v V) {
	if el, ok := c.index[k]; ok {
		el.Value.(*entry[K, V]).value = v
		c.order.MoveToFront(el)
		return
	}
	c.index[k] = c.order.PushFront(&entry[K, V]{key: k, value: v})
	for c.order.Len() > c.max {
		back := c.order.Back()
		c.order.Remove(back)
		delete(c.index, back.Value.(*entry[K, V]).key)
	}
}

// Len returns the number of entries.
func (c *Cache[K, V]) Len() int {
	return c.order.Len()
}
