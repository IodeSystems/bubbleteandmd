# bubbleteandmd streaming cost, llx-markdown's method

**Not like-for-like.** The `llx` column is llx-markdown 0.4.1 on an Apple M5 with bun, rendering React to a happy-dom DOM (llx `bench/results/latest.md`, commit ad88e30). Our columns are AMD Ryzen 9 3900X 12-Core Processor with Go, rendering to ANSI strings through glamour. Compare shapes (growth, which rows stay under a frame), not absolute times.

- **date:** 2026-11-10
- **cpu:** AMD Ryzen 9 3900X 12-Core Processor (24 threads)
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
| chat-5k · token | 22.1 ms (0.03) | 677 ms (1.14) | 752 ms (1.19) | 3255 ms (5.43) | 4610 ms (7.78) |
| chat-5k · frame | 9.25 ms (0.07) | 166 ms (1.31) | 176 ms (1.25) | 708 ms (5.29) | 952 ms (6.64) |
| answer-20k · token | 86.9 ms (0.03) | 5788 ms (3.75) | 14040 ms (11.6) | stopped at 59% | stopped at 46% |
| answer-20k · frame | 34.4 ms (0.09) | 1973 ms (5.70) | 2489 ms (7.01) | 25425 ms (57.5) | 29488 ms (60.9) |
| answer-20k · line | 16.2 ms (0.10) | 1406 ms (12.6) | 1299 ms (10.4) | 12522 ms (74.2) | 9378 ms (64.0) |
| forms-20k · frame | 67.3 ms (0.15) | 9494 ms (28.2) | 16914 ms (47.9) | stopped at 69% | stopped at 60% |
| math-20k · frame | 31.5 ms (0.06) | 2935 ms (10.6) | 3112 ms (10.6) | stopped at 87% | stopped at 87% |
| code-50k · token | 125 ms (0.01) | stopped at 19% | stopped at 16% | stopped at 19% | stopped at 17% |
| code-50k · frame | 36.1 ms (0.02) | stopped at 36% | stopped at 44% | stopped at 38% | stopped at 44% |
| table-20k · frame | 30.5 ms (0.10) | 13551 ms (30.8) | 11089 ms (23.8) | 17238 ms (38.2) | 10067 ms (20.7) |
| long-100k · frame | 240 ms (0.09) | 9573 ms (5.27) | 12377 ms (6.75) | stopped at 20% | stopped at 18% |
| crlf-20k · frame | 35.2 ms (0.07) | 2981 ms (9.07) | 3842 ms (11.6) | stopped at 93% | stopped at 91% |
| footnotes-20k · frame | 32.4 ms (0.06) | 2837 ms (7.13) | 3088 ms (7.99) | stopped at 94% | stopped at 88% |
| thread-30 · frame | 65.6 ms (0.18) | 1992 ms (12.0) | 2469 ms (14.8) | 11119 ms (51.1) | 13643 ms (66.3) |

## Frame budget

Updates longer than 16.7 ms / longer than 50 ms / the longest update.

| | `llx` (M5, bun, DOM) | stream w78 | stream w98 | naive w78 | naive w98 |
| --- | ---: | ---: | ---: | ---: | ---: |
| chat-5k · token | 0 / 0 / 0.14 ms | 0 / 0 / 2.93 ms | 0 / 0 / 3.48 ms | 0 / 0 / 10.7 ms | 0 / 0 / 13.3 ms |
| chat-5k · frame | 0 / 0 / 0.16 ms | 0 / 0 / 1.84 ms | 0 / 0 / 1.69 ms | 0 / 0 / 7.23 ms | 0 / 0 / 8.42 ms |
| answer-20k · token | 0 / 0 / 1.38 ms | 0 / 0 / 8.59 ms | 72 / 1 / 69.5 ms | 637 / 34 / 117 ms (stopped at 59%) | 801 / 35 / 80.1 ms (stopped at 46%) |
| answer-20k · frame | 0 / 0 / 1.54 ms | 0 / 0 / 13.0 ms | 2 / 0 / 20.7 ms | 601 / 103 / 94.1 ms | 668 / 170 / 123 ms |
| answer-20k · line | 0 / 0 / 0.29 ms | 3 / 0 / 22.3 ms | 4 / 0 / 24.8 ms | 253 / 91 / 138 ms | 230 / 37 / 89.6 ms |
| forms-20k · frame | 0 / 0 / 1.58 ms | 222 / 0 / 39.4 ms | 421 / 40 / 78.2 ms | 537 / 260 / 171 ms (stopped at 69%) | 480 / 223 / 192 ms (stopped at 60%) |
| math-20k · frame | 0 / 0 / 1.77 ms | 6 / 0 / 24.7 ms | 15 / 0 / 23.2 ms | 688 / 193 / 132 ms (stopped at 87%) | 682 / 194 / 109 ms (stopped at 87%) |
| code-50k · token | 0 / 0 / 1.74 ms | 875 / 0 / 49.4 ms (stopped at 19%) | 949 / 14 / 86.1 ms (stopped at 16%) | 846 / 5 / 65.5 ms (stopped at 19%) | 819 / 16 / 79.5 ms (stopped at 17%) |
| code-50k · frame | 0 / 0 / 1.23 ms | 665 / 195 / 105 ms (stopped at 36%) | 781 / 36 / 75.3 ms (stopped at 44%) | 691 / 181 / 90.8 ms (stopped at 38%) | 727 / 90 / 73.8 ms (stopped at 44%) |
| table-20k · frame | 0 / 0 / 1.53 ms | 346 / 3 / 56.2 ms | 263 / 0 / 31.0 ms | 496 / 5 / 61.6 ms | 241 / 0 / 28.6 ms |
| long-100k · frame | 0 / 0 / 1.60 ms | 0 / 0 / 11.9 ms | 1 / 0 / 17.5 ms | 717 / 191 / 69.7 ms (stopped at 20%) | 698 / 196 / 83.8 ms (stopped at 18%) |
| crlf-20k · frame | 0 / 0 / 1.35 ms | 2 / 0 / 17.3 ms | 14 / 0 / 30.9 ms | 639 / 145 / 97.1 ms (stopped at 93%) | 644 / 207 / 93.4 ms (stopped at 91%) |
| footnotes-20k · frame | 0 / 0 / 2.52 ms | 3 / 1 / 53.3 ms | 3 / 1 / 109 ms | 712 / 123 / 153 ms (stopped at 94%) | 638 / 189 / 141 ms (stopped at 88%) |
| thread-30 · frame | 0 / 0 / 1.51 ms | 7 / 0 / 28.2 ms | 14 / 0 / 30.1 ms | 290 / 28 / 95.2 ms | 308 / 68 / 104 ms |


## Notes

- One pass of 3 runs per cell; the box was idle but not isolated. A 2x gap between w78 and w98 on one row (answer-20k token: 5788 vs 14040 ms) is likely noise or GC, not width. No repeat passes (llx's `--passes=3`) were run.
- code-50k and table-20k are one growing block, so the tail is the whole block and Stream cannot cache it: Stream costs about the same as naive there.
- Stream still has updates over 50 ms on forms-20k, footnotes-20k and answer-20k token w98. Cause not investigated.
- Run: `BENCH_LLX=1 BENCH_LLX_OUT=<abs path> go test ./bench -run TestLLXMethod -v -timeout 3h` (about 25 min). This file was cut from the test log: the output directory did not exist on the first run, so the in-test write failed after the measurements finished.
