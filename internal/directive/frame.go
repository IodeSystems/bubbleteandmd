package directive

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/glamour/v2/ansi"
	xansi "github.com/charmbracelet/x/ansi"
)

// Style is the piece frame of one glamour style: what glamour wraps around
// each paragraph line. Measured in internal/spike/frame_spike_test.go.
type Style struct {
	Margin string // left margin, plain spaces (Document.Margin)
	Cell   string // one right-pad cell: a styled space, or " " in notty
}

// StyleOf derives the frame Style from a glamour style. The pad cell is a
// space in the document colour: `ESC[38;5;Nm ESC[m` for an index, `38;2;r;g;b`
// for hex, a bare " " when plain or when the document has no colour.
func StyleOf(s ansi.StyleConfig, plain bool) Style {
	st := Style{Cell: " "}
	if s.Document.Margin != nil {
		st.Margin = strings.Repeat(" ", int(*s.Document.Margin))
	}
	if plain || s.Document.Color == nil {
		return st
	}
	c := *s.Document.Color
	if n, ok := atoiSmall(c); ok {
		st.Cell = "\x1b[38;5;" + strconv.Itoa(n) + "m \x1b[m"
	} else if len(c) == 7 && c[0] == '#' {
		var r, g, b uint8
		if _, err := fmt.Sscanf(c[1:], "%02x%02x%02x", &r, &g, &b); err == nil {
			st.Cell = fmt.Sprintf("\x1b[38;2;%d;%d;%dm \x1b[m", r, g, b)
		}
	}
	return st
}

// Frame wraps content lines (each at most width cells, no margin, no pad) in
// the frame glamour gives a paragraph piece: margin, line, pad cells up to
// width, newline. width is the content width (line width minus margin). A
// piece that is not first starts with one blank line, as glamour's paragraph
// spacing does.
func Frame(lines []string, width int, s Style, first bool) string {
	var b strings.Builder
	blank := s.Margin + strings.Repeat(s.Cell, width) + "\n"
	if !first {
		b.WriteString(blank)
	}
	for _, l := range lines {
		b.WriteString(s.Margin)
		b.WriteString(l)
		if w := xansi.StringWidth(l); w < width {
			b.WriteString(strings.Repeat(s.Cell, width-w))
		}
		b.WriteString("\n")
	}
	return b.String()
}
