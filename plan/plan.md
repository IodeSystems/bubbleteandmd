# Plan

How this plan works: rules in ~/CLAUDE.md "Planning" (plan.md = current
state + active work; done.md = archive; icebox.md = deferred).

Source of the work: owner brief from dun-bf, 2026-11-10 (Carl approved).
Layers in order, each usable alone. dun pins tags; tags/push need Carl's OK.

## Active work

- ✅ 0. Measure — see done.md.
- ✅ 1. Streaming core — v0.1.0, see done.md. Open: D4 splitWide ownership (dun-bf). Chroma tie colours / per-process
  chroma theme registration is upstream glamour/chroma (docs/decisions.md); tell dun-bf.

### ◐ 2. Directive blocks (lipgloss, static)
Design (planner 2026-11-10): root Renderer implements layer-1 Renderer; per top-level block, internal/directive.Parse
(goldmark AST) → lipgloss piece, else glamour. No directive → glamour(doc) unchanged. Directive piece copies glamour's
piece frame (leading blank, margin, right pad, trailing \n) measured per (style,width). Bodies render recursively.
Task lists already in glamour. Mermaid/math stay code.
DAG: ✅ D1 frame spike (internal/spike/frame_spike_test.go: margin 2 plain, content W-4, pad cells styled per style; dark/light code piece ends without \n) · ✅ D2+D3 internal/directive parsers (33/33 forms, 4/4 dialect) · ✅ D4 Theme + Frame · ✅ D5a-d renderers (goldens, 3 styles; math has no Kind → D6 rewrites $$ to ```math)
· ✅ D6+D7 NewRenderer → *DirectiveRenderer (name clash with layer-1 Renderer interface; Carl to confirm) + invariant · ◻ D8 bench · ◻ D9 docs · ◐ D10 Go format prompt (prompt/).
- **next**: D6+D7, D10 running → O1-O3 → D8 bench → D9 docs → v0.2.0 (needs Carl OK).
- Open cosmetics: notty callout bar uses `| `, details `▾` kept; source titles lose inline emphasis; stat with no value leaves empty row.
- **risks**: frame bytes must match exactly (pad, empty SGR, pen carry); link-ref defs not visible in directive bodies;
  goldmark vs remark list reading; growing tail directive re-rendered every frame; pros alone stacked → side by side when cons arrives.
- **blocking decisions**: llx-deviation: DETAILS title rule (parser uses callout rule; llx drops plain title before formatted line — looks like llx bug) needs Carl. One-line `> quote — Author` not attributed (follows llx code, not syntax.md).
- Carl 2026-11-10: B1 NewRenderer(style,width) · B2 top level + inside form bodies · B3 DETAILS open · B4/B5 kind label always + fixed GitHub hues, notty plain · B6 64 cells · B7 $$ as math code · O1 section numbers, O2 figures, O3 raw <details> IN scope.
### ◻ 3. Interactive bubbletea component (fold, tabs, state per block id)

### ◐ Fixes (review + audit 2026-11-10)
- ✅ S1 escape injection: Sanitize before split/heal/cache (c1d5939); DirectiveRenderer.Render calls it (cf7e63f).
- ✅ R1 cache keys copy block source (c28cc3d) · ✅ R2 ErrTrimmed for a Renderer that trims newlines + Renderer doc (1ff8485)
  · ✅ R3 no heal change (TS behaviour); Stream CRLF test (1fa93b8) · ✅ R4 dedentItem tabs to 4-col stops (67949f2)
  · ✅ R5 Frame clamps width<0; NewRenderer w0..5 test (4acb1f9; NewRenderer did not panic before) · ✅ R6 heal fence indent spaces only (dade43f)
  · ✅ R7 titled code lines cut to width (f3647e0) · ✅ R8 pad cell as glamour writes it; colour 8 dark text (ebcfb43)
  · ✅ R9 DropCells reads CSI/OSC, incomplete safe (6aa9119) · ✅ R10 README "Use"; prompt.Examples() returns a copy (f2146f4).
- ◻ New (found in R6): heal inline fixes apply inside indented code (`\t```` tail gets a "`" while streaming). TS has it too. Fix needs a list-aware
  "is indented code" test in heal; a naive 4-space rule would stop healing list continuation paragraphs.
- ◐ Bench vs llx: Go llx-method bench (worktree agent); llx's own bench on M1 Max 10.100.0.3 (office VPN) with local bun in ~/tmp/bubbleteandmd-bench.
- **next**: v0.2.0 ask (Carl). **risks**: R9 changes join for every seam (invariant passes, see run); R2 adds one probe render per Stream.

