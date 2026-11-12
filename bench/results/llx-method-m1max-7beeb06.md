<!-- Run on Apple M1 Max (go1.27.3 darwin/arm64), 1218 s, commit 7beeb06 (git clone of bundle). The llx column is the M5 data embedded in the test; llx 0.5.0 (flowdown) on M1 Max is llx-m1max-1032274.md. -->
# bubbleteandmd streaming cost, llx-markdown's method

**Not like-for-like.** The `llx` column is llx-markdown 0.4.1 on an Apple M5 with bun, rendering React to a happy-dom DOM (llx `bench/results/latest.md`, commit ad88e30). Our columns are Apple M1 Max with Go, rendering to ANSI strings through glamour. Compare shapes (growth, which rows stay under a frame), not absolute times.

- **date:** 2026-11-12
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
| chat-5k · token | 22.1 ms (0.03) | 211 ms (0.40) | 230 ms (0.44) | 222 ms (0.43) | 241 ms (0.46) | 1748 ms (2.88) | 2031 ms (3.29) |
| chat-5k · frame | 9.25 ms (0.07) | 52.6 ms (0.44) | 57.3 ms (0.48) | 55.0 ms (0.47) | 59.4 ms (0.51) | 398 ms (2.92) | 468 ms (3.36) |
| answer-20k · token | 86.9 ms (0.03) | 2166 ms (1.84) | 2282 ms (1.91) | 1924 ms (1.75) | 2035 ms (1.83) | stopped at 81% | stopped at 79% |
| answer-20k · frame | 34.4 ms (0.09) | 518 ms (1.87) | 551 ms (1.92) | 491 ms (1.88) | 523 ms (1.96) | 10604 ms (20.9) | 11361 ms (22.7) |
| answer-20k · line | 16.2 ms (0.10) | 253 ms (2.38) | 267 ms (2.46) | 267 ms (2.46) | 281 ms (2.53) | 4174 ms (22.4) | 4451 ms (24.2) |
| forms-20k · frame | 67.3 ms (0.15) | 5751 ms (17.6) | 6008 ms (18.0) | 2720 ms (6.99) | 2893 ms (7.42) | stopped at 92% | stopped at 89% |
| math-20k · frame | 31.5 ms (0.06) | 597 ms (2.06) | 631 ms (2.15) | 576 ms (2.07) | 611 ms (2.14) | 13791 ms (23.2) | 14476 ms (24.5) |
| code-50k · token | 125 ms (0.01) | 19693 ms (3.13) | 20812 ms (3.31) | 18835 ms (3.07) | 19945 ms (3.23) | stopped at 29% | stopped at 28% |
| code-50k · frame | 36.1 ms (0.02) | 4634 ms (3.21) | 4894 ms (3.39) | 4438 ms (3.14) | 4703 ms (3.31) | stopped at 61% | stopped at 59% |
| table-20k · frame | 30.5 ms (0.10) | 6602 ms (14.0) | 6182 ms (13.1) | 1404 ms (4.63) | 1388 ms (4.58) | 6555 ms (13.8) | 6162 ms (12.9) |
| long-100k · frame | 240 ms (0.09) | 3107 ms (2.06) | 3288 ms (2.10) | 3004 ms (1.99) | 3197 ms (2.07) | stopped at 29% | stopped at 29% |
| crlf-20k · frame | 35.2 ms (0.07) | 884 ms (3.32) | 910 ms (3.47) | 837 ms (3.25) | 876 ms (3.42) | 13518 ms (26.4) | 13869 ms (27.0) |
| footnotes-20k · frame | 32.4 ms (0.06) | 562 ms (1.55) | 580 ms (1.52) | 546 ms (1.58) | 567 ms (1.53) | 10613 ms (18.9) | 10997 ms (19.7) |
| thread-30 · frame | 65.6 ms (0.18) | 353 ms (2.17) | 368 ms (2.25) | 329 ms (2.09) | 348 ms (2.19) | 2942 ms (11.8) | 3199 ms (12.8) |

## Frame budget

Updates longer than 16.7 ms / longer than 50 ms / the longest update.

