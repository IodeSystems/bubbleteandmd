# bubbleteandmd

Streaming Markdown for terminal programs built on
[bubbletea](https://github.com/charmbracelet/bubbletea). A Go port of the
ideas in [llx-markdown](https://github.com/rsaryev/llx-markdown) (MIT; see
NOTICE), for the terminal. First user: [dun](https://github.com/iodesystems/dun).

Status: layer 1 (streaming core: `NewStream`, `Frame`, `Final`) in v0.1.0. Layers 2-3 in progress.

## Problem

An LLM reply arrives token by token. Rendering the whole document again per
frame costs more as the reply grows, and half-written syntax (`**bold`, an
open code fence, a link without its `)`) shows raw until it closes.

## Design (three layers, each usable alone)

1. **Streaming core.** A line scanner splits the source into blocks only
   where a full parse would split. Blocks before the last two are final:
   rendered once, cached by (source, width, style). The block before the
   last renders again only when its source changes. The last block (the
   tail) is healed while streaming only — the final text renders as
   written — and rendered again. Lists, blockquotes, footnotes and link definitions force
   a wider re-render where needed. Block rendering uses glamour.
2. **Directive blocks.** llx-markdown's syntax, unchanged, so the same
   model output renders in both: `> [!NOTE]`-style callouts, `> [!STEPS]`,
   `> [!TABS]` + `####` panels, `> [!STATS]`, `> [!PROS]`/`> [!CONS]`,
   `> [!DETAILS]`, `> [!SOURCES]`, ```` ```tree ```` fences, `title="…"` on
   code fences, GFM task lists. Drawn with lipgloss. Mermaid and math show
   as code.
3. **Interactive blocks.** A bubbletea component that holds fold and tab
   state per block id across re-renders: fold `[!DETAILS]` and trees, switch
   tabs. While a block streams, tabs show stacked.

Plus a short format prompt that teaches a model the directive forms
(like llx-markdown's `answerFormatPrompt`).

## Use

```go
r, err := bubbleteandmd.NewRenderer(styles.DarkStyleConfig, width) // or a raw *glamour.TermRenderer
cache := bubbleteandmd.NewCache(512)                              // one per program
s := bubbleteandmd.NewStream(r, fmt.Sprintf("dark/%d", width), cache) // one per reply
out, err := s.Frame(reply) // each time the reply grows
out, err = s.Final(reply)  // once, when the reply ends
```

- One Stream per reply. When the width or style changes, make a new
  Renderer and a new Stream with a key that names both.
- Call Final when the stream ends and show its output. Frame hides a
  partial table row and other half-written syntax, so the last Frame of a
  reply that ends in one is not the reply.
- Pass the Renderer raw. Process the source before Frame and the output
  after; a wrapper that trims glamour's newlines gives `ErrTrimmed`.
- `r.WideBlocks(pane)` (before the first Render): a top-level code block
  or table wider than `width` is not wrapped; it renders at its own width
  and each line is cut at `pane` with `… +N cols →`. Prose still wraps at
  `width`. Put `pane` in the Stream key.
- Interactive forms: `d := bubbleteandmd.NewDoc(r, key, cache)` in place
  of NewStream; `d.Frame`/`d.Final` as above. The layout stays as streamed
  until the user acts. `d.Focus()` gives the Doc the keys;
  `d.HandleKey(msg, bubbleteandmd.DefaultKeyMap())` returns false for a key
  the host handles (↑/↓ at height 0, tab past the last form, esc). Render
  again after a true; `d.Stops()` and `d.Focused()` give the lines to
  scroll to. On a width change make a new Renderer and Doc and copy the
  state: `*nd.State() = *old.State()` (form keys are byte offsets, the
  same at every width).
- Viewport: `d.SetHeight(h)` (h > 0) makes the Doc own the scroll.
  Frame/Final/`d.View()` return only lines `[d.Offset(), d.Offset()+h)`;
  ↑/↓, pgup/pgdown, home/end scroll (consumed, also at an edge and with no
  focus; show `d.View()`, no render needed); a focus change scrolls the
  focused stop into view; a view at the bottom follows the reply as it
  grows, a view scrolled up stays put. The view opens at the top when the
  first output it shows is taller than h, else at the bottom. Height 0 (default): all lines, the
  scroll keys return false. Stop lines count in the whole output.
- On an error, show the raw text (after `Sanitize`). A panic in chroma
  while it highlights a code block is an error too (Render and Stream
  return "" and the error).
- Call `Sanitize` on model text you render outside Stream.

### Lex timeout (opt-in)

chroma gives each lexer rule a 250 ms match timeout on a wall clock. On a
loaded machine a match can time out on valid code, and one character then
draws in the Error style (a red background). To stop this, call once at
program start, before any rendering:

```go
bubbleteandmd.DisableLexTimeout()
```

Trade-off: the setting is for the whole process. It also removes the
guard against runaway matches of chroma and of every other user of
regexp2 in the process. The library never calls it.

## Rules

- Terminal controls in the source never reach the terminal: Stream calls
  `Sanitize` first; call it yourself for text you render outside Stream.
- Healed text never reaches the stored source.
- Joined blocks must equal glamour's whole-document spacing: the final frame
  must not move when the stream ends (checked by a test that compares
  streamed and whole renders at every chunk boundary).
