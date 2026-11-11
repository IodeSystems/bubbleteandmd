# Plan

How this plan works: rules in ~/CLAUDE.md "Planning" (plan.md = current
state + active work; done.md = archive; icebox.md = deferred).

Source of the work: owner brief from dun-bf, 2026-11-10 (Carl approved).
Layers in order, each usable alone. dun pins tags; tags/push need Carl's OK.

## Active work

### ✅ 0. Measure glamour whole-document render cost
Result (bench/, glamour v2.0.1, dun options render.go:84, Ryzen 9 3900X): linear ~2.3–2.6 ms per 1k chars;
10k = 23 ms, 25k = 57 ms, 100k = 230–262 ms per render. One render > 33 ms (one 30 Hz frame) past ~13k chars.
100k reply at 1 render/frame: ~2900 s CPU over an ~833 s stream. Corpus is synthetic (bench/testdata/reply.md).

### ◻ 1. Streaming core
splitter (REOPENED = 2) + per-block cache (source, width, style) + healer + glamour per block + join.
Invariant test: at every chunk boundary, final streamed frame == whole-doc glamour render; healed text never in stored source.
Join rule (planner, from glamour v2.0.1 source, unverified): block i>0 rendered as "<!-- -->\n\n"+block so the
first node has a previous sibling; strip doc prefix/suffix P,S per render; concat; wrap P…S. No footnotes in
glamour (GFM+DefinitionList only) → `[^x]:` is a link-ref def → whole-doc block. Splitter must not cut before `: ` (deflist).
DAG:
- ✅ N1 join spike `internal/spike/join_spike_test.go`: 102/102 (17 docs × dark/light/notty × w40/80) with fixed rule:
  seg i>0 = sentinel + seg + "\n" + all link-ref defs re-emitted; strip P,S; skip empty pieces; if prev piece ends
  without "\n", drop its last line's visible width in cells from the next piece's first line; wrap P…S.
  Untested: fences in list items, wrapped text in lists, other widths. Cell-dropping is fragile → N7 must cover.
- ◻ S1 not needed (D3 = one root package; internal/ for splitter, healer, lru).
- ✅ N2 internal/corpus: 14 llx docs + emoji.md (Go-only), 10 scenarios, mulberry32, prefixes cut on rune boundaries. Scenario bytes not checked vs TS.
- ◐ N3 port blocks.ts splitter, REOPENED=2 (opus).
- ✅ N4 internal/heal: 42/43 heal.test.ts cases (the cache-linearity test dropped: no cache, rescans per call). CRLF fences never closed (llx parity).
- ✅ N5 internal/lru: count bound only (no weight).
- ◻ N6 stream.Stream (N1,N3,N4,N5) · N7 invariant test every chunk boundary (N2,N6) · N8 splitter vs goldmark differential (N2,N3).
- ◻ N9 bench Stream vs whole · N10 docs.
- ✅ N3 internal/blocks · ✅ N8 goldmark differential (no mismatches).
- **next**: N6+N7 result → ship layer 1 (Carl 2026-11-10: ship layer 1 first, then layer 2). Before tag/push: confirm version + push with Carl; then message dun-bf version + API.
- **risks**: SGR pen carry across blocks (margin.go:159-201); sentinel may emit bytes; scanner written for remark, parser here is goldmark; dun's renderAnswer/splitWide is not one whole-doc render, so the invariant does not cover dun's wide path; token-prefix invariant is O(n²).
- **blocking decisions**: D4 splitWide ownership (dun-bf, open). D6 default: token-prefix run skipped under `-short` (assumption).
- **optional extensions**: holdHeight-style no-shrink; splitWide port.

### ◐ 2. Directive blocks (lipgloss, static)
Design (planner 2026-11-10): root Renderer implements layer-1 Renderer; per top-level block, internal/directive.Parse
(goldmark AST) → lipgloss piece, else glamour. No directive → glamour(doc) unchanged. Directive piece copies glamour's
piece frame (leading blank, margin, right pad, trailing \n) measured per (style,width). Bodies render recursively.
Task lists already in glamour. Mermaid/math stay code.
DAG: ◐ D1 frame spike · ◐ D2+D3 scaffold + parsers · ◻ D4 Theme (B5) · ◻ D5a-d renderers (goldens, 3 styles × w40/80)
· ◻ D6 root Renderer (needs layer-1 root) · ◻ D7 invariant with our Renderer · ◻ D8 bench · ◻ D9 docs · ◻ D10 Go format prompt.
- **next**: D1, D2+D3 running; Carl answers B1-B7, O1-O3.
- **risks**: frame bytes must match exactly (pad, empty SGR, pen carry); link-ref defs not visible in directive bodies;
  goldmark vs remark list reading; growing tail directive re-rendered every frame; pros alone stacked → side by side when cons arrives.
- **blocking decisions**: B1 NewRenderer(StyleConfig,width) vs wrap TermRenderer · B2 nesting scope · B3 DETAILS static open/summary
  · B4 callout label without title · B5 hue table vs derive from glamour · B6 pros/cons threshold 64 · B7 $$ → math fence.
- **optional extensions**: O1 section numbers · O2 figures · O3 raw <details> as DETAILS.
### ◻ 3. Interactive bubbletea component (fold, tabs, state per block id)

## Decisions / assumptions
- Carl 2026-11-10: D1 merge mismatched kinds into one block · D2 port all of heal.ts · D3 one root package, rest internal/ · D5 port generator to Go.
- Assumption: benchmark code lives in the repo (`bench/` package, `_test.go`), committed as `test:`/`chore:`.