| | `llx` (M5, bun, DOM) | stream w78 | stream w98 | directive w78 | directive w98 | naive w78 | naive w98 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| chat-5k · token | 0 / 0 / 0.14 ms | 0 / 0 / 0.72 ms | 0 / 0 / 0.77 ms | 0 / 0 / 0.72 ms | 0 / 0 / 0.86 ms | 0 / 0 / 3.37 ms | 0 / 0 / 3.80 ms |
| chat-5k · frame | 0 / 0 / 0.16 ms | 0 / 0 / 0.73 ms | 0 / 0 / 0.76 ms | 0 / 0 / 0.86 ms | 0 / 0 / 0.70 ms | 0 / 0 / 3.30 ms | 0 / 0 / 3.81 ms |
| answer-20k · token | 0 / 0 / 1.38 ms | 0 / 0 / 5.49 ms | 0 / 0 / 5.47 ms | 0 / 0 / 3.31 ms | 0 / 0 / 3.45 ms | 237 / 0 / 22.0 ms (stopped at 81%) | 335 / 0 / 23.7 ms (stopped at 79%) |
| answer-20k · frame | 0 / 0 / 1.54 ms | 0 / 0 / 3.19 ms | 0 / 0 / 3.35 ms | 0 / 0 / 3.08 ms | 0 / 0 / 3.00 ms | 243 / 0 / 22.2 ms | 288 / 0 / 24.2 ms |
| answer-20k · line | 0 / 0 / 0.29 ms | 0 / 0 / 3.23 ms | 0 / 0 / 3.33 ms | 0 / 0 / 3.27 ms | 0 / 0 / 3.16 ms | 122 / 0 / 23.8 ms | 142 / 0 / 25.5 ms |
| forms-20k · frame | 0 / 0 / 1.58 ms | 60 / 0 / 26.4 ms | 60 / 0 / 26.8 ms | 0 / 0 / 10.00 ms | 0 / 0 / 10.1 ms | 631 / 250 / 66.3 ms (stopped at 92%) | 651 / 248 / 69.2 ms (stopped at 89%) |
| math-20k · frame | 0 / 0 / 1.77 ms | 0 / 0 / 5.45 ms | 0 / 0 / 5.58 ms | 0 / 0 / 5.26 ms | 0 / 0 / 5.40 ms | 420 / 0 / 24.8 ms | 445 / 0 / 26.1 ms |
| code-50k · token | 0 / 0 / 1.74 ms | 1 / 1 / 67.1 ms | 1 / 1 / 70.8 ms | 1 / 1 / 68.0 ms | 1 / 1 / 71.9 ms | 371 / 0 / 20.6 ms (stopped at 29%) | 423 / 0 / 20.8 ms (stopped at 28%) |
| code-50k · frame | 0 / 0 / 1.23 ms | 1 / 1 / 70.1 ms | 1 / 1 / 75.3 ms | 1 / 1 / 71.0 ms | 1 / 1 / 73.0 ms | 868 / 0 / 41.8 ms (stopped at 61%) | 866 / 0 / 41.7 ms (stopped at 59%) |
| table-20k · frame | 0 / 0 / 1.53 ms | 0 / 0 / 15.2 ms | 0 / 0 / 14.2 ms | 2 / 0 / 20.9 ms | 2 / 0 / 21.1 ms | 0 / 0 / 14.8 ms | 0 / 0 / 13.9 ms |
| long-100k · frame | 0 / 0 / 1.60 ms | 0 / 0 / 5.78 ms | 0 / 0 / 5.89 ms | 0 / 0 / 5.42 ms | 0 / 0 / 5.79 ms | 932 / 0 / 37.3 ms (stopped at 29%) | 923 / 0 / 38.3 ms (stopped at 29%) |
| crlf-20k · frame | 0 / 0 / 1.35 ms | 0 / 0 / 6.37 ms | 0 / 0 / 6.09 ms | 0 / 0 / 6.09 ms | 0 / 0 / 6.18 ms | 433 / 0 / 28.2 ms | 453 / 0 / 29.1 ms |
| footnotes-20k · frame | 0 / 0 / 2.52 ms | 1 / 0 / 20.9 ms | 1 / 0 / 21.4 ms | 1 / 0 / 22.0 ms | 1 / 0 / 22.9 ms | 186 / 0 / 20.3 ms | 221 / 0 / 21.3 ms |
| thread-30 · frame | 0 / 0 / 1.51 ms | 0 / 0 / 3.36 ms | 0 / 0 / 3.56 ms | 0 / 0 / 3.31 ms | 0 / 0 / 3.24 ms | 0 / 0 / 12.9 ms | 0 / 0 / 13.8 ms |
