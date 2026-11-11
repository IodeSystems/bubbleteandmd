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
- ◻ N2 port corpora as Go testdata + Go port of the scenario generator (D5).
- ◻ N3 port blocks.ts splitter, REOPENED=2 (opus) · N4 port heal.ts (sonnet, scope D2) · N5 bounded LRU (haiku).
- ◻ N6 stream.Stream (N1,N3,N4,N5) · N7 invariant test every chunk boundary (N2,N6) · N8 splitter vs goldmark differential (N2,N3).
- ◻ N9 bench Stream vs whole · N10 docs.
- **next**: N2–N5 in parallel.
- **risks**: SGR pen carry across blocks (margin.go:159-201); sentinel may emit bytes; scanner written for remark, parser here is goldmark; dun's renderAnswer/splitWide is not one whole-doc render, so the invariant does not cover dun's wide path; token-prefix invariant is O(n²).
- **blocking decisions**: D4 splitWide ownership (dun-bf, open). D6 default: token-prefix run skipped under `-short` (assumption).
- **optional extensions**: holdHeight-style no-shrink; splitWide port.

### ◻ 2. Directive blocks (lipgloss, static)
### ◻ 3. Interactive bubbletea component (fold, tabs, state per block id)

## Decisions / assumptions
- Carl 2026-11-10: D1 merge mismatched kinds into one block · D2 port all of heal.ts · D3 one root package, rest internal/ · D5 port generator to Go.
- Assumption: benchmark code lives in the repo (`bench/` package, `_test.go`), committed as `test:`/`chore:`.
