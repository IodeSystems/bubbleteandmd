<!-- Run on Apple M1 Max (go1.27.3 darwin/arm64), 1296 s, commit f88f0d4 (git archive). The llx column is the M5 data embedded in the test; llx on M1 Max is llx-m1max-2026-11-10.md. -->
# bubbleteandmd streaming cost, llx-markdown's method

**Not like-for-like.** The `llx` column is llx-markdown 0.4.1 on an Apple M5 with bun, rendering React to a happy-dom DOM (llx `bench/results/latest.md`, commit ad88e30). Our columns are Apple M1 Max with Go, rendering to ANSI strings through glamour. Compare shapes (growth, which rows stay under a frame), not absolute times.

- **date:** 2026-11-11
- **cpu:** Apple M1 Max (10 threads)
- **go:** go1.27.3
- **glamour:** charm.land/glamour/v2 v2.0.1, dark style, word wrap 78 and 98 (dun's widths)
- **runs:** 3, median; one warm-up on the first 200 updates; budget 30000 ms per cell

## Method

Same as llx `bench/src/run.ts`, `measure.tsx`, `report.ts`: every growing prefix is passed one update at a time; each update is timed alone; **total** is the sum of the updates (the first render and the final non-streaming render are not in it); **p95** is `sorted[floor(0.95*n)]`; the frame budget counts updates over 16.7 ms and over 50 ms, and the longest. The llx timer runs until React is idle (render, commit, effects, DOM writes); ours runs to the return of `Stream.Frame` (heal, split, glamour render of the tail, join) and has no DOM or layout.

- **stream**: `bubbleteandmd.Stream.Frame(prefix)` on one Stream with a fresh 4096-block Cache per pass. `Final` is not timed (llx excludes its finish render too).
- **directive**: the same Stream over `bubbleteandmd.NewRenderer` (dark style): forms drawn, and a growing tail table served from cached rows (it tells Stream its glamour options).
- **naive**: one `TermRenderer.Render(prefix)` of the whole raw prefix per update (what dun does today; no heal).
- A pass that spends 30000 ms is stopped and not repeated; the cell says how far it got.
- thread-30: the 30 history replies are rendered once before timing, for both modes, and are not in the totals. llx keeps them mounted (React-memoized); equivalent in spirit.
- Chunk patterns come from `internal/corpus` (ported from llx `test/chunks.ts`; token seed 42). Scenario text comes from `internal/corpus` `Scenarios()`, ported from `replies.ts`. Byte-equality with the TS output is NOT verified (no TS run allowed), so text-dependent numbers can differ from llx for that reason alone.

## Scenarios

All ten llx scenarios exist in our corpus; none is missing. llx's `answer-10k`/`answer-40k` scaling pair exists too (`corpus.Scaling()`) but is not run here. Patterns as llx: `token` for chat-5k, answer-20k, code-50k; `line` for answer-20k; `frame` for all.

| scenario | text length (bytes; all ASCII) | updates |
| --- | ---: | --- |
| chat-5k | 5158 | token 1148, frame 258 |
| answer-20k | 20008 | token 4412, frame 1001, line 343 |
| forms-20k | 20588 | frame 1030 |
| math-20k | 20136 | frame 1007 |
| code-50k | 48837 | token 10763, frame 2442 |
| table-20k | 17839 | frame 892 |
| long-100k | 100643 | frame 5033 |
| crlf-20k | 20409 | frame 1021 |
| footnotes-20k | 21038 | frame 1052 |
| thread-30 | 10283 | frame 515 |

llx's README gives only nominal sizes (`-5k`, `-20k`, ...) and its `latest.json` only a hash per text, so llx's exact lengths are not known here without running its generator. A generated reply grows by whole blocks until its length reaches the nominal size, so lengths are slightly over nominal.

## Whole stream

Total ms to stream the reply, p95 of one update in brackets. Lower is better.

| | `llx` (M5, bun, DOM) | stream w78 | stream w98 | directive w78 | directive w98 | naive w78 | naive w98 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| chat-5k · token | 22.1 ms (0.03) | 251 ms (0.49) | 274 ms (0.54) | 261 ms (0.52) | 287 ms (0.58) | 2182 ms (3.63) | 2580 ms (4.18) |
| chat-5k · frame | 9.25 ms (0.07) | 63.0 ms (0.54) | 68.2 ms (0.60) | 65.0 ms (0.59) | 71.2 ms (0.61) | 502 ms (3.62) | 595 ms (4.29) |
| answer-20k · token | 86.9 ms (0.03) | 2316 ms (1.91) | 2460 ms (1.99) | 2146 ms (1.94) | 2278 ms (1.98) | stopped at 77% | stopped at 75% |
| answer-20k · frame | 34.4 ms (0.09) | 553 ms (1.94) | 603 ms (2.00) | 541 ms (2.04) | 576 ms (2.07) | 11799 ms (23.1) | 12547 ms (25.0) |
| answer-20k · line | 16.2 ms (0.10) | 261 ms (2.33) | 277 ms (2.42) | 281 ms (2.41) | 300 ms (2.56) | 4380 ms (23.4) | 4759 ms (25.8) |
| forms-20k · frame | 67.3 ms (0.15) | 5618 ms (16.5) | 5910 ms (16.9) | 3298 ms (8.45) | 3499 ms (8.97) | stopped at 92% | stopped at 88% |
| math-20k · frame | 31.5 ms (0.06) | 643 ms (2.23) | 678 ms (2.29) | 703 ms (2.83) | 749 ms (2.84) | 15086 ms (25.8) | 15923 ms (27.5) |
| code-50k · token | 125 ms (0.01) | 20577 ms (3.27) | 21644 ms (3.43) | 20864 ms (3.31) | 21780 ms (3.45) | stopped at 28% | stopped at 27% |
| code-50k · frame | 36.1 ms (0.02) | 4870 ms (3.38) | 5108 ms (3.54) | 4906 ms (3.40) | 5137 ms (3.56) | stopped at 59% | stopped at 58% |
| table-20k · frame | 30.5 ms (0.10) | 7208 ms (15.3) | 6729 ms (14.2) | 901 ms (5.22) | 1055 ms (4.80) | 7303 ms (15.3) | 6820 ms (14.2) |
| long-100k · frame | 240 ms (0.09) | 3251 ms (2.10) | 3424 ms (2.15) | 3223 ms (2.11) | 3425 ms (2.16) | stopped at 28% | stopped at 28% |
| crlf-20k · frame | 35.2 ms (0.07) | 915 ms (3.31) | 935 ms (3.35) | 898 ms (3.25) | 933 ms (3.39) | 14776 ms (28.7) | 14963 ms (29.1) |
| footnotes-20k · frame | 32.4 ms (0.06) | 624 ms (1.76) | 657 ms (1.73) | 615 ms (1.81) | 645 ms (1.71) | 11960 ms (21.4) | 12420 ms (22.5) |
| thread-30 · frame | 65.6 ms (0.18) | 361 ms (2.13) | 386 ms (2.23) | 357 ms (2.21) | 383 ms (2.27) | 3255 ms (12.9) | 3579 ms (14.1) |

## Frame budget

Updates longer than 16.7 ms / longer than 50 ms / the longest update.

| | `llx` (M5, bun, DOM) | stream w78 | stream w98 | directive w78 | directive w98 | naive w78 | naive w98 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| chat-5k · token | 0 / 0 / 0.14 ms | 0 / 0 / 0.92 ms | 0 / 0 / 0.97 ms | 0 / 0 / 0.97 ms | 0 / 0 / 1.15 ms | 0 / 0 / 4.24 ms | 0 / 0 / 4.83 ms |
| chat-5k · frame | 0 / 0 / 0.16 ms | 0 / 0 / 0.91 ms | 0 / 0 / 0.86 ms | 0 / 0 / 0.93 ms | 0 / 0 / 0.92 ms | 0 / 0 / 4.20 ms | 0 / 0 / 4.78 ms |
| answer-20k · token | 0 / 0 / 1.38 ms | 0 / 0 / 4.60 ms | 0 / 0 / 4.64 ms | 0 / 0 / 4.66 ms | 0 / 0 / 4.66 ms | 420 / 0 / 19.4 ms (stopped at 77%) | 592 / 0 / 32.1 ms (stopped at 75%) |
| answer-20k · frame | 0 / 0 / 1.54 ms | 0 / 0 / 2.84 ms | 0 / 0 / 3.31 ms | 0 / 0 / 3.11 ms | 0 / 0 / 3.10 ms | 331 / 0 / 24.8 ms | 379 / 0 / 26.9 ms |
| answer-20k · line | 0 / 0 / 0.29 ms | 0 / 0 / 3.17 ms | 0 / 0 / 3.30 ms | 0 / 0 / 3.41 ms | 0 / 0 / 3.40 ms | 136 / 0 / 24.9 ms | 150 / 0 / 32.8 ms |
| forms-20k · frame | 0 / 0 / 1.58 ms | 47 / 0 / 24.3 ms | 59 / 0 / 25.3 ms | 0 / 0 / 11.5 ms | 0 / 0 / 11.9 ms | 653 / 246 / 65.7 ms (stopped at 92%) | 661 / 253 / 69.4 ms (stopped at 88%) |
| math-20k · frame | 0 / 0 / 1.77 ms | 0 / 0 / 5.22 ms | 0 / 0 / 5.14 ms | 0 / 0 / 5.18 ms | 0 / 0 / 5.46 ms | 462 / 0 / 27.1 ms | 483 / 0 / 28.5 ms |
| code-50k · token | 0 / 0 / 1.74 ms | 1 / 1 / 71.2 ms | 1 / 1 / 73.8 ms | 1 / 1 / 71.1 ms | 1 / 1 / 74.1 ms | 446 / 0 / 19.7 ms (stopped at 28%) | 477 / 0 / 20.6 ms (stopped at 27%) |
| code-50k · frame | 0 / 0 / 1.23 ms | 1 / 1 / 72.4 ms | 1 / 1 / 75.9 ms | 1 / 1 / 73.4 ms | 1 / 1 / 76.0 ms | 866 / 0 / 41.5 ms (stopped at 59%) | 858 / 0 / 44.3 ms (stopped at 58%) |
| table-20k · frame | 0 / 0 / 1.53 ms | 0 / 0 / 16.4 ms | 0 / 0 / 15.3 ms | 4 / 0 / 18.3 ms | 0 / 0 / 15.3 ms | 0 / 0 / 16.6 ms | 0 / 0 / 15.4 ms |
| long-100k · frame | 0 / 0 / 1.60 ms | 0 / 0 / 5.58 ms | 0 / 0 / 5.71 ms | 0 / 0 / 5.62 ms | 0 / 0 / 5.86 ms | 888 / 0 / 39.2 ms (stopped at 28%) | 871 / 0 / 40.1 ms (stopped at 28%) |
| crlf-20k · frame | 0 / 0 / 1.35 ms | 0 / 0 / 5.70 ms | 0 / 0 / 5.84 ms | 0 / 0 / 5.59 ms | 0 / 0 / 6.08 ms | 480 / 0 / 30.6 ms | 485 / 0 / 30.8 ms |
| footnotes-20k · frame | 0 / 0 / 2.52 ms | 1 / 0 / 22.9 ms | 1 / 0 / 23.6 ms | 1 / 0 / 24.8 ms | 1 / 0 / 26.4 ms | 266 / 0 / 23.0 ms | 300 / 0 / 24.2 ms |
| thread-30 · frame | 0 / 0 / 1.51 ms | 0 / 0 / 3.00 ms | 0 / 0 / 3.18 ms | 0 / 0 / 3.17 ms | 0 / 0 / 3.17 ms | 0 / 0 / 14.2 ms | 0 / 0 / 15.3 ms |
