package bubbleteandmd

import (
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/join"
)

// plainLang is the language a fence gets when chroma does not know its own.
// chroma registers its plaintext lexer under this alias (map lookup, cheap).
const plainLang = "text"

// langCache remembers, per language name, whether chroma knows it.
// chroma's lexers.Get scans every lexer's file-name globs on a miss
// (8-18 ms measured), and glamour calls it for every fence at every render,
// also with an empty name.
type langCache struct {
	mu     sync.Mutex
	known  map[string]bool
	lookup func(name string) bool
}

func newLangCache() *langCache {
	return &langCache{known: map[string]bool{}, lookup: func(name string) bool { return lexers.Get(name) != nil }}
}

// langs is the process-wide cache: chroma's registry is process-wide too.
var langs = newLangCache()

func (c *langCache) knows(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	k, ok := c.known[name]
	if !ok {
		k = c.lookup(name)
		c.known[name] = k
	}
	return k
}

// plainUnknownLangs rewrites the language of each top-level fenced code
// block in md to `text` when it is empty or chroma has no lexer for it. The
// rest of the info string (title="..." and so on) stays.
//
// Why: for a miss, glamour v2.0.1 (ansi/codeblock.go) calls chroma's
// quick.Highlight, which runs lexers.Get (the slow glob scan), then
// lexers.Analyse (guesses a lexer from the content), then the plaintext
// fallback. With `text` the first lookup hits at once and the other two
// never run. Behaviour change: a fence with no or an unknown language no
// longer gets colours guessed from its content; it renders in the code
// block's plain colour. Frame and lines are the same. Raw
// *glamour.TermRenderer users do not get this; only DirectiveRenderer.
//
// Only a top-level block that starts with a fence (as blocks.Split cuts
// them) is rewritten, on its opening line. Fences in lists or quotes,
// indented code, and fences inside other fences are not touched: a line
// scan cannot tell a closing fence of a nested block from an opening one.
func (c *langCache) plainUnknownLangs(md string) string {
	if !strings.Contains(md, "```") && !strings.Contains(md, "~~~") {
		return md
	}
	pre := ""
	if strings.HasPrefix(md, join.Sentinel) {
		pre, md = join.Sentinel, md[len(join.Sentinel):]
	}
	var b strings.Builder
	last := 0
	for _, blk := range blocks.Split(md) {
		first := blk.Source
		if i := strings.IndexByte(first, '\n'); i >= 0 {
			first = first[:i+1]
		}
		text := strings.TrimRight(first, "\r\n")
		m := codeFenceRe.FindStringSubmatch(text)
		if m == nil || (m[1][0] == '`' && strings.Contains(text[len(m[0]):], "`")) {
			continue
		}
		trimmed := strings.TrimLeft(text[len(m[0]):], " \t")
		name, tail := trimmed, ""
		if i := strings.IndexAny(trimmed, " \t"); i >= 0 {
			name, tail = trimmed[:i], trimmed[i:]
		}
		if name != "" && c.knows(name) {
			continue
		}
		b.WriteString(md[last:blk.Offset])
		b.WriteString(m[0] + plainLang + tail + first[len(text):])
		last = blk.Offset + len(first)
	}
	if last == 0 {
		return pre + md
	}
	b.WriteString(md[last:])
	return pre + b.String()
}
