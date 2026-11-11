<!-- Run on Apple M1 Max (go1.27.3 darwin/arm64), 1206 s, commit ccd1caf (git archive): deps bump + cached chroma formatter. The llx column is the M5 data embedded in the test; llx on M1 Max is llx-m1max-2026-11-10.md. -->
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
| chat-5k · token | 22.1 ms (0.03) | 206 ms (0.39) | 223 ms (0.43) | 216 ms (0.42) | 232 ms (0.45) | 1713 ms (2.82) | 2021 ms (3.29) |
| chat-5k · frame | 9.25 ms (0.07) | 51.8 ms (0.44) | 56.5 ms (0.47) | 54.4 ms (0.46) | 58.9 ms (0.51) | 395 ms (2.92) | 468 ms (3.43) |
| answer-20k · token | 86.9 ms (0.03) | 2150 ms (1.86) | 2264 ms (1.91) | 1847 ms (1.77) | 1943 ms (1.81) | stopped at 81% | stopped at 79% |
| answer-20k · frame | 34.4 ms (0.09) | 514 ms (1.89) | 539 ms (1.91) | 469 ms (1.84) | 495 ms (1.90) | 10675 ms (20.9) | 11357 ms (22.7) |
| answer-20k · line | 16.2 ms (0.10) | 250 ms (2.36) | 266 ms (2.45) | 253 ms (2.34) | 272 ms (2.42) | 4146 ms (22.4) | 4437 ms (23.8) |
| forms-20k · frame | 67.3 ms (0.15) | 5749 ms (17.5) | 5996 ms (17.9) | 2712 ms (7.07) | 2894 ms (7.52) | stopped at 92% | stopped at 89% |
| math-20k · frame | 31.5 ms (0.06) | 590 ms (2.06) | 617 ms (2.12) | 541 ms (2.01) | 577 ms (2.07) | 13794 ms (23.2) | 14441 ms (24.4) |
| code-50k · token | 125 ms (0.01) | 19564 ms (3.12) | 20616 ms (3.27) | 18455 ms (3.01) | 19535 ms (3.17) | stopped at 29% | stopped at 28% |
| code-50k · frame | 36.1 ms (0.02) | 4590 ms (3.20) | 4855 ms (3.36) | 4357 ms (3.10) | 4624 ms (3.24) | stopped at 61% | stopped at 59% |
| table-20k · frame | 30.5 ms (0.10) | 6550 ms (13.8) | 6118 ms (12.9) | 814 ms (4.47) | 959 ms (4.23) | 6541 ms (13.8) | 6115 ms (12.8) |
| long-100k · frame | 240 ms (0.09) | 2973 ms (2.01) | 3144 ms (2.07) | 2805 ms (1.93) | 2995 ms (2.02) | stopped at 29% | stopped at 29% |
| crlf-20k · frame | 35.2 ms (0.07) | 859 ms (3.26) | 887 ms (3.34) | 808 ms (3.20) | 843 ms (3.26) | 13442 ms (26.2) | 13681 ms (26.7) |
| footnotes-20k · frame | 32.4 ms (0.06) | 549 ms (1.51) | 575 ms (1.52) | 515 ms (1.50) | 542 ms (1.49) | 10551 ms (18.8) | 10941 ms (19.7) |
| thread-30 · frame | 65.6 ms (0.18) | 345 ms (2.17) | 364 ms (2.26) | 316 ms (2.09) | 336 ms (2.17) | 2924 ms (11.9) | 3187 ms (12.7) |

## Frame budget

Updates longer than 16.7 ms / longer than 50 ms / the longest update.

