// Package bubbleteandmd renders streaming LLM Markdown in a terminal, for
// bubbletea programs. Finished blocks are rendered once and cached; only the
// open tail is rendered again as tokens arrive, and that tail is healed
// (an open fence, `**`, a half-typed link) so raw syntax does not show
// mid-stream. It reads llx-markdown's directive syntax (`> [!STEPS]`,
// `> [!TABS]`, ```tree fences …). See README.md.
package bubbleteandmd
