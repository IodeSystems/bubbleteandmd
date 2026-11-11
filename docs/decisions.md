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