### ◐ Speed: growing tail block (planner 2026-11-10)
llx-method bench: 30-100× slower than llx; worst = one big growing tail (code-50k stops at 30 s, table-20k 13.5 s, forms-20k 9.5 s).
Findings: code splice possible only at safe token cuts (named lang, root lexer state); glamour builds code with s+= (O(n²), elements.go:336);
tables not spliceable (lipgloss widths from all rows); tail = last 2 blocks re-rendered (stream.go:117-143); Sanitize O(n) per frame.
forms-20k bench used plain glamour, not DirectiveRenderer.
- ✅ N1 profile: tail render 88-96%; code-50k chroma lex 38% + writers 40%; table-20k lipgloss table.String 48%; forms-20k chroma lexers.Get filename-glob 32% (unknown fence lang), tail = 2 blocks in ~100% frames.
- ✅ N2 code splice: raw 0%; with pen-carry repair exact only at safe cuts; cheap predictor 96% precision → not exact. Whole lex 77 ms of 212 ms at 50k.
- ✅ N3 tables: widths settle by row ≤21; rows splice byte-equal 100% when widths stable; width check 0.35 ms/160 rows.
- M1 Max same-machine (bench/results/llx-m1max-*.md vs llx-method-m1max-*.md): ours 5-139× slower than llx; code-50k stops at 30 s.
- ✅ N4 tail = last block (76f419e): block n-2 memoized per Stream by source (splitter can reopen it: [!CONS] under [!PROS]), to Cache when stable; heal falls back to last two blocks when heal crosses the boundary (callout marker drop eats the "\n" before). Full invariants pass (470988 + 324648 checks).
- ✅ N5 incremental Sanitize (5b97d18): kept prefix to last "\n"; FuzzSanitizeSplit.
- N4+N5 evidence (2026-11-11, dev box, load avg ~45 from other agents, so ms is noisy): bytes passed to Render per frame-pattern stream, w78, a4171cf → 5b97d18: answer-20k 640k→334k, long-100k 3.64M→1.81M, footnotes-20k 837k→468k, thread-30 370k→205k, forms-20k 1.99M→1.48M (forms ms unchanged ~15 s: one big tail block; chroma lexers.Get, N1). BENCH_LLX stream w78 totals before→after: answer token 5690→4478, frame 1324→1250, line 568→854, forms 8919→14027, long 8745→7926, footnotes 1840→1879, thread 853→1133 ms; naive slowed 1.5-2× in the same window, so ms totals are not comparable.
- ✅ N6 code chunker: internal/codestream.Fence (e5a5a46): code-50k 3.9 ms/frame warm vs 125 ms whole; 28758/28764 chunked frames byte-equal; lines+text always equal. Wired into Stream Frame (e0dabcb): last-block fence tail, raw glamour and DirectiveRenderer (titled fence / tree are forms: whole). Invariant relaxed to SGR-only ONLY when Stream reports a chunk splice; the test fails if that happens on a tail that is not a fence of >40 lines.
- ✅ N7 table: internal/tablestream (735fd3f) byte-equal, table-20k 2.1 ms/frame vs 20.7. Wired (561100b): optional `GlamourOptions() (ansi.Options, bool)`; DirectiveRenderer implements it, ok=false when NewRenderer got glamour opts (cannot be read back). **Deviation**: planned signature had no bool. One Table per tail offset, last block only; its renders go through the Stream's Renderer, so bytes are r.Render's. Raw glamour: whole tail. Form-body tables: whole, unchanged. TestTableFastPath: table-20k w78 dark 831/892 frames spliced.
- ✅ N8 M1 Max rerun, `directive` stream column added (bench/results/llx-method-m1max-f88f0d4.md; before = llx-method-m1max-2026-11-10.md at a4171cf, so N4+N5 gains are in the delta too). w78 ms, before → after (directive column; stream column within ~5% except table/forms): code-50k frame stopped 59% → 4906, token stopped 28% → 20864; table-20k 7236 → 901 (raw-glamour stream 7208: no options); forms-20k 6614 → 3298 (stream 5618); answer-20k frame 913 → 541; long-100k 5650 → 3223. Still 5-85× llx on M1 (llx M1: code frame 66.6, table 52.2, forms 122).
- Full suite on dev box: TestInvariant 470988/470988, TestRendererInvariant 385812/385812 (20513 tail tables spliced); growing-fence relaxed checks 9756 per test, 6 differ (SGR only).
- ◻ N9 docs (README: GlamourOptions; Frame SGR exception for growing fences)
- Carl 2026-11-11: B1 code = approximate chunked Frame + exact Final (lines never move; seam colours may change at end) · B2 tables = T5 exact row splice, whole render on width change · B3 = draft upstream issue texts only, Carl files them.

### ◐ Deps + local caches (Carl 2026-11-11)
- Bump to latest tags: chroma 2.27, goldmark 1.8.x, lipgloss 2.1.0, x/ansi 0.11.9; glamour stays v2.0.1 (v2.0.2 tag request drafted for Carl).
- ◐ 5 fence-language cache in DirectiveRenderer: unknown/empty lang → `text` (colours only change) — worktree agent.
- ◻ 4 cached terminal256 formatter under a new chroma name via glamour.WithChromaFormatter (no shared value changed); prototype byte-equal, small blocks ~5× faster (scratchpad fmt4/).
- Upstream: report only what reproduces on latest (memory rule); 6 drafts left in scratchpad upstream-issues.md for Carl.
- **next**: merge 5 → bump + 4 → invariants + bench on M1 → ask Carl v0.3.0.

## Decisions / assumptions
- Carl 2026-11-10: D1 merge mismatched kinds into one block · D2 port all of heal.ts · D3 one root package, rest internal/ · D5 port generator to Go.
- Assumption: benchmark code lives in the repo (`bench/` package, `_test.go`), committed as `test:`/`chore:`.
