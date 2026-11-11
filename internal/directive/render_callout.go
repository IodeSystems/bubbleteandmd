package directive

import (
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

const calloutBar = "▌ "

// calloutBold is bold text, or plain text in notty.
func calloutBold(th Theme, s string) string {
	if th.Plain || s == "" {
		return s
	}
	return lipgloss.NewStyle().Bold(true).Render(s)
}

// calloutBoldStyle is the style calloutBold draws with.
func calloutBoldStyle(th Theme) lipgloss.Style {
	if th.Plain {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Bold(true)
}

// calloutInner renders md at width and splits it into lines, dropping the
// trailing blank lines. Each line is clipped to width. slot names a form
// body (Ctx.Body); "" renders through Inner, with no marks.
func calloutInner(c Ctx, md string, width int, slot string) ([]string, []Mark, error) {
	if strings.TrimSpace(md) == "" {
		return nil, nil, nil
	}
	var out string
	var marks []Mark
	var err error
	if slot == "" {
		out, err = c.Inner(md, width)
	} else {
		out, marks, err = c.body(md, width, slot)
	}
	if err != nil {
		return nil, nil, err
	}
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil, nil, nil
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = calloutClip(l, width)
	}
	return lines, place(marks, 0, len(lines), 0), nil
}

func calloutClip(l string, width int) string {
	if xansi.StringWidth(l) > width {
		return xansi.Truncate(l, width, "")
	}
	return l
}

// calloutWrap wraps plain text to width cells, hard-breaking long words.
func calloutWrap(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	return strings.Split(xansi.Wrap(s, width, ""), "\n")
}

func calloutPrefix(prefix string, lines []string, width int) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = calloutClip(prefix+l, width)
	}
	return out
}

func renderCallout(d Directive, c Ctx) ([]string, []Mark, error) {
	co := d.Callout
	if co == nil {
		return nil, nil, errNotImplemented
	}
	th := c.Theme
	hue := th.Callout[co.Kind]
	bar := calloutBar
	if th.Plain {
		bar = th.Quote
	}
	barW := xansi.StringWidth(bar)
	inner := c.Width - barW
	if inner < 1 {
		inner = 1
	}
	styledBar := hue.Render(strings.TrimRight(bar, " ")) + bar[len(strings.TrimRight(bar, " ")):]
	if th.Plain {
		styledBar = bar
	}

	label := strings.ToUpper(co.Kind)
	text := label
	if co.Title != "" {
		text += " " + co.Title
	}
	var lines []string
	remain := len(label)
	for _, l := range calloutWrap(text, inner) {
		n := remain
		if n > len(l) {
			n = len(l)
		}
		remain -= n
		lines = append(lines, hue.Render(l[:n])+calloutBold(th, l[n:]))
	}
	body, marks, err := calloutInner(c, co.Body, inner, "b")
	if err != nil {
		return nil, nil, err
	}
	marks = place(marks, 0, len(body), len(lines))
	lines = append(lines, body...)
	return calloutPrefix(styledBar, lines, c.Width), marks, nil
}

// renderDetails draws "▾ Title" over the body, or "▸ Title" alone when
// folded (State).
func renderDetails(d Directive, c Ctx) ([]string, []Mark, error) {
	de := d.Details
	if de == nil {
		return nil, nil, errNotImplemented
	}
	title := de.Title
	if title == "" {
		title = "Details"
	}
	folded := c.State.details(c.Key).Folded
	focused := c.State.focused(c.Key)
	head, indent := c.Theme.foldGlyph("▾", "▼", focused&&c.Theme.Plain)+" ", "  "
	if folded {
		head = c.Theme.foldGlyph("▸", "▶", focused&&c.Theme.Plain) + " "
	}
	inner := c.Width - 2
	if inner < 1 {
		inner = 1
	}
	var lines []string
	for i, l := range calloutWrap(title, inner) {
		p := indent
		if i == 0 {
			p = head
		}
		t := calloutBold(c.Theme, l)
		if focused {
			t = c.Theme.focus(calloutBoldStyle(c.Theme), true).Render(l)
		}
		lines = append(lines, calloutClip(p+t, c.Width))
	}
	marks := []Mark{{Key: c.Key, Kind: MarkHeader, Line: 0, Height: len(lines)}}
	if folded {
		return lines, marks, nil
	}
	body, bm, err := calloutInner(c, de.Body, inner, "b")
	if err != nil {
		return nil, nil, err
	}
	marks = append(marks, place(bm, 0, len(body), len(lines))...)
	return append(lines, calloutPrefix(indent, body, c.Width)...), marks, nil
}

func renderAttribution(d Directive, c Ctx) ([]string, error) {
	at := d.Attribution
	if at == nil {
		return nil, errNotImplemented
	}
	th := c.Theme
	q := th.Quote
	inner := c.Width - xansi.StringWidth(q)
	if inner < 1 {
		inner = 1
	}
	pq := th.Muted.Render(q)
	lines, _, err := calloutInner(c, at.Quote, inner, "")
	if err != nil {
		return nil, err
	}
	out := calloutPrefix(pq, lines, c.Width)
	src, _, err := calloutInner(c, "— "+at.Source, inner, "")
	if err != nil {
		return nil, err
	}
	for i, l := range src {
		if !th.Plain {
			l = lipgloss.NewStyle().Italic(true).Render(l)
		}
		src[i] = l
	}
	return append(out, calloutPrefix(pq, src, c.Width)...), nil
}
