// Ported from llx-markdown packages/llx-markdown/test/random.ts (MIT, see NOTICE).

// Package corpus holds the streaming test corpora, the seeded reply generator and the chunking
// helpers ported from llx-markdown, for invariant tests.
package corpus

import (
	"math"
	"unicode/utf16"
)

// Rand is mulberry32, bit-exact with the TS seeded(): the same seed gives the same sequence.
type Rand struct{ state uint32 }

// Seeded returns a generator for seed.
func Seeded(seed uint32) *Rand { return &Rand{state: seed} }

// Next returns a float64 in [0, 1).
func (r *Rand) Next() float64 {
	// JS: state = (state + 0x6d2b79f5) >>> 0; Math.imul is uint32 wrap-around multiply.
	r.state += 0x6d2b79f5
	t := r.state
	t = (t ^ (t >> 15)) * (t | 1)
	t ^= t + (t^(t>>7))*(t|61)
	return float64(t^(t>>14)) / 4294967296
}

// Int returns an integer in [min, max].
func (r *Rand) Int(min, max int) int {
	return min + int(math.Floor(r.Next()*float64(max-min+1)))
}

// Pick returns a random element of items.
func Pick[T any](r *Rand, items []T) T {
	return items[int(math.Floor(r.Next()*float64(len(items))))]
}

// Hash is FNV-1a over the UTF-16 code units of text, as 8 hex digits (the TS hash()).
func Hash(text string) string {
	h := uint32(0x811c9dc5)
	for _, u := range utf16.Encode([]rune(text)) {
		h = (h ^ uint32(u)) * 0x01000193
	}
	const digits = "0123456789abcdef"
	out := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		out[i] = digits[h&0xf]
		h >>= 4
	}
	return string(out)
}
