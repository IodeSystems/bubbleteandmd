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
- **next**: after 0.
- **risks**: glamour per-block spacing may not join to whole-doc spacing exactly (margins, list/blockquote context).
- **blocking decisions**: none yet.

### ◻ 2. Directive blocks (lipgloss, static)
### ◻ 3. Interactive bubbletea component (fold, tabs, state per block id)

## Decisions / assumptions
- Assumption: benchmark code lives in the repo (`bench/` package, `_test.go`), committed as `test:`/`chore:`.
