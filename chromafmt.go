package bubbleteandmd

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
)

// chromaFormatter is the chroma formatter name NewRenderer gives glamour
// (glamour.WithChromaFormatter). It writes the bytes of chroma's stock
// "terminal256" formatter; "terminal256" itself is not replaced.
const chromaFormatter = "bubbleteandmd-terminal256"

var registerFormatter sync.Once

// registerChromaFormatter registers the cached formatter under
// chromaFormatter, once per process, at the first NewRenderer. chroma's
// formatters.Registry is a plain map with no lock: a goroutine that reads it
// (formatters.Get, as glamour does at each code block render) while another
// calls NewRenderer for the first time is a data race. Make the first
// NewRenderer before rendering on other goroutines.
func registerChromaFormatter() {
	registerFormatter.Do(func() {
		formatters.Register(chromaFormatter, newCachedFormatter(formatters.Get("terminal256")))
	})
}

// cachedFormatter writes what stock (chroma's indexed TTY formatter) writes,
// but builds the escape sequence table of a style once, not at every Format
// call. Why: the stock formatter converts every style entry to a palette
// index (Lab distance over the table) per call, which is most of the time of
// a small code block (~5× in the scratchpad prototype fmt4).
//
// Tables are learned from stock: one Format call per style over one token
// of each type. They are keyed by the style pointer; chroma styles do not
// change once built, and glamour gets them from chroma's styles registry, so
// the number of tables is the number of styles used.
type cachedFormatter struct {
	stock chroma.Formatter
	mu    sync.Mutex
	tabs  map[*chroma.Style]map[chroma.TokenType]string
}

func newCachedFormatter(stock chroma.Formatter) *cachedFormatter {
	return &cachedFormatter{stock: stock, tabs: map[*chroma.Style]map[chroma.TokenType]string{}}
}

// probe is the token text used to learn a table; no escape sequence has it.
const probe = "|"

func (c *cachedFormatter) table(style *chroma.Style) map[chroma.TokenType]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.tabs[style]; ok {
		return t
	}
	// stock adds a Background entry to every style (clearBackground).
	types := append(style.Types(), chroma.Background)
	toks := make([]chroma.Token, len(types))
	for i, tt := range types {
		toks[i] = chroma.Token{Type: tt, Value: probe}
	}
	var b bytes.Buffer
	_ = c.stock.Format(&b, style, chroma.Literator(toks...))
	out := b.String()
	t := make(map[chroma.TokenType]string, len(types))
	for _, tt := range types {
		i := strings.Index(out, probe)
		pre := out[:i]
		out = out[i+len(probe):]
		if pre != "" {
			out = strings.TrimPrefix(out, "\x1b[0m")
		}
		t[tt] = pre
	}
	c.tabs[style] = t
	return t
}

// Format is chroma's indexedTTYFormatter.Format with the table cached.
func (c *cachedFormatter) Format(w io.Writer, style *chroma.Style, it chroma.Iterator) error {
	theme := c.table(style)
	for token := it(); token != chroma.EOF; token = it() {
		clr, ok := theme[token.Type]
		if !ok {
			clr, ok = theme[token.Type.SubCategory()]
			if !ok {
				clr, ok = theme[token.Type.Category()]
				if !ok {
					clr, ok = theme[chroma.Text]
					if !ok {
						clr = theme[chroma.Background]
					}
				}
			}
		}
		writeToken(w, clr, token.Value)
	}
	return nil
}

var crOrCrLf = regexp.MustCompile(`\r?\n`)

// writeToken is chroma's formatters.writeToken (v2.27.0): the escape is
// closed before each line end and opened again after it.
func writeToken(w io.Writer, f, text string) {
	if f == "" {
		_, _ = io.WriteString(w, text)
		return
	}
	last := 0
	for _, ix := range crOrCrLf.FindAllStringIndex(text, -1) {
		_, _ = io.WriteString(w, f+text[last:ix[0]]+"\x1b[0m"+text[ix[0]:ix[1]])
		last = ix[1]
	}
	if last < len(text) {
		_, _ = io.WriteString(w, f+text[last:]+"\x1b[0m")
	}
}
