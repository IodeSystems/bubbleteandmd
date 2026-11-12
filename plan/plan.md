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
### ◐ 3. Interactive component (planner 2026-11-11)
Shape: not a tea.Model (host needs "key consumed" bool). `Doc` per reply wraps Stream + state map; `Frame/Final`, `Do(Action) bool`,
`HandleKey(tea.KeyPressMsg, KeyMap) bool`, `Focus/Blur`, `Stops() []Stop{Key,Kind,Line,Height}`, `Reset()`.
Carl 2026-11-11: layout stays as streamed (tabs stacked, details/trees open) until the user interacts; stream end changes nothing ·
bubbletea import OK for thin HandleKey · tab/shift+tab move stops, enter/space toggle, ←/→ tabs, ↑/↓ = scroll · tabs switch to
one-tab view on ←/→ or enter, not on focus · tree = one stop, rows walked inside · start now, before dun adopts Stream ·
viewport: constrained (host sets height) → component owns scroll (↑/↓ PgUp/PgDn, focus scrolls into view, follow bottom unless
scrolled up); unconstrained (height 0) → render all, scroll keys return false, host scrolls via Stops().
- ✅ L1 line map spike (1a964bf): exact from newline counts; 43042 marks/config, 0 wrong; 3-16 µs/frame. API: Stream.FrameSpans.
- ✅ L2 directive.IDs (0e41a93): 118745 IDs at every frame prefix, 0 kind changes, 0 collisions.
- ✅ L3 stateful renderers + Theme.Focus + marks (d3002ba): `directive.State` (Tabs/Details/Trees by FormID.Key, Focused);
  `Ctx{State, Key, Body}`; `Render` → (lines, []Mark{Key, Kind header|panel|row, Line, Height, Index, Path}, err), After drawn inside Render;
  unexported `DirectiveRenderer.renderState(md, offset, st)` (marks counted in out, P lines included). Glyphs: tabbed `‹i/n› Label ───` + selected panel only;
  folded `▸ Title`; tree with a State entry: `▾ name/` / `▸ name/`, rows under a closed folder hidden. Focus: reverse video (Theme.Focus, Inherit over
  the part's style); notty: no visible focus (no SGR in notty), host reads State.Focused. Tests: zero State = 36 doc goldens (testdata/golden/docs) whole
  and per block; mark keys = interactive IDs keys (48 docs incl. forms-20k, fuzz); line map 4 states × 3 styles × w36/76; 90 state goldens.
- ✅ L4 state-aware Stream: root aliases State/TabsState/DetailsState/TreeState/Mark/MarkKind(+consts); `StateRenderer{Renderer; RenderState(md, offset, *State) (string, []Mark, error)}`
  (DirectiveRenderer.RenderState, was renderState); `Stream.SetState(*State)` (read every call), `Stream.Marks() []Mark` (last output, lines in it).
  Cache key + near memo += block offset + stateKey(st, [at, at+len)) (entries whose top-level key is in the block, focus too; sorted). Plain Renderer: offset 0, "" (as before).
  Fast paths: fence/table render through tailRenderer (RenderState), marks kept from the whole-tail fallback render; held/chunks/rows have no forms.
  `$$` fix: MathFences moved to internal/directive with shifts; renderer keys = RawOffset; IDs (and test formsIn) read MathFences(block) too → keys agree also in tab panels/bodies.
  Tests: TestStreamStateInvariant (5104 line prefixes, 4 states × dark/notty × w40/78: Final and Frame (healed; held/chunked aside) + Marks == RenderState), TestStreamStateCache, TestStateKey, TestStateMathBeforeForm, TestStateFastPaths.
- ✅ L5 Doc (bc32fc7; decisions.md 2026-11-11 L5): `NewDoc(r, key, cache)` wraps a Stream, owns State; `Frame/Final`, `Do(Action) bool`,
  `HandleKey(tea.KeyPressMsg, KeyMap) bool` (only file importing bubbletea: keymap.go), `Focus/FocusLast/Blur/Reset`, `Stops() []Stop{Key,Kind,Line,Height}`,
  `Focused() (Stop,bool)`, `State()`. Actions focus.next/prev, toggle, move.left/right, focus.leave; `DefaultKeyMap()`, `KeyMap.Label/Action`. Mark += Panels, Folder.
  Dep: bubbletea v2.1.0 (MVS: ultraviolet, go-runewidth 0.27→0.30, terminfo v1.0.0; dun already higher). No NormalizeKey (reason in decisions.md).
  Tests (interact_test.go): stops per kind, layout == Stream at every frame + focus keeps widths (dark/light/notty, w36/76), stream order == IDs order
  (stateDocs, dark/notty), actions per form, consumed/fallthrough, plain Renderer.
- ✅ L5b viewport (viewport.go; decisions.md 2026-11-11 L5b): `SetHeight/Height/Offset/Lines/View`; actions scroll.up/down, page.up/down,
  scroll.top/bottom (↑/↓ pgup/pgdown home/end in DefaultKeyMap). h>0: Frame/Final/View = lines [Offset, Offset+h); scroll consumed with or without
  focus and at edges; follow bottom; focus change → stop into view (tall: top); tree cursor → row into view. h=0 unchanged.
  Tests (viewport_test.go): window == slice of Stream output at every frame (dark/notty, h 1/5/12), follow vs scrolled-up (view bytes fixed), focus
  into view per stop (h 2/4, 3 styles), keys per edge, SetHeight.
  **risks (L5b)**: Offset is stale between Focus/an action and the next render (reveal applies at render); scrolled-up view over the healed tail
  can change bytes (offset still fixed); toggle/tab switch do not scroll (focused header can stay out of view only if it already was).
  **optional extensions**: scroll indicator (host can draw from Offset/Lines/Height); scroll inside a tall focused stop on ↑/↓.
- ✅ L6 audit (decisions.md "L6 audit fixes"): viewport opens at top when the first output overflows h, else bottom (SetHeight order no
  longer matters); tablehold counts `x | y` body rows; tree toggle always consumed on a focused tree; Doc/viewport cursor row =
  renderer's (`directive.CursorRow`); scroll drops a pending reveal (kept, tested); width change = new Doc + `*nd.State() = *old.State()`
  (TestDocStateNewWidth, no new API); Doc comment states the held-table exception.
  Reported, not changed: lone header line → paragraph → `table…`; a wrapped header shrinks that frame (79 cells w50: 6→5 lines).
  **risks (L6)**: h below the first frame's height (h 1-2) or a tall first frame opens at the top and does not follow; CursorRow
  reads ancestry from TreeRow.Path (`plain#i`): a made-up Cursor path under a drawn folder now shows on that folder (was: row 0).
  `go test -short -timeout 60m ./...` with L6: all ok (exit 0), root 524 s, vet clean.
- ◻ L7 docs
  dun adoption: NewStream → NewDoc, Focus when its answer is entered (like insideWide); viewport mode only if dun hands the pane to the Doc.
  **blocking decision (Carl, open 2026-11-11)**: whether/how dun's input handling splits out into a shared lib — L5 built only what Doc needs.
  **risks (L5)**: Stops are stale between a consumed key and the next render (host must render after true); notty focus on a tree whose cursor is a
  depth-0 file row, and on stacked tabs, has no cue (Carl's notty rule); details stop covers the title only, tabs stop covers all drawn panels;
  ←/→ wrap (not return false at an edge) — assumed. Doc with no stops yet: Focus false, host must call it again when forms arrive.
- `go test -short -timeout 60m ./...` with L5b: all ok (exit 0), root 588 s, vet clean.
- `go test -short -timeout 60m ./...` at 3e6517b (L5): all ok, root 426 s, vet clean.
- `go test -short ./...` at 1d9bacc: all ok (root 483 s with -timeout 60m; default 10m timed out under load ~50, Render vs RenderState cost equal on forms-20k).
- **risks (L4)**: full (non-short) TestInvariant/TestRendererInvariant not run on dev box (run on M1). With a StateRenderer
  the Cache key holds the block offset: same source at another offset misses. A held table tail followed in the same block by a form (no blank line) shows no marks.
  stateKey scans all State entries per stable block per call (fine for few entries).
- **risks**: dun still renders raw glamour + splitWide (only calls Sanitize) → nothing reaches dun until D4; interacted tail form re-renders whole each frame; tab switch changes height.
- notty focus: same-width glyph swap (233d92d; Carl): ▸→▶, ▾→▼, ‹›→«», tree guide ──→━━; no cue on depth-0 rows without fold mark and stacked tab labels (decisions.md).
- ✅ list-item fence no-shrink (c98856f): heal closes a fence at the opener's indent. Gap: opener indented ≥4 (nested list) gets no heal closer.
- M1 full suite: c98856f all pass (root 1132 s). 82af877 (L4) not yet run on M1.
- **release**: v0.4.0 tag + push main waits on Carl (auto-mode blocks agent push); suggested at c98856f.
- **risks (L3)**: TestLineMapSpike fails at d334510 too (mirror text != Stream on dir-code.md, doc-reply: 3 of 1266 frames; marks all ok) — stale mirror, not L3. (`$$` before a form: fixed in L4.) Mark Lines of a form in a side-by-side verdict column are rows, not columns. Tree file rows get no glyph pad (folder names shift 2 cells on first interaction).

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
- ✅ Bump (a381e0d): chroma 2.14.0→2.27.0, goldmark 1.7.8→1.8.7, lipgloss 2.0.4→2.1.0, x/ansi 0.11.7→0.11.9; glamour v2.0.1. go directive 1.26.7.
  Goldens: codetitle_{dark,light}_{36,76} SGR only (chroma closes escapes before "\n"). dun higher on indirect: goldmark-emoji 1.0.6, x/sync 0.24.0, x/exp/slice v0.1.0 (MVS in dun picks those).
- ✅ tablestream width model ports lipgloss 2.1.0 minWidth floor (6199992; TestWidthModel, NOTICE).
- ✅ 5 fence-language cache (17bb6d3).
- ✅ 4 cached formatter `bubbleteandmd-terminal256` (ccd1caf, chromafmt.go). Finding: stock terminal256 is not deterministic on palette ties (20/75 chroma styles, glamour dark Comment 241|242); cached keeps the first choice per process.
- ✅ M1 Max at ccd1caf: vet clean; full `go test` all ok except TestRendererPlain (fuzz-47, pre-existing since 17bb6d3: test did not expect the fence-lang rewrite; fixed in 4f5942c; root package rerun on M1 with the fix: ok, 608 s).
  Bench (bench/results/llx-method-m1max-ccd1caf.md) directive w78 f88f0d4 → ccd1caf: code-50k frame 4906→4357, token 20864→18455, forms-20k 3298→2712, table-20k 901→814, long-100k 3223→2805, answer-20k frame 541→469 ms. Still 2.7-76× llx M1 (thread-30 316 vs 118; code-50k token 18455 vs 242).
- Upstream: report only what reproduces on latest (memory rule); 6 drafts left in scratchpad upstream-issues.md for Carl. Candidate: chroma findClosest tie (still on v2.27.0).
- **next**: M1 results → ask Carl v0.3.0.
- **risks**: formatter registry write is unlocked (first NewRenderer before concurrent renders; documented on NewRenderer). Tied colours differ between processes (as stock).
- **optional extensions**: deterministic tie-break (lowest palette index) in the cached formatter — changes bytes vs stock's random pick, needs Carl.

### ◐ Wide blocks (Carl + dun-bf decision C, 2026-11-11)
dun's splitWide ported as `DirectiveRenderer.WideBlocks(pane)` (wide.go; tablestream.Natural; decisions.md).
- ✅ render + cut + note, Stream wide-table fast path, tests (wide_test.go), bench (bench/wide_bench_test.go).
- **next**: dun adopts it (dun-bf): NewRenderer(style, proseW) + WideBlocks(avail), drop splitWide/renderAnswer.
- **risks**: code width read from the source (tab = 4): a tab-heavy line can wrap by a cell or two; table rows with
  an email or emoji shortcode are not wide (width model refuses them); lines past 512 cells wrap.
- **blocking decisions**: none. Assumed: note style Theme.Muted; pane < width read as width.
- **optional extensions**: sideways offset (dun's ←/→) as layer-3 state; wide blocks in lists/quotes/form bodies.

## Decisions / assumptions
- Carl 2026-11-10: D1 merge mismatched kinds into one block · D2 port all of heal.ts · D3 one root package, rest internal/ · D5 port generator to Go.
- Assumption: benchmark code lives in the repo (`bench/` package, `_test.go`), committed as `test:`/`chore:`.
