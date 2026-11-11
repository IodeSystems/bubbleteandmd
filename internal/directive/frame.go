package directive

import (
	"strings"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// Style is the piece frame of one glamour style: what glamour wraps around
// each paragraph line. Measured in internal/spike/frame_spike_test.go.
type Style struct {
	Margin string // left margin, plain spaces (Document.Margin)
	Cell   string // one right-pad cell: a styled space, or " " in notty
}

// StyleOf derives the frame Style from a glamour style. The pad cell is a
// space in the document colour, written as glamour writes it (ansi/margin.go
// renderText: x/ansi Style, lipgloss.Color): "0".."15" as basic 3x/9x
// codes, other indexes as 38;5;N, hex (short or long) as 38;2;r;g;b. A bare
// " " when plain or when the document has no colour.
func StyleOf(s ansi.StyleConfig, plain bool) Style {
	st := Style{Cell: " "}
	if s.Document.Margin != nil {
		st.Margin = strings.Repeat(" ", int(*s.Document.Margin))
	}
	if plain || s.Document.Color == nil {
		return st
	}
	st.Cell = xansi.Style{}.ForegroundColor(lipgloss.Color(*s.Document.Color)).Styled(" ")
	return st
}

// Frame wraps content lines (each at most width cells, no margin, no pad) in
// the frame glamour gives a paragraph piece: margin, line, pad cells up to
// width, newline. width is the content width (line width minus margin). A
// piece that is not first starts with one blank line, as glamour's paragraph
// spacing does. A width below 0 is 0.
func Frame(lines []string, width int, s Style, first bool) string {
	width = max(width, 0)
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
