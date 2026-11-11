# bubbleteandmd streaming cost, llx-markdown's method

**Not like-for-like.** The `llx` column is llx-markdown 0.4.1 on an Apple M5 with bun, rendering React to a happy-dom DOM (llx `bench/results/latest.md`, commit ad88e30). Our columns are unknown with Go, rendering to ANSI strings through glamour. Compare shapes (growth, which rows stay under a frame), not absolute times.

- **date:** 2026-11-11
- **cpu:** unknown (10 threads)
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
| chat-5k · token | 22.1 ms (0.03) | 205 ms (0.39) | 224 ms (0.44) | 215 ms (0.41) | 234 ms (0.46) | 1712 ms (2.86) | 2022 ms (3.30) |
| chat-5k · frame | 9.25 ms (0.07) | 51.7 ms (0.44) | 56.9 ms (0.48) | 54.1 ms (0.46) | 58.7 ms (0.48) | 394 ms (2.89) | 471 ms (3.39) |
| answer-20k · token | 86.9 ms (0.03) | 2000 ms (1.86) | 2117 ms (1.91) | 1789 ms (1.73) | 1883 ms (1.80) | stopped at 81% | stopped at 79% |
| answer-20k · frame | 34.4 ms (0.09) | 476 ms (1.83) | 508 ms (1.91) | 433 ms (1.78) | 459 ms (1.82) | 10559 ms (20.8) | 11320 ms (22.6) |
| answer-20k · line | 16.2 ms (0.10) | 240 ms (2.35) | 257 ms (2.46) | 222 ms (2.26) | 235 ms (2.35) | 4167 ms (22.3) | 4450 ms (24.1) |
| forms-20k · frame | 67.3 ms (0.15) | 5764 ms (17.5) | 6015 ms (18.1) | 2722 ms (7.06) | 2910 ms (7.46) | stopped at 92% | stopped at 89% |
| math-20k · frame | 31.5 ms (0.06) | 543 ms (2.08) | 576 ms (2.20) | 494 ms (1.96) | 521 ms (2.01) | 13772 ms (23.1) | 14482 ms (24.4) |
| code-50k · token | 125 ms (0.01) | 19773 ms (3.15) | 20855 ms (3.32) | 18514 ms (3.02) | 19577 ms (3.18) | stopped at 29% | stopped at 28% |
| code-50k · frame | 36.1 ms (0.02) | 4661 ms (3.22) | 4868 ms (3.37) | 4388 ms (3.11) | 4614 ms (3.25) | stopped at 61% | stopped at 59% |
| table-20k · frame | 30.5 ms (0.10) | 6525 ms (13.9) | 6120 ms (13.0) | 822 ms (4.65) | 970 ms (4.38) | 6621 ms (13.9) | 6264 ms (13.1) |
| long-100k · frame | 240 ms (0.09) | 2816 ms (2.02) | 3007 ms (2.10) | 2627 ms (1.93) | 2801 ms (1.99) | stopped at 29% | stopped at 29% |
| crlf-20k · frame | 35.2 ms (0.07) | 837 ms (3.29) | 865 ms (3.38) | 783 ms (3.21) | 806 ms (3.23) | 13513 ms (26.4) | 13740 ms (26.9) |
| footnotes-20k · frame | 32.4 ms (0.06) | 518 ms (1.56) | 543 ms (1.50) | 478 ms (1.52) | 502 ms (1.48) | 10564 ms (18.8) | 10985 ms (19.9) |
| thread-30 · frame | 65.6 ms (0.18) | 334 ms (2.21) | 356 ms (2.23) | 312 ms (2.07) | 326 ms (2.15) | 2938 ms (11.8) | 3211 ms (12.8) |

## Frame budget

Updates longer than 16.7 ms / longer than 50 ms / the longest update.

