# Decisions

- 2026-11-10 (Carl): a separate repo, not a package inside dun, so other
  bubbletea programs can use it and dun pins it like its other dependencies.
- 2026-11-10 (Carl): keep llx-markdown's directive syntax unchanged, so one
  model output renders in both.
- 2026-11-10 (Carl, D3): one root package `bubbleteandmd` holds the public
  API; splitter, healer, join and lru stay in internal/.
- 2026-11-10: layer-1 API is `Renderer`, `NewCache`, `NewStream(r, key, c)`,
  `Stream.Frame`, `Stream.Final`. The cache key is (key, block source, block
  is first): a block at offset 0 renders without the join sentinel, so the
  same source renders differently there. Only blocks before the splitter's
  last two are cached; the tail is rendered each call.
- 2026-11-10: Frame heals the tail only (`heal.Close(src[tailOffset:])`),
  not the whole text. The invariant test checks, at every prefix, that this
  equals `heal.Close(src)` with the stable blocks unchanged, so healed text
  never reaches a stable block or a cache key.
- 2026-11-10: join keeps glamour's empty styled run (`ESC[..m ESC[m`) when
  it drops the cells the previous block left on an open line. Found by the
  invariant: a code block followed by a heading or `---` (light and dark
  styles) lost 14 bytes. No D1 merge was needed; every element kind joins
  byte-equal on the corpus, scenarios and 300 fuzz docs.
- 2026-11-10: glamour's whole render is not deterministic in code blocks.
  chroma v2.14.0 `findClosest` ranges over a map, so a colour equally far
  from two 256-colour entries gets either, per code block, per render (dark
  Comment #676767 -> 241|242, light Operator #FF2626 -> 9|196). glamour
  v2.0.1 also registers its chroma theme once per process, from the first
  style that renders a code block. The invariant maps those two pairs to one
  code on both sides before comparing. A cached block keeps one choice, so
  Stream output flickers less than whole renders do.
- 2026-11-10: `Sanitize` removes terminal controls from the source before
  Stream splits, heals or caches it. Why: model text can carry them, and
  glamour v2.0.1 passes them through (verified: OSC 52 clipboard write and
  `\r` in prose, SGR and OSC 0 in code fences, C1 CSI U+009B and raw 0x9B,
  `\b`, BEL in prose and table cells); the lipgloss directive renderers
  read the same text. Removed: C0 controls but tab and newline (`\r\n`
  becomes `\n`, any other `\r` goes), DEL, C1 U+0080-U+009F, bytes that
  are not UTF-8, and bidi controls U+202A-U+202E and U+2066-U+2069 (they
  reorder the text shown: spoofing). ZWJ and emoji stay. Only the control
  byte goes: the text of an escape sequence after ESC stays visible.
  Prefix-stable rule: Sanitize(s[:k]) is a prefix of Sanitize(s) for every
  cut k, so an incomplete UTF-8 sequence at the end is held back, not
  dropped as invalid; stable blocks and cache keys do not change as the
  text grows (fuzz test). The CRLF corpus scenario now renders with `\n`
  line ends. A caller that renders model text outside Stream (glamour
  directly, its own lipgloss) must call Sanitize first. glamour's own
  OSC 8 link sequences (ended by BEL) stay; their URL is sanitized text.
- 2026-11-10: Stream checks its Renderer at setup: a probe paragraph must
  end in a newline, else every call returns `ErrTrimmed`. Why: a wrapper
  that trims glamour's newlines gave empty affixes and wrong output with no
  error. Pre- and post-processing go on src and on output, not in the
  Renderer.
- 2026-11-10: heal reads a fence's indent as up to three spaces (CommonMark,
  blocks.Split), not the TS `\s{0,3}`. Why: `` \t``` `` is indented code; the
  TS closed it as a fence, so the frame showed a fence the final text lacks.
  heal's other CRLF limits stay as in the TS; Stream turns `\r\n` into
  `\n` first. Still open: heal's inline fixes apply in indented code (a
  `` \t``` `` line gets a closing backtick while it streams).
