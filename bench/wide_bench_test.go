package bench

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/glamour/v2/styles"
	"github.com/iodesystems/bubbleteandmd"
	llxcorpus "github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// wideCode: a reply that writes one fence of n lines, every 7th line 150
// columns or more (wider than 78 and 98).
func wideCode(n int) string {
	var b strings.Builder
	b.WriteString("Here is the file:\n\n```go\n")
	for i := 0; i < n; i++ {
		if i%7 == 3 {
			fmt.Fprintf(&b, "\tlog.Printf(\"step %d: %s\", %s) // tail %d\n", i, strings.Repeat("value ", 18), strings.Repeat("arg, ", 6), i)
			continue
		}
		fmt.Fprintf(&b, "\tx%d := f(%d)\n", i, i)
	}
	b.WriteString("```\n\nDone.\n")
	return b.String()
}

// wideTable: a reply that ends in one table of n rows, its last column
// 60-170 columns wide (the natural width grows while it streams).
func wideTable(n int) string {
	var b strings.Builder
	b.WriteString("Results:\n\n| # | name | what it does |\n| ---: | --- | --- |\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "| %d | tool-%d | %s[docs](https://example.com/%d) end %d |\n", i, i, strings.Repeat("code navigation ", 3+i%5+i/20), i%4, i)
	}
	return b.String()
}

// BenchmarkWideStream: one op streams a whole reply at frame prefixes
// (corpus.FramePrefixes) through NewRenderer at w78, with wide blocks off
// (the block wraps) and on (pane 78: cut, not wrapped), then calls Final.
func BenchmarkWideStream(b *testing.B) {
	for _, sc := range []struct{ name, text string }{
		{"wide-code-600", wideCode(600)},
		{"wide-table-160", wideTable(160)},
	} {
		frames := llxcorpus.FramePrefixes(sc.text)
		for _, pane := range []int{0, 78} {
			b.Run(fmt.Sprintf("%s/pane%d", sc.name, pane), func(b *testing.B) {
				b.ReportMetric(float64(len(frames)), "frames/op")
				for i := 0; i < b.N; i++ {
					r, err := bubbleteandmd.NewRenderer(styles.DarkStyleConfig, 78)
					if err != nil {
						b.Fatal(err)
					}
					r.WideBlocks(pane)
					s := bubbleteandmd.NewStream(r, fmt.Sprint("dark/78/", pane), bubbleteandmd.NewCache(512))
					for _, p := range frames {
						if _, err := s.Frame(p); err != nil {
							b.Fatal(err)
						}
					}
					if _, err := s.Final(sc.text); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
