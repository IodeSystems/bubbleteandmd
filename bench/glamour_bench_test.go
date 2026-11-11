// Package bench measures glamour v2 whole-document render cost against reply
// length: the per-frame cost a streaming TUI pays when it re-renders the
// whole reply each frame. Options mirror dun's newMarkdown (dark standard
// style, word wrap at width).
package bench

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"

	"charm.land/glamour/v2"
)

//go:embed testdata/reply.md
var template string

// corpus repeats the template (section number varied) to at least n chars,
// cut at a block boundary (blank line).
func corpus(n int) string {
	var b strings.Builder
	for i := 1; b.Len() < n; i++ {
		fmt.Fprintf(&b, template, i)
	}
	s := b.String()
	if len(s) > n {
		if k := strings.LastIndex(s[:n], "\n\n"); k > 0 {
			s = s[:k+2]
		}
	}
	return s
}

func newRenderer(w int) *glamour.TermRenderer {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(w),
	)
	if err != nil {
		panic(err)
	}
	return r
}

// dun widths: min(term,100)-2 prose column: 98 (wide), 78 (80 cols), 48 (narrow pane).
var widths = []int{48, 78, 98}
var sizes = []int{1000, 5000, 10000, 25000, 50000, 100000}

func BenchmarkRender(b *testing.B) {
	for _, w := range widths {
		for _, n := range sizes {
			src := corpus(n)
			b.Run(fmt.Sprintf("w%d/n%d", w, n), func(b *testing.B) {
				r := newRenderer(w)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := r.Render(src); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkNewRenderer: construction cost. dun builds one per resize (and
// per new wide-block width, cached), not per frame.
func BenchmarkNewRenderer(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		newRenderer(98)
	}
}
