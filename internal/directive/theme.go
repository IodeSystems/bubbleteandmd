package directive

import (
	"image/color"
	"strings"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
)

// Theme is what the form renderers draw with. It comes from a glamour style,
// so forms match the Markdown around them. Every lipgloss.Style here is the
// zero style when Plain is set: Render returns its input unchanged, no SGR.
type Theme struct {
	Plain bool // notty: no colour, no bold, no italics at all

	Text    lipgloss.Style
	Heading lipgloss.Style
	Link    lipgloss.Style
	Code    lipgloss.Style
	Muted   lipgloss.Style // dim: hints, hosts, `when` text

	// Quote is the blockquote indent token, e.g. "│ " or "| ". Never empty.
	Quote string

	// Callout is keyed by lower-case callout kind (CalloutKinds). Each style is
	// the callout hue, bold.
	Callout map[string]lipgloss.Style

	// Frame is the piece frame glamour puts around a paragraph.
	Frame Style
}

// PlainStyle tells notty from coloured: a glamour style is notty (styles.NoTTYStyleConfig,
// which is the ASCII style) when its document, heading and link carry no colour.
// Callers may also pass plain explicitly to ThemeFrom.
func PlainStyle(s ansi.StyleConfig) bool {
	return s.Document.Color == nil && s.Heading.Color == nil && s.Link.Color == nil && s.Text.Color == nil
}

// Callout hues are GitHub's alert colours as 256-colour indexes: dark, light.
var calloutHue = map[string][2]uint8{
	"note":      {75, 25},   // blue
	"tip":       {77, 28},   // green
	"important": {141, 91},  // purple
	"warning":   {178, 130}, // yellow
	"caution":   {203, 124}, // red
}

// ThemeFrom derives a Theme from a glamour style. plain selects notty.
// Dark or light is read from the style (isDarkStyle).
func ThemeFrom(s ansi.StyleConfig, plain bool) Theme {
	th := Theme{Plain: plain, Quote: "│ ", Callout: map[string]lipgloss.Style{}}
	if s.BlockQuote.IndentToken != nil && *s.BlockQuote.IndentToken != "" {
		th.Quote = *s.BlockQuote.IndentToken
	}
	th.Frame = StyleOf(s, plain)
	if plain {
		for k := range calloutHue {
			th.Callout[k] = lipgloss.NewStyle()
		}
		return th
	}
	th.Text = prim(s.Text)
	th.Heading = prim(s.Heading.StylePrimitive).Bold(true)
	th.Link = prim(s.Link).Underline(true)
	th.Code = prim(s.Code.StylePrimitive)
	th.Muted = lipgloss.NewStyle().Faint(true)
	if c := s.Text.Color; c != nil {
		th.Muted = th.Muted.Foreground(lipgloss.Color(*c))
	}
	idx := 0
	if isDarkStyle(s) {
		idx = 0
	} else {
		idx = 1
	}
	for k, h := range calloutHue {
		th.Callout[k] = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(h[idx]))
	}
	return th
}

// isDarkStyle: dark styles use a light document foreground: index 7, 9..15
// (8 is bright black, grey) or 244 and above, or hex brighter than mid grey.
func isDarkStyle(s ansi.StyleConfig) bool {
	c := s.Document.Color
	if c == nil {
		c = s.Text.Color
	}
	if c == nil {
		return true
	}
	n, ok := atoiSmall(*c)
	if !ok {
		return !strings.HasPrefix(*c, "#") || hexLight(*c)
	}
	return n >= 244 || n == 7 || (n >= 9 && n <= 15)
}

func hexLight(h string) bool {
	col, ok := lipgloss.Color(h).(color.RGBA)
	if !ok {
		return true
	}
	return int(col.R)+int(col.G)+int(col.B) > 384
}

func atoiSmall(s string) (int, bool) {
	if s == "" || len(s) > 3 {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

func prim(p ansi.StylePrimitive) lipgloss.Style {
	st := lipgloss.NewStyle()
	if p.Color != nil {
		st = st.Foreground(lipgloss.Color(*p.Color))
	}
	if p.Bold != nil && *p.Bold {
		st = st.Bold(true)
	}
	if p.Italic != nil && *p.Italic {
		st = st.Italic(true)
	}
	if p.Underline != nil && *p.Underline {
		st = st.Underline(true)
	}
	if p.Faint != nil && *p.Faint {
		st = st.Faint(true)
	}
	return st
}
