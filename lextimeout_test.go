package bubbleteandmd

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/dlclark/regexp2/v2"
)

// TestMain stops regexp2's match timeouts in this test binary.
//
// Why: chroma v2.27.0 gives every lexer rule a 250 ms regexp2 MatchTimeout
// (regexp.go:375) on a wall clock, and takes a timed-out rule as no match,
// so the lexer writes an Error token (regexp.go:258, 502). A goroutine that
// is off the CPU for 250 ms in the middle of a match times out: under load
// (the full suite on the M1, t.Parallel) valid code now and then gets one
// character in the Error style, and the invariant tests see two renders of
// the same text differ. Measured: TestLexUnderLoad without this, 535 of
// 16711 lexes (dev box, 2000 goroutines) and 460 of 17608 (M1, 1000).
//
// How: regexp2 reads the time from a clock goroutine that sleeps the check
// period between reads. With a period of 100 years the clock does not move
// after it starts, so no deadline is reached. Only tests: the library does
// not change regexp2 settings of the process it runs in.
func TestMain(m *testing.M) {
	regexp2.SetTimeoutCheckPeriod(100 * 365 * 24 * time.Hour)
	os.Exit(m.Run())
}

// TestLexUnderLoad: many goroutines lex valid Go source at once and get no
// Error token (TestMain's regexp2 setting holds).
func TestLexUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("3 s under load")
	}
	src := wideCode(120)
	src = src[len("```go\n") : len(src)-len("```\n")]
	lx := chroma.Coalesce(lexers.Get("go"))
	errs := func() int {
		it, err := lx.Tokenise(nil, src)
		if err != nil {
			t.Error(err)
			return 0
		}
		n := 0
		for tok := it(); tok != chroma.EOF; tok = it() {
			if tok.Type == chroma.Error {
				n++
			}
		}
		return n
	}
	if n := errs(); n != 0 {
		t.Fatalf("%d Error tokens in valid source", n)
	}
	var runs, bad atomic.Int64
	var wg sync.WaitGroup
	end := time.Now().Add(3 * time.Second)
	for i := 0; i < 2000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(end) {
				runs.Add(1)
				if errs() != 0 {
					bad.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if bad.Load() != 0 {
		t.Errorf("%d of %d lexes have Error tokens", bad.Load(), runs.Load())
	}
}
