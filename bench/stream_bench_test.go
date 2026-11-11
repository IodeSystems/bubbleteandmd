package bench

import (
	"fmt"
	"testing"

	"github.com/iodesystems/bubbleteandmd"
)

// BenchmarkStreamFrame: per-frame cost of Stream.Frame on a doc that has
// grown to n chars, with every block before the tail already cached (as
// after streaming up to n). Compare with BenchmarkRender (whole doc).
// Each iteration appends one 20-char chunk to the doc at n, so the tail
// changes per frame as it does while streaming.
func BenchmarkStreamFrame(b *testing.B) {
	for _, w := range []int{78, 98} {
		for _, n := range []int{10000, 25000, 100000} {
			full := corpus(n + 8000) // cut back to a block end; >= n+4000
			src := full[:n]
			b.Run(fmt.Sprintf("w%d/n%d", w, n), func(b *testing.B) {
				r := newRenderer(w)
				c := bubbleteandmd.NewCache(4096)
				s := bubbleteandmd.NewStream(r, fmt.Sprint(w), c)
				if _, err := s.Frame(src); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					end := n + 20*(i%200+1)
					if _, err := s.Frame(full[:end]); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