- 2026-11-11: heal closes a fence at the opener's indent, not column 0 as
  the TS does. Why: in a list item a column-0 closer ends the item and opens
  an empty fence, one row more than the real closer draws, so the frame
  shrank when the closing line arrived.
- 2026-11-11 (N4): the tail is the last block, not the last two. The block
  before the last is memoized per Stream by (source, first), not put in the
  shared Cache (the splitter can still reopen it: `[!CONS]` joins `[!PROS]`;
  TestReopenedBlockNotStale); a changed source is a miss. It goes to the
  Cache from the memo when it is stable. Frame heals the last block alone,
  except when heal of the last two blocks is not their text + heal of the
  last (heal drops a callout marker line with the `\n` before it): then the
  last two render together, as before.
- 2026-11-11 (N5): Stream keeps Sanitize of the previous src up to its last
  `\n`; a src that extends it scans only the new text. Why exact:
  Sanitize(a+b) == Sanitize(a)+Sanitize(b) when a ends in `\n`
  (FuzzSanitizeSplit).
- 2026-11-11 (perf): DirectiveRenderer rewrites the language of each top-level
  fence to `text` when it is empty or chroma has no lexer for it
  (fencelang.go; per-name cache, one `lexers.Get` per name per process).
  Why: glamour v2.0.1 calls `quick.Highlight`: `lexers.Get` (a miss scans
  every lexer's file globs, 8-18 ms per fence per render, also for an empty
  name), then `lexers.Analyse` (guesses from content), then the plaintext
  fallback. `text` is a registered alias: a map hit. Change: such fences no
  longer get content-guessed colours (Carl accepted); lines and text are the
  same (tested). Other info-string parts (`title="..."`) stay; `math`,
  `mermaid` become `text`; `tree` and titled code are read as forms first,
  and a titled fence renders through the same path. Only a block that
  starts with a fence (blocks.Split) is rewritten, on its opening line: a
  first line-scan version broke TestRendererPlain (fuzz-1; a fence closer in
  a list read as an opener). Fences nested in lists
  or quotes (not top-level) keep the slow path. Raw `*glamour.TermRenderer`
  users do not get this.
- 2026-11-11 (deps): chroma v2.27.0, goldmark v1.8.7, lipgloss v2.1.0,
  x/ansi v0.11.9; glamour stays v2.0.1 (Carl). Changes: chroma closes each
  escape before a line end (codetitle goldens, SGR only); lipgloss v2.1.0
  stops shrinking a column at a floor (frame + narrowest cell) first, and
  internal/tablestream's width model ports it (TestWidthModel; on the old
  model such tables fell back to whole renders, bytes stayed exact).
- 2026-11-11 (perf): NewRenderer's glamour renderers (outer, inner) use
  chroma formatter `bubbleteandmd-terminal256` (chromafmt.go): stock
  `terminal256` with each style's escape table built once per style
  pointer, learned from one stock Format call. `terminal256` is not
  replaced. Registered with sync.Once at the first NewRenderer; chroma's
  formatter registry is a map with no lock, so that call must come before
  other goroutines render code. caller opts come after it and can name
  another formatter. Stock is not deterministic on a palette tie (chroma
  v2.27.0 still has it; 20 of 75 chroma styles, glamour dark Comment
  241|242); the cached table keeps the first choice for the process.
  Tests: byte-equal for every style with no tie on 108 code samples;
  for tied styles every cached escape is one stock can write; glamour
  renders of the corpus equal in every glamour style with tied escapes
  read as one.
