package bench

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/iodesystems/bubbleteandmd"
	llxcorpus "github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// TestLLXMethod measures streaming cost the way llx-markdown's bench
// (bench/src/run.ts, measure.tsx, report.ts) does: every growing prefix is
// handed over one update at a time; an update is timed on its own; total is
// the sum of the updates (the first render and the final non-streaming render
// are not in it); p95 is sorted[floor(0.95*n)]; one warm-up on the first 200
// updates; 3 runs, median (lower middle); a renderer that spends the 30 s
// budget is stopped and the table says how far it got.
//
// Skipped unless BENCH_LLX=1. BENCH_LLX_OUT=<file> writes the markdown there
// as well as to the log. BENCH_LLX_ONLY=chat-5k,... limits the scenarios.
func TestLLXMethod(t *testing.T) {
	if os.Getenv("BENCH_LLX") != "1" {
		t.Skip("set BENCH_LLX=1")
	}
	const budget = 30 * time.Second
	const runs = 3
	const frameMs, longMs = 1000.0 / 60, 50.0
	widths := []int{78, 98}

	patterns := map[string][]llxcorpus.Pattern{
		"chat-5k":    {llxcorpus.Token, llxcorpus.Frame},
		"answer-20k": {llxcorpus.Token, llxcorpus.Frame, llxcorpus.Line},
		"code-50k":   {llxcorpus.Token, llxcorpus.Frame},
	}
	prefixes := func(text string, p llxcorpus.Pattern) []string {
		switch p {
		case llxcorpus.Token:
			return llxcorpus.TokenPrefixes(text)
		case llxcorpus.Line:
			return llxcorpus.LinePrefixes(text)
		}
		return llxcorpus.FramePrefixes(text)
	}
	only := map[string]bool{}
	for _, s := range strings.Split(os.Getenv("BENCH_LLX_ONLY"), ",") {
		if s != "" {
			only[s] = true
		}
	}

	type cell struct {
		total, p95, longest float64
		overFrame, overLong float64
		timedOut            bool
		share               int
	}
	median := func(v []float64) float64 {
		s := append([]float64(nil), v...)
		sort.Float64s(s)
		return s[(len(s)-1)/2]
	}
	// measure warms up on the first 200 updates, then streams the whole reply
	// `runs` times. restart is called before each pass.
	measure := func(steps []string, restart func(), send func(string)) cell {
		restart()
		for _, s := range steps[:min(200, len(steps))] {
			send(s)
		}
		var tot, p95, mx, of, ol, drawn []float64
		timedOut := false
		for r := 0; r < runs; r++ {
			restart()
			times := make([]float64, 0, len(steps))
			spent := 0.0
			for _, s := range steps {
				t0 := time.Now()
				send(s)
				d := float64(time.Since(t0).Nanoseconds()) / 1e6
				times = append(times, d)
				spent += d
				if spent > float64(budget.Milliseconds()) {
					break
				}
			}
			if len(times) < len(steps) {
				timedOut = true
			}
			sorted := append([]float64(nil), times...)
			sort.Float64s(sorted)
			var nf, nl float64
			for _, d := range times {
				if d > frameMs {
					nf++
				}
				if d > longMs {
					nl++
				}
			}
			tot = append(tot, spent)
			p95 = append(p95, sorted[min(len(sorted)-1, int(0.95*float64(len(sorted))))])
			mx = append(mx, sorted[len(sorted)-1])
			of = append(of, nf)
			ol = append(ol, nl)
			drawn = append(drawn, float64(len(times)))
			if timedOut {
				break // a stopped renderer is not repeated
			}
		}
		return cell{median(tot), median(p95), median(mx), median(of), median(ol), timedOut,
			int(100 * median(drawn) / float64(len(steps)))}
	}

	type rowKey struct{ scen, pat string }
	var order []rowKey
	stream := map[rowKey]map[int]cell{}
	naive := map[rowKey]map[int]cell{}
	chars := map[string]int{}
	updates := map[rowKey]int{}

	for _, sc := range llxcorpus.Scenarios() {
		if len(only) > 0 && !only[sc.Name] {
			continue
		}
		chars[sc.Name] = len(sc.Text)
		pats := patterns[sc.Name]
		if pats == nil {
			pats = []llxcorpus.Pattern{llxcorpus.Frame}
		}
		for _, p := range pats {
			k := rowKey{sc.Name, string(p)}
			order = append(order, k)
			stream[k], naive[k] = map[int]cell{}, map[int]cell{}
			steps := prefixes(sc.Text, p)
			updates[k] = len(steps)
			for _, w := range widths {
				r := newRenderer(w)
				// History (thread-30) is finished replies above the live one:
				// rendered once, outside the timed updates, for both modes.
				for _, h := range sc.History {
					if _, err := r.Render(h); err != nil {
						t.Fatal(err)
					}
				}
				// Stream: a fresh Cache and Stream per pass, so no pass runs on
				// blocks an earlier one cached. The warm-up warms code only.
				var cur *bubbleteandmd.Stream
				stream[k][w] = measure(steps,
					func() {
						cur = bubbleteandmd.NewStream(r, fmt.Sprint(w), bubbleteandmd.NewCache(4096))
					},
					func(s string) {
						if _, err := cur.Frame(s); err != nil {
							t.Fatal(err)
						}
					})
				naive[k][w] = measure(steps, func() {}, func(s string) {
					if _, err := r.Render(s); err != nil {
						t.Fatal(err)
					}
				})
				t.Logf("%s %s w%d stream %.0f ms naive %.0f ms (stopped=%v)", k.scen, k.pat, w,
					stream[k][w].total, naive[k][w].total, naive[k][w].timedOut)
			}
		}
	}

	// llx column: llx 0.4.1 total (p95) and frame budget, from llx-markdown
	// bench/results/latest.md at ad88e30 (Apple M5, bun, DOM).
	llxTotal := map[string]string{
		"chat-5k token": "22.1 ms (0.03)", "chat-5k frame": "9.25 ms (0.07)",
		"answer-20k token": "86.9 ms (0.03)", "answer-20k frame": "34.4 ms (0.09)", "answer-20k line": "16.2 ms (0.10)",
		"forms-20k frame": "67.3 ms (0.15)", "math-20k frame": "31.5 ms (0.06)",
		"code-50k token": "125 ms (0.01)", "code-50k frame": "36.1 ms (0.02)",
		"table-20k frame": "30.5 ms (0.10)", "long-100k frame": "240 ms (0.09)",
		"crlf-20k frame": "35.2 ms (0.07)", "footnotes-20k frame": "32.4 ms (0.06)", "thread-30 frame": "65.6 ms (0.18)",
	}
	llxBudget := map[string]string{
		"chat-5k token": "0 / 0 / 0.14", "chat-5k frame": "0 / 0 / 0.16",
		"answer-20k token": "0 / 0 / 1.38", "answer-20k frame": "0 / 0 / 1.54", "answer-20k line": "0 / 0 / 0.29",
		"forms-20k frame": "0 / 0 / 1.58", "math-20k frame": "0 / 0 / 1.77",
		"code-50k token": "0 / 0 / 1.74", "code-50k frame": "0 / 0 / 1.23",
		"table-20k frame": "0 / 0 / 1.53", "long-100k frame": "0 / 0 / 1.60",
		"crlf-20k frame": "0 / 0 / 1.35", "footnotes-20k frame": "0 / 0 / 2.52", "thread-30 frame": "0 / 0 / 1.51",
	}
	ms := func(v float64) string {
		switch {
		case v >= 100:
			return fmt.Sprintf("%.0f", v)
		case v >= 10:
			return fmt.Sprintf("%.1f", v)
		}
		return fmt.Sprintf("%.2f", v)
	}
	tot := func(c cell) string {
		if c.timedOut {
			return fmt.Sprintf("stopped at %d%%", c.share)
		}
		return fmt.Sprintf("%s ms (%s)", ms(c.total), ms(c.p95))
	}
	bud := func(c cell) string {
		s := fmt.Sprintf("%.0f / %.0f / %s ms", c.overFrame, c.overLong, ms(c.longest))
		if c.timedOut {
			s += fmt.Sprintf(" (stopped at %d%%)", c.share)
		}
		return s
	}

	var b strings.Builder
	cpu := "unknown"
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(l, "model name") {
				cpu = strings.TrimSpace(l[strings.Index(l, ":")+1:])
				break
			}
		}
	}
	glam := "unknown"
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "charm.land/glamour/v2" {
				glam = d.Version
			}
		}
	}
	fmt.Fprintf(&b, "# bubbleteandmd streaming cost, llx-markdown's method\n\n")
	fmt.Fprintf(&b, "**Not like-for-like.** The `llx` column is llx-markdown 0.4.1 on an Apple M5 with bun, rendering React to a happy-dom DOM (llx `bench/results/latest.md`, commit ad88e30). Our columns are %s with Go, rendering to ANSI strings through glamour. Compare shapes (growth, which rows stay under a frame), not absolute times.\n\n", cpu)
	fmt.Fprintf(&b, "- **date:** %s\n- **cpu:** %s (%d threads)\n- **go:** %s\n- **glamour:** charm.land/glamour/v2 %s, dark style, word wrap 78 and 98 (dun's widths)\n- **runs:** %d, median; one warm-up on the first 200 updates; budget %d ms per cell\n\n",
		time.Now().Format("2006-01-02"), cpu, runtime.NumCPU(), runtime.Version(), glam, runs, budget.Milliseconds())
	fmt.Fprintf(&b, "## Method\n\nSame as llx `bench/src/run.ts`, `measure.tsx`, `report.ts`: every growing prefix is passed one update at a time; each update is timed alone; **total** is the sum of the updates (the first render and the final non-streaming render are not in it); **p95** is `sorted[floor(0.95*n)]`; the frame budget counts updates over 16.7 ms and over 50 ms, and the longest. The llx timer runs until React is idle (render, commit, effects, DOM writes); ours runs to the return of `Stream.Frame` (heal, split, glamour render of the tail, join) and has no DOM or layout.\n\n")
	fmt.Fprintf(&b, "- **stream**: `bubbleteandmd.Stream.Frame(prefix)` on one Stream with a fresh 4096-block Cache per pass. `Final` is not timed (llx excludes its finish render too).\n- **naive**: one `TermRenderer.Render(prefix)` of the whole raw prefix per update (what dun does today; no heal).\n- A pass that spends 30000 ms is stopped and not repeated; the cell says how far it got.\n- thread-30: the 30 history replies are rendered once before timing, for both modes, and are not in the totals. llx keeps them mounted (React-memoized); equivalent in spirit.\n- Chunk patterns come from `internal/corpus` (ported from llx `test/chunks.ts`; token seed 42). Scenario text comes from `internal/corpus` `Scenarios()`, ported from `replies.ts`. Byte-equality with the TS output is NOT verified (no TS run allowed), so text-dependent numbers can differ from llx for that reason alone.\n\n")
	fmt.Fprintf(&b, "## Scenarios\n\nAll ten llx scenarios exist in our corpus; none is missing. llx's `answer-10k`/`answer-40k` scaling pair exists too (`corpus.Scaling()`) but is not run here. Patterns as llx: `token` for chat-5k, answer-20k, code-50k; `line` for answer-20k; `frame` for all.\n\n| scenario | text length (bytes; all ASCII) | updates |\n| --- | ---: | --- |\n")
	seen := map[string]bool{}
	for _, k := range order {
		if seen[k.scen] {
			continue
		}
		seen[k.scen] = true
		var us []string
		for _, p := range []string{"token", "frame", "line"} {
			if n, ok := updates[rowKey{k.scen, p}]; ok {
				us = append(us, fmt.Sprintf("%s %d", p, n))
			}
		}
		fmt.Fprintf(&b, "| %s | %d | %s |\n", k.scen, chars[k.scen], strings.Join(us, ", "))
	}
	fmt.Fprintf(&b, "\nllx's README gives only nominal sizes (`-5k`, `-20k`, ...) and its `latest.json` only a hash per text, so llx's exact lengths are not known here without running its generator. A generated reply grows by whole blocks until its length reaches the nominal size, so lengths are slightly over nominal.\n\n")

	fmt.Fprintf(&b, "## Whole stream\n\nTotal ms to stream the reply, p95 of one update in brackets. Lower is better.\n\n| | `llx` (M5, bun, DOM) | stream w78 | stream w98 | naive w78 | naive w98 |\n| --- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, k := range order {
		fmt.Fprintf(&b, "| %s · %s | %s | %s | %s | %s | %s |\n", k.scen, k.pat, llxTotal[k.scen+" "+k.pat],
			tot(stream[k][78]), tot(stream[k][98]), tot(naive[k][78]), tot(naive[k][98]))
	}
	fmt.Fprintf(&b, "\n## Frame budget\n\nUpdates longer than 16.7 ms / longer than 50 ms / the longest update.\n\n| | `llx` (M5, bun, DOM) | stream w78 | stream w98 | naive w78 | naive w98 |\n| --- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, k := range order {
		l := llxBudget[k.scen+" "+k.pat]
		if l != "" {
			l += " ms"
		}
		fmt.Fprintf(&b, "| %s · %s | %s | %s | %s | %s | %s |\n", k.scen, k.pat, l,
			bud(stream[k][78]), bud(stream[k][98]), bud(naive[k][78]), bud(naive[k][98]))
	}
	out := b.String()
	t.Log("\n" + out)
	if f := os.Getenv("BENCH_LLX_OUT"); f != "" {
		if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
