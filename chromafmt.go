package bubbleteandmd

import (
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
)

// chromaFormatter is the chroma formatter name NewRenderer gives glamour
// (glamour.WithChromaFormatter). It writes the bytes of chroma's stock
// "terminal256" formatter, except on a palette tie (lowest index);
// "terminal256" itself is not replaced.
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
		formatters.Register(chromaFormatter, newCachedFormatter())
	})
}

// cachedFormatter writes the bytes of chroma's stock "terminal256" formatter
// (indexedTTYFormatter), but builds the escape sequence table of a style once,
// not at every Format call, and breaks colour ties the same way every time.
// Why cached: the stock formatter converts every style entry to a palette
// index (Lab distance over the table) per call, which is most of the time of
// a small code block (~5x in the scratchpad prototype fmt4).
// Why tie-break: stock ranges over a map, so a style colour equally far from
// two palette colours gets either, per call (20 of 75 chroma styles; glamour
// dark Comment #676767 is 241 or 242). Here the lowest palette index wins,
// in every process (Carl). Colours that appear twice in the palette (#ff0000
// is 9 and 196) are one candidate with the later index, as in stock's map,
// so a style without a tie gets stock's bytes.
//
// Tables are keyed by the style pointer; chroma styles do not change once
// built, and glamour gets them from chroma's styles registry, so the number
// of tables is the number of styles used.
type cachedFormatter struct {
	mu   sync.Mutex
	tabs map[*chroma.Style]map[chroma.TokenType]string
}

func newCachedFormatter() *cachedFormatter {
	return &cachedFormatter{tabs: map[*chroma.Style]map[chroma.TokenType]string{}}
}

// palette is the xterm 256 palette in index order (chroma ttyTables[256]).
var palette = func() [256]chroma.Colour {
	var p [256]chroma.Colour
	ansi := [16]chroma.Colour{
		0x000000, 0x800000, 0x008000, 0x808000, 0x000080, 0x800080, 0x008080, 0xc0c0c0,
		0x808080, 0xff0000, 0x00ff00, 0xffff00, 0x0000ff, 0xff00ff, 0x00ffff, 0xffffff,
	}
	for i, c := range ansi {
		p[i] = chroma.NewColour(uint8(c>>16), uint8(c>>8), uint8(c))
	}
	level := [6]uint8{0, 0x5f, 0x87, 0xaf, 0xd7, 0xff}
	for i := 0; i < 216; i++ {
		p[16+i] = chroma.NewColour(level[i/36], level[i/6%6], level[i%6])
	}
	for i := 0; i < 24; i++ {
		g := uint8(8 + 10*i)
		p[232+i] = chroma.NewColour(g, g, g)
	}
	return p
}()

// candidates: palette indexes in ascending order, without an index whose
// colour appears again at a later index (stock's map keeps the later).
var candidates = func() []int {
	last := map[chroma.Colour]int{}
	for i, c := range palette {
		last[c] = i
	}
	var out []int
	for i, c := range palette {
		if last[c] == i {
			out = append(out, i)
		}
	}
	return out
}()

// nearest is the palette index nearest to c; on equal distance the lowest.
func nearest(c chroma.Colour) int {
	best, bestD := 0, math.MaxFloat64
	for _, i := range candidates {
		if d := palette[i].Distance(c); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// escape is chroma's entryToEscapeSequence for the 256 palette.
func escape(e chroma.StyleEntry) string {
	var b strings.Builder
	if e.Bold == chroma.Yes {
		b.WriteString("\x1b[1m")
	}
	if e.Underline == chroma.Yes {
		b.WriteString("\x1b[4m")
	}
	if e.Italic == chroma.Yes {
		b.WriteString("\x1b[3m")
	}
	if e.Colour.IsSet() {
		b.WriteString("\x1b[38;5;" + strconv.Itoa(nearest(e.Colour)) + "m")
	}
	if e.Background.IsSet() {
		b.WriteString("\x1b[48;5;" + strconv.Itoa(nearest(e.Background)) + "m")
	}
	return b.String()
}

func (c *cachedFormatter) table(style *chroma.Style) map[chroma.TokenType]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.tabs[style]; ok {
		return t
	}
	// stock clears the background colour (clearBackground) and keeps the
	// Background entry without inheritance.
	bld := style.Builder()
	bg := bld.Get(chroma.Background)
	bg.Background, bg.NoInherit = 0, true
	bld.AddEntry(chroma.Background, bg)
	cleared, _ := bld.Build()
	t := map[chroma.TokenType]string{}
	for _, tt := range cleared.Types() {
		t[tt] = escape(cleared.Get(tt))
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