- 2026-11-11 (fix): `bubbleteandmd-terminal256` breaks palette ties by the
  lowest palette index, in every process (Carl). Supersedes the "keeps the
  first choice for the process" line above. The table is built in
  chromafmt.go from its own xterm 256 palette (not learned from one stock
  call): nearest by chroma `Colour.Distance`, ascending index, strict `<`.
  A colour listed twice in the palette (#ff0000: 9 and 196) is one
  candidate with the later index, as in stock's map, so a style with no tie
  writes stock's bytes. Tests: palette equals chroma's table; the 20 tied
  styles (of 75) give the lowest-index table across 50 fresh builds; the
  rest are byte-equal to stock. Stream/Renderer invariants with NewRenderer
  compare without `untie`; `untie` stays where raw glamour (stock
  terminal256) is the reference.
- 2026-11-11 (table hold, Carl): while Frame streams, a GFM table that is the
  last block and has fewer than 8 body rows (`holdRows`, counted on the
  healed tail, so a partial row is not counted) draws as one muted line
  `table…` (DirectiveRenderer: Muted style in the glamour paragraph frame;
  any other Renderer: a plain paragraph). It becomes the table when it has 8
  rows, when a later block starts, or at Final. Reason: column widths settle
  in the first rows, so the table appears once and does not reflow (no screen
  bounce); the line count grows only downward. Final is unchanged. Frame ==
  the whole render with the tail block replaced by the placeholder piece
  (invariant tests; `stats.held` counts held tails and the test fails if a
  held tail is not a table of < 8 rows, or a such table is not held).
  Measured (table-20k, w78/w98, dark): total table width at first
  appearance equals final (76, 96); column 1 is 2 cells narrower at row 8
  than final (9 vs 11), so the column rule can still change once. The frame
  never shrinks after the table shows.
- 2026-11-11 (L3): form state is `directive.State`, by FormID.Key; the zero
  State draws the streamed layout byte for byte (goldens of testdata/directives
  made before the change). Tabbed tabs: one `‹i/n› Label ───` line and the
  selected panel's body; folded details: the `▸ Title` lines only; a tree with
  a State entry marks folders `▾`/`▸` and hides rows under a closed one, a
  tree with no entry keeps today's bytes. Focus is reverse video over the
  part's own style (dark, light). notty (Carl 2026-11-11): no SGR, and the
  focused header changes its own glyph in place, same width so nothing
  moves: details `▸`→`▶`, `▾`→`▼`; tree folder (with fold marks) `▸`→`▶`,
  `▾`→`▼`; tabbed header `‹…›`→`«…»`; tree row with no fold mark (file, or
  any row of a tree with no State entry) at depth ≥ 1: guide `──`→`━━`.
  No cue: a depth-0 row with no fold mark, and the stacked tabs label list
  (not Tabbed). The host also reads State.Focused. Marks (header, panel, row) come from the form renderers'
  own line counts and are shifted as Frame (leading blank), join (count of
  "\n") and each body trim (leading "\n" in tabs and verdict, leading blank
  lines in steps, After's blank line) place the lines. Forms in a body take
  keys `<form>/<slot>@<offset>` as IDs gives them; text that is not a body
  (titles, stat values, attribution) renders with no state and no marks.
- 2026-11-11 (Carl + dun-bf, decision C): dun's splitWide moves into
  DirectiveRenderer as `WideBlocks(pane)` (off by default). Why: dun
  rendered wide code and tables outside Stream, so the no-move invariant
  and the fast paths did not cover them. A top-level fenced code block or
  table (one block, no form, no `$$`) wider than the word wrap width
  renders through a glamour renderer at its own width (code: widest line
  + code margin + document margins, rounded up to 20 cells; table: the
  tablestream width model's natural width + margins, exact, since glamour
  stretches a table to its wrap width), capped at 512 cells. Each line is
  then cut: one that fits the word wrap width keeps glamour's padding to
  it (same bytes as a plain render), a wider one loses its trailing blanks,
  one wider than the pane is cut there with dun's note `… +N cols →`
  (Theme.Muted). The piece keeps glamour's frame, so blocks join as in a
  whole render. Differences from dun: no sideways offset (layer 3 can add
  it), no blank-row stripping and "\n\n" joins (glamour's own spacing),
  "wide" measured against the word wrap width minus nothing (dun: the
  prose column), code width measured from the source as dun does, tables
  from glamour's own cell widths, a cap. Fast paths: a wide table tail
  grows from cached rows through the wide renderer (Stream keys its Table
  by wrap width); a wide code tail goes through codestream with the
  DirectiveRenderer (each chunk decides wide alone; a cut line does not
  depend on the render width, so chunks agree, SGR-only rule as before).
- 2026-11-11 (L5): `Doc` wraps a Stream (`NewDoc(r, key, cache)`, same
  arguments as NewStream) and owns its State; it is not a tea.Model (Carl).
  Why a wrapper, not a Doc that takes Marks: one object per reply, as
  dun keeps one Stream per reply today; the host cannot render without
  updating the stops or forget SetState; Stream stays usable alone.
  `Do(Action) bool` / `HandleKey(tea.KeyPressMsg, KeyMap) bool` return
  "consumed". With no focus nothing is consumed: the host calls `Focus`
  (or `FocusLast`) to give the Doc the keys, so a host that offers every
  key to the Doc first still cycles its own zones. Leaving the Doc
  (ActionNext past the last stop, ActionPrev before the first,
  ActionLeave/esc) blurs it and returns false (dun's insideWideKeys rule).
  Stops: one per drawn interactive form, from the marks of the last output
  (Mark gained Panels and Folder for this), in line order; focus is by
  form key, so a stop keeps focus as the reply grows; forms hidden in a
  folded details or another tab are not stops. Keys (DefaultKeyMap): tab /
  shift+tab, enter and space toggle, ←/→, esc; ↑/↓ unbound (host scroll).
  Tabs: toggle switches stacked ↔ one-tab; ←/→ from stacked switch to
  one-tab at the selected panel (first press does not move), then step,
  wrapping. Tree (one stop): ←/→ move the cursor over the drawn folder
  rows, wrapping; toggle opens/closes the folder at the cursor, not
  consumed on a file row. A tree gets its State entry (fold marks, which
  shift folder names 2 cells) on its first ←/→/toggle, not on focus. No
  NormalizeKey: dun's folds shift+enter / SS3 M into ctrl+j (composer
  newline), which means nothing to a Doc, and a real keypad enter arrives
  as enter.
- 2026-11-11 (L5b, Carl): `Doc.SetHeight(h)`. h > 0: the Doc owns the
  scroll; Frame, Final and `View()` return lines [Offset, Offset+h) of the
  whole output (a substring, so fewer lines when the output is shorter;
  the host pads). h = 0 (default): today's behaviour, the scroll actions
  return false and the host scrolls with Stops. Actions scroll.up/down
  (↑/↓), page.up/down (pgup/pgdown, h lines), scroll.top/bottom
  (home/end) in DefaultKeyMap. Follow: at each render a view that was at
  the bottom (Offset = Lines-h, or the output fits) goes to the new
  bottom; any other view keeps its Offset, so nothing in it moves as the
  reply grows below; a shrink clamps. Focus change (Focus, FocusLast,
  focus.next/prev) scrolls the focused stop into view at the next render,
  the least that shows it; a stop taller than h shows its top line. A tree
  cursor move (←/→) scrolls the cursor row into view. Toggle and tab switch
  do not scroll (clamp only). Consumed: with h > 0 a scroll action is
  consumed with or without focus (a Doc with no forms has no focus and
  must still scroll), and at the top/bottom edge too: the vertical axis is
  the Doc's in a viewport, and leaving is tab/shift+tab/esc. dun's
  insideWideKeys lets ↑/↓ leave and fall through because there the axis it
  owns is horizontal; its ← at the left edge leaves but is consumed. A
  scroll changes View with no render. Stop and Mark lines stay counted in
  the whole output. SetHeight keeps a view at the bottom at the bottom;
  from h = 0 the view starts at the top. Width is the host's (new Renderer
  and Doc). Reset keeps height and Offset.
