package directive

import (
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// verdictSideBySide is the least content width at which pros and cons sit
// side by side (Carl 2026-11-10).
const verdictSideBySide = 64

// tabsFit cuts s to w cells, with an ellipsis when it cut.
func tabsFit(s string, w int) string {
	if xansi.StringWidth(s) <= w {
		return s
	}
	return xansi.Truncate(s, w, "…")
}

// tabsInner renders md at width w through c.Inner and returns its lines,
// without leading or trailing blank lines.
func tabsInner(c Ctx, md string, w int) ([]string, error) {
	if w < 1 {
		w = 1
	}
	if strings.TrimSpace(md) == "" {
		return nil, nil
	}
	out, err := c.Inner(md, w)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.Trim(out, "\n"), "\n")
	for i, l := range lines {
		if xansi.StringWidth(l) > c.Width {
			lines[i] = xansi.Truncate(l, c.Width, "")
		}
	}
	return lines, nil
}

// renderTabs draws the panels stacked: a list of labels, then per panel a
// "▸ Label ───" header and the body.
func renderTabs(d Directive, c Ctx) ([]string, error) {
	t := d.Tabs
	if t == nil || len(t.Panels) == 0 {
		return nil, nil
	}
	w := c.Width
	labels := make([]string, len(t.Panels))
	for i, p := range t.Panels {
		labels[i] = p.Label
	}
	lines := []string{c.Theme.Muted.Render(tabsFit(strings.Join(labels, " · "), w))}
	for _, p := range t.Panels {
		lines = append(lines, "")
		head := tabsFit("▸ "+p.Label, w)
		hw := xansi.StringWidth(head)
		line := c.Theme.Heading.Render(head)
		if hw < w {
			// one space before the rule when it fits
			if hw+1 < w {
				line += " "
				hw++
			}
			line += c.Theme.Muted.Render(strings.Repeat("─", w-hw))
		}
		lines = append(lines, line)
		body, err := tabsInner(c, p.Body, w)
		if err != nil {
			return nil, err
		}
		lines = append(lines, body...)
	}
	return lines, nil
}

// verdictColumn is a header and bullets, each line at most w cells.
func verdictColumn(c Ctx, w int, st lipgloss.Style, sign, title string, items []Item) ([]string, error) {
	lines := []string{st.Render(tabsFit(sign+" "+title, w))}
	bw := w - 2
	if bw < 1 {
		bw = 1
	}
	for _, it := range items {
		body, err := tabsInner(Ctx{Width: bw, Theme: c.Theme, Inner: c.Inner}, it.Body, bw)
		if err != nil {
			return nil, err
		}
		if len(body) == 0 {
			continue
		}
		for i, l := range body {
			if w < 3 {
				// no room for a bullet gutter
				lines = append(lines, tabsFit(l, w))
				continue
			}
			if i == 0 {
				lines = append(lines, st.Render("•")+" "+l)
			} else {
				lines = append(lines, "  "+l)
			}
		}
	}
	return lines, nil
}

// renderVerdict draws pros and cons, side by side from 64 cells, else stacked.
func renderVerdict(d Directive, c Ctx) ([]string, error) {
	v := d.Verdict
	if v == nil {
		return nil, nil
	}
	pt, ct := v.ProsTitle, v.ConsTitle
	if pt == "" {
		pt = "Pros"
	}
	if ct == "" {
		ct = "Cons"
	}
	pst, cst := c.Theme.Callout["tip"], c.Theme.Callout["caution"]
	hasP, hasC := v.Pros != nil, v.Cons != nil
	if !hasP && !hasC {
		return nil, nil
	}
	if hasP && hasC && c.Width >= verdictSideBySide {
		cw := (c.Width - 4) / 2
		p, err := verdictColumn(c, cw, pst, "+", pt, v.Pros)
		if err != nil {
			return nil, err
		}
		k, err := verdictColumn(c, cw, cst, "−", ct, v.Cons)
		if err != nil {
			return nil, err
		}
		left, right := p, k
		if v.ConsFirst {
			left, right = k, p
		}
		pad := func(ls []string) string {
			out := make([]string, len(ls))
			for i, l := range ls {
				out[i] = l + strings.Repeat(" ", cw-xansi.StringWidth(l))
			}
			return strings.Join(out, "\n")
		}
		joined := lipgloss.JoinHorizontal(lipgloss.Top, pad(left), strings.Repeat(" ", 4), pad(right))
		rows := strings.Split(joined, "\n")
		for i, r := range rows {
			rows[i] = strings.TrimRight(r, " ")
		}
		return rows, nil
	}
	var p, k []string
	var err error
	if hasP {
		if p, err = verdictColumn(c, c.Width, pst, "+", pt, v.Pros); err != nil {
			return nil, err
		}
	}
	if hasC {
		if k, err = verdictColumn(c, c.Width, cst, "−", ct, v.Cons); err != nil {
			return nil, err
		}
	}
	first, second := p, k
	if v.ConsFirst {
		first, second = k, p
	}
	lines := first
	if len(first) > 0 && len(second) > 0 {
		lines = append(lines, "")
	}
	return append(lines, second...), nil
}