| | `llx` (M5, bun, DOM) | stream w78 | stream w98 | directive w78 | directive w98 | naive w78 | naive w98 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| chat-5k · token | 0 / 0 / 0.14 ms | 0 / 0 / 0.74 ms | 0 / 0 / 0.81 ms | 0 / 0 / 0.78 ms | 0 / 0 / 0.81 ms | 0 / 0 / 3.43 ms | 0 / 0 / 4.00 ms |
| chat-5k · frame | 0 / 0 / 0.16 ms | 0 / 0 / 0.69 ms | 0 / 0 / 0.73 ms | 0 / 0 / 0.70 ms | 0 / 0 / 0.86 ms | 0 / 0 / 3.24 ms | 0 / 0 / 3.76 ms |
| answer-20k · token | 0 / 0 / 1.38 ms | 0 / 0 / 5.48 ms | 0 / 0 / 5.45 ms | 0 / 0 / 3.11 ms | 0 / 0 / 3.19 ms | 235 / 0 / 22.6 ms (stopped at 81%) | 340 / 0 / 38.2 ms (stopped at 79%) |
| answer-20k · frame | 0 / 0 / 1.54 ms | 0 / 0 / 2.91 ms | 0 / 0 / 3.30 ms | 0 / 0 / 3.11 ms | 0 / 0 / 3.24 ms | 241 / 0 / 23.8 ms | 291 / 0 / 23.9 ms |
| answer-20k · line | 0 / 0 / 0.29 ms | 0 / 0 / 3.29 ms | 0 / 0 / 3.56 ms | 0 / 0 / 3.39 ms | 0 / 0 / 3.46 ms | 121 / 0 / 23.4 ms | 140 / 0 / 26.5 ms |
| forms-20k · frame | 0 / 0 / 1.58 ms | 60 / 0 / 26.2 ms | 60 / 0 / 26.4 ms | 0 / 0 / 9.93 ms | 0 / 0 / 10.3 ms | 627 / 251 / 67.8 ms (stopped at 92%) | 648 / 244 / 72.4 ms (stopped at 89%) |
| math-20k · frame | 0 / 0 / 1.77 ms | 0 / 0 / 5.38 ms | 0 / 0 / 5.51 ms | 0 / 0 / 5.21 ms | 0 / 0 / 5.37 ms | 424 / 0 / 24.5 ms | 442 / 0 / 26.8 ms |
| code-50k · token | 0 / 0 / 1.74 ms | 1 / 1 / 67.1 ms | 1 / 1 / 70.7 ms | 1 / 1 / 67.2 ms | 1 / 1 / 70.7 ms | 373 / 0 / 20.5 ms (stopped at 29%) | 419 / 0 / 22.3 ms (stopped at 28%) |
| code-50k · frame | 0 / 0 / 1.23 ms | 1 / 1 / 70.0 ms | 1 / 1 / 72.3 ms | 1 / 1 / 69.7 ms | 1 / 1 / 72.0 ms | 874 / 0 / 41.3 ms (stopped at 61%) | 864 / 0 / 44.2 ms (stopped at 59%) |
| table-20k · frame | 0 / 0 / 1.53 ms | 0 / 0 / 15.8 ms | 0 / 0 / 14.0 ms | 1 / 0 / 17.2 ms | 0 / 0 / 14.2 ms | 0 / 0 / 15.3 ms | 0 / 0 / 14.1 ms |
| long-100k · frame | 0 / 0 / 1.60 ms | 0 / 0 / 5.55 ms | 0 / 0 / 5.86 ms | 0 / 0 / 5.55 ms | 0 / 0 / 5.78 ms | 932 / 0 / 37.2 ms (stopped at 29%) | 919 / 0 / 38.1 ms (stopped at 29%) |
| crlf-20k · frame | 0 / 0 / 1.35 ms | 0 / 0 / 6.15 ms | 0 / 0 / 6.13 ms | 0 / 0 / 5.80 ms | 0 / 0 / 5.99 ms | 433 / 0 / 28.2 ms | 445 / 0 / 30.4 ms |
| footnotes-20k · frame | 0 / 0 / 2.52 ms | 1 / 0 / 20.8 ms | 1 / 0 / 21.3 ms | 1 / 0 / 21.7 ms | 1 / 0 / 22.9 ms | 190 / 0 / 20.2 ms | 218 / 0 / 21.2 ms |
| thread-30 · frame | 0 / 0 / 1.51 ms | 0 / 0 / 3.31 ms | 0 / 0 / 3.54 ms | 0 / 0 / 3.14 ms | 0 / 0 / 3.27 ms | 0 / 0 / 13.0 ms | 0 / 0 / 13.8 ms |
