package bench

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/glamour/v2/styles"
	"github.com/iodesystems/bubbleteandmd"
	llxcorpus "github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// codeLines: n lines of Go, every 3rd a removed/added pair when diff.
func codeLines(n int, diff bool) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		line := fmt.Sprintf("\tv%d := compute(alpha, beta%d) // step %d", i, i%7, i)
		switch {
		case !diff:
			b.WriteString(line + "\n")
		case i%3 == 1:
			b.WriteString("-" + line + "\n")
		case i%3 == 2:
			b.WriteString("+" + strings.Replace(line, "beta", "gamma", 1) + "\n")
		default:
			b.WriteString(" " + line + "\n")
		}
	}
	return b.String()
}

// diffReplies: a 2000-line ```diff fence (titled: Go colours, word diff)
// and the same lines as a plain ```go fence (codestream chunks).
func diffReplies(n int) []struct{ name, text string } {
	return []struct{ name, text string }{
		{"diff-titled", "Here:\n\n```diff title=\"main.go\"\n" + codeLines(n, true) + "```\n"},
		{"diff-untitled", "Here:\n\n```diff\n" + codeLines(n, true) + "```\n"},
		{"go-fence", "Here:\n\n```go\n" + codeLines(n, false) + "```\n"},
	}
}

// BenchmarkDiffTailFrame: the cost of one Frame of a reply whose last block
// is a 2000-line fence, warm (the frame before it was drawn): ns/op is per
// frame. Each op adds one byte of the fence's last line, alternating.
func BenchmarkDiffTailFrame(b *testing.B) {
	for _, sc := range diffReplies(2000) {
		b.Run(sc.name, func(b *testing.B) {
			r, err := bubbleteandmd.NewRenderer(styles.DarkStyleConfig, 78)
			if err != nil {
				b.Fatal(err)
			}
			s := bubbleteandmd.NewStream(r, "dark/78", bubbleteandmd.NewCache(512))
			end := strings.LastIndex(sc.text, "\n```")
			ps := []string{sc.text[:end-4], sc.text[:end-3]}
			for _, p := range ps {
				if _, err := s.Frame(p); err != nil {
					b.Fatal(err)
				}
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := s.Frame(ps[i%2]); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkDiffStream: one op streams the whole reply at frame prefixes
// (corpus.FramePrefixes) and calls Final, as BenchmarkWideStream.
func BenchmarkDiffStream(b *testing.B) {
	for _, sc := range diffReplies(2000) {
		frames := llxcorpus.FramePrefixes(sc.text)
		b.Run(sc.name, func(b *testing.B) {
			b.ReportMetric(float64(len(frames)), "frames/op")
			for i := 0; i < b.N; i++ {
				r, err := bubbleteandmd.NewRenderer(styles.DarkStyleConfig, 78)
				if err != nil {
					b.Fatal(err)
				}
				s := bubbleteandmd.NewStream(r, "dark/78", bubbleteandmd.NewCache(512))
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
