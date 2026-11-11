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
