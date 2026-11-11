<!-- Run on Apple M1 Max (go1.27.3 darwin/arm64), 1065 s. Extracted from go test log: BENCH_LLX_OUT path is relative to ./bench and did not exist. The llx column below is the M5 data embedded in the test, not M1 Max. -->
# bubbleteandmd streaming cost, llx-markdown's method

**Not like-for-like.** The `llx` column is llx-markdown 0.4.1 on an Apple M5 with bun, rendering React to a happy-dom DOM (llx `bench/results/latest.md`, commit ad88e30). Our columns are unknown with Go, rendering to ANSI strings through glamour. Compare shapes (growth, which rows stay under a frame), not absolute times.

- **date:** 2026-11-10
- **cpu:** unknown (10 threads)
- **go:** go1.27.3
- **glamour:** charm.land/glamour/v2 v2.0.1, dark style, word wrap 78 and 98 (dun's widths)
- **runs:** 3, median; one warm-up on the first 200 updates; budget 30000 ms per cell

## Method

Same as llx `bench/src/run.ts`, `measure.tsx`, `report.ts`: every growing prefix is passed one update at a time; each update is timed alone; **total** is the sum of the updates (the first render and the final non-streaming render are not in it); **p95** is `sorted[floor(0.95*n)]`; the frame budget counts updates over 16.7 ms and over 50 ms, and the longest. The llx timer runs until React is idle (render, commit, effects, DOM writes); ours runs to the return of `Stream.Frame` (heal, split, glamour render of the tail, join) and has no DOM or layout.

- **stream**: `bubbleteandmd.Stream.Frame(prefix)` on one Stream with a fresh 4096-block Cache per pass. `Final` is not timed (llx excludes its finish render too).
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

| | `llx` (M5, bun, DOM) | stream w78 | stream w98 | naive w78 | naive w98 |
| --- | ---: | ---: | ---: | ---: | ---: |
| chat-5k · token | 22.1 ms (0.03) | 453 ms (0.76) | 508 ms (0.77) | 2141 ms (3.56) | 2522 ms (4.11) |
| chat-5k · frame | 9.25 ms (0.07) | 111 ms (0.82) | 123 ms (0.86) | 493 ms (3.63) | 583 ms (4.18) |
| answer-20k · token | 86.9 ms (0.03) | 3936 ms (2.57) | 4172 ms (2.68) | stopped at 78% | stopped at 76% |
| answer-20k · frame | 34.4 ms (0.09) | 913 ms (2.61) | 986 ms (2.77) | 11519 ms (22.7) | 12451 ms (24.9) |
| answer-20k · line | 16.2 ms (0.10) | 398 ms (3.03) | 432 ms (3.21) | 4451 ms (23.8) | 4798 ms (26.0) |
| forms-20k · frame | 67.3 ms (0.15) | 6614 ms (17.9) | 7127 ms (18.9) | stopped at 91% | stopped at 88% |
| math-20k · frame | 31.5 ms (0.06) | 1142 ms (4.03) | 1208 ms (4.18) | 15310 ms (26.4) | 16233 ms (28.1) |
| code-50k · token | 125 ms (0.01) | stopped at 28% | stopped at 27% | stopped at 28% | stopped at 27% |
| code-50k · frame | 36.1 ms (0.02) | stopped at 59% | stopped at 58% | stopped at 59% | stopped at 58% |
| table-20k · frame | 30.5 ms (0.10) | 7236 ms (15.3) | 6837 ms (14.3) | 7424 ms (15.6) | 6920 ms (14.5) |
| long-100k · frame | 240 ms (0.09) | 5650 ms (3.02) | 5962 ms (3.13) | stopped at 28% | stopped at 28% |
| crlf-20k · frame | 35.2 ms (0.07) | 1380 ms (4.31) | 1424 ms (4.43) | 14724 ms (28.4) | 14993 ms (29.1) |
| footnotes-20k · frame | 32.4 ms (0.06) | 1090 ms (2.83) | 1145 ms (2.69) | 11988 ms (21.5) | 12616 ms (22.7) |
| thread-30 · frame | 65.6 ms (0.18) | 586 ms (2.77) | 621 ms (2.85) | 3234 ms (12.9) | 3554 ms (14.0) |

## Frame budget

Updates longer than 16.7 ms / longer than 50 ms / the longest update.

| | `llx` (M5, bun, DOM) | stream w78 | stream w98 | naive w78 | naive w98 |
| --- | ---: | ---: | ---: | ---: | ---: |
| chat-5k · token | 0 / 0 / 0.14 ms | 0 / 0 / 1.09 ms | 0 / 0 / 1.14 ms | 0 / 0 / 4.06 ms | 0 / 0 / 4.77 ms |
| chat-5k · frame | 0 / 0 / 0.16 ms | 0 / 0 / 1.22 ms | 0 / 0 / 1.12 ms | 0 / 0 / 4.03 ms | 0 / 0 / 4.70 ms |
| answer-20k · token | 0 / 0 / 1.38 ms | 0 / 0 / 6.32 ms | 0 / 0 / 6.46 ms | 329 / 0 / 20.3 ms (stopped at 78%) | 553 / 0 / 27.8 ms (stopped at 76%) |
| answer-20k · frame | 0 / 0 / 1.54 ms | 0 / 0 / 3.47 ms | 0 / 0 / 3.90 ms | 304 / 0 / 26.7 ms | 378 / 0 / 26.0 ms |
| answer-20k · line | 0 / 0 / 0.29 ms | 0 / 0 / 3.93 ms | 0 / 0 / 4.22 ms | 137 / 0 / 25.0 ms | 149 / 0 / 27.4 ms |
| forms-20k · frame | 0 / 0 / 1.58 ms | 63 / 0 / 25.5 ms | 65 / 0 / 25.5 ms | 656 / 244 / 67.3 ms (stopped at 91%) | 657 / 264 / 69.6 ms (stopped at 88%) |
| math-20k · frame | 0 / 0 / 1.77 ms | 0 / 0 / 6.70 ms | 0 / 0 / 6.77 ms | 459 / 0 / 30.6 ms | 486 / 0 / 46.1 ms |
| code-50k · token | 0 / 0 / 1.74 ms | 483 / 0 / 30.3 ms (stopped at 28%) | 485 / 0 / 34.9 ms (stopped at 27%) | 453 / 0 / 30.1 ms (stopped at 28%) | 474 / 0 / 30.1 ms (stopped at 27%) |
| code-50k · frame | 0 / 0 / 1.23 ms | 859 / 0 / 48.1 ms (stopped at 59%) | 859 / 1 / 58.0 ms (stopped at 58%) | 851 / 1 / 60.9 ms (stopped at 59%) | 861 / 0 / 42.5 ms (stopped at 58%) |
| table-20k · frame | 0 / 0 / 1.53 ms | 0 / 0 / 16.4 ms | 0 / 0 / 15.3 ms | 2 / 0 / 16.8 ms | 0 / 0 / 15.7 ms |
| long-100k · frame | 0 / 0 / 1.60 ms | 0 / 0 / 5.83 ms | 0 / 0 / 6.00 ms | 893 / 0 / 39.2 ms (stopped at 28%) | 878 / 0 / 41.3 ms (stopped at 28%) |
| crlf-20k · frame | 0 / 0 / 1.35 ms | 0 / 0 / 6.49 ms | 0 / 0 / 6.64 ms | 477 / 0 / 30.4 ms | 487 / 0 / 30.6 ms |
| footnotes-20k · frame | 0 / 0 / 2.52 ms | 1 / 0 / 22.3 ms | 1 / 0 / 24.2 ms | 267 / 0 / 22.9 ms | 303 / 0 / 24.6 ms |
| thread-30 · frame | 0 / 0 / 1.51 ms | 0 / 0 / 3.46 ms | 0 / 0 / 3.67 ms | 0 / 0 / 14.0 ms | 0 / 0 / 15.2 ms |