| | `llx` (M5, bun, DOM) | stream w78 | stream w98 | directive w78 | directive w98 | naive w78 | naive w98 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| chat-5k · token | 0 / 0 / 0.14 ms | 0 / 0 / 0.70 ms | 0 / 0 / 0.76 ms | 0 / 0 / 0.78 ms | 0 / 0 / 0.74 ms | 0 / 0 / 3.30 ms | 0 / 0 / 3.87 ms |
| chat-5k · frame | 0 / 0 / 0.16 ms | 0 / 0 / 0.68 ms | 0 / 0 / 0.81 ms | 0 / 0 / 0.78 ms | 0 / 0 / 0.75 ms | 0 / 0 / 3.20 ms | 0 / 0 / 3.81 ms |
| answer-20k · token | 0 / 0 / 1.38 ms | 0 / 0 / 5.53 ms | 0 / 0 / 5.58 ms | 0 / 0 / 4.26 ms | 0 / 0 / 3.26 ms | 237 / 0 / 23.6 ms (stopped at 81%) | 350 / 0 / 24.4 ms (stopped at 79%) |
| answer-20k · frame | 0 / 0 / 1.54 ms | 0 / 0 / 2.98 ms | 0 / 0 / 2.94 ms | 0 / 0 / 2.87 ms | 0 / 0 / 3.16 ms | 251 / 0 / 22.8 ms | 290 / 0 / 24.5 ms |
| answer-20k · line | 0 / 0 / 0.29 ms | 0 / 0 / 3.44 ms | 0 / 0 / 3.40 ms | 0 / 0 / 3.29 ms | 0 / 0 / 3.57 ms | 116 / 0 / 23.5 ms | 139 / 0 / 25.3 ms |
| forms-20k · frame | 0 / 0 / 1.58 ms | 60 / 0 / 26.0 ms | 60 / 0 / 26.8 ms | 0 / 0 / 11.1 ms | 0 / 0 / 10.2 ms | 629 / 251 / 66.5 ms (stopped at 92%) | 650 / 240 / 73.2 ms (stopped at 89%) |
| math-20k · frame | 0 / 0 / 1.77 ms | 0 / 0 / 5.45 ms | 0 / 0 / 5.88 ms | 0 / 0 / 5.21 ms | 0 / 0 / 5.55 ms | 420 / 0 / 24.8 ms | 442 / 0 / 25.8 ms |
| code-50k · token | 0 / 0 / 1.74 ms | 1 / 1 / 68.2 ms | 1 / 1 / 69.1 ms | 1 / 1 / 68.4 ms | 1 / 1 / 70.6 ms | 361 / 0 / 23.7 ms (stopped at 29%) | 417 / 0 / 20.4 ms (stopped at 28%) |
| code-50k · frame | 0 / 0 / 1.23 ms | 1 / 1 / 70.7 ms | 1 / 1 / 72.4 ms | 1 / 1 / 71.5 ms | 1 / 1 / 73.1 ms | 874 / 0 / 42.2 ms (stopped at 61%) | 865 / 0 / 42.4 ms (stopped at 59%) |
| table-20k · frame | 0 / 0 / 1.53 ms | 0 / 0 / 15.1 ms | 0 / 0 / 14.0 ms | 1 / 0 / 16.7 ms | 0 / 0 / 13.6 ms | 0 / 0 / 16.2 ms | 0 / 0 / 13.9 ms |
| long-100k · frame | 0 / 0 / 1.60 ms | 0 / 0 / 5.44 ms | 0 / 0 / 5.83 ms | 0 / 0 / 5.42 ms | 0 / 0 / 5.53 ms | 935 / 0 / 37.2 ms (stopped at 29%) | 921 / 0 / 38.1 ms (stopped at 29%) |
| crlf-20k · frame | 0 / 0 / 1.35 ms | 0 / 0 / 6.18 ms | 0 / 0 / 6.07 ms | 0 / 0 / 5.96 ms | 0 / 0 / 6.19 ms | 430 / 0 / 28.3 ms | 441 / 0 / 28.3 ms |
| footnotes-20k · frame | 0 / 0 / 2.52 ms | 1 / 0 / 20.2 ms | 1 / 0 / 21.6 ms | 1 / 0 / 21.8 ms | 1 / 0 / 22.6 ms | 184 / 0 / 20.6 ms | 218 / 0 / 21.6 ms |
| thread-30 · frame | 0 / 0 / 1.51 ms | 0 / 0 / 3.31 ms | 0 / 0 / 3.26 ms | 0 / 0 / 3.21 ms | 0 / 0 / 3.16 ms | 0 / 0 / 12.8 ms | 0 / 0 / 13.8 ms |
