package directive

import (
	"strconv"
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

// tabsInner renders md at width w through c.Body under slot and returns
// its lines, without leading or trailing blank lines, and its marks.
func tabsInner(c Ctx, md string, w int, slot string) ([]string, []Mark, error) {
	if w < 1 {
		w = 1
	}
	if strings.TrimSpace(md) == "" {
		return nil, nil, nil
	}
	out, marks, err := c.body(md, w, slot)
	if err != nil {
		return nil, nil, err
	}
	lead := len(out) - len(strings.TrimLeft(out, "\n"))
	lines := strings.Split(strings.Trim(out, "\n"), "\n")
	for i, l := range lines {
		if xansi.StringWidth(l) > c.Width {
			lines[i] = xansi.Truncate(l, c.Width, "")
		}
	}
	return lines, place(marks, lead, len(lines), 0), nil
}

// tabsRule is head in the heading style, then a muted rule to w cells.
func tabsRule(c Ctx, head string, w int, focused bool) string {
	hw := xansi.StringWidth(head)
	line := c.Theme.focus(c.Theme.Heading, focused).Render(head)
	if hw < w {
		// one space before the rule when it fits
		if hw+1 < w {
			line += " "
			hw++
		}
		line += c.Theme.Muted.Render(strings.Repeat("─", w-hw))
	}
	return line
}

// renderTabs draws the panels stacked: a list of labels, then per panel a
// "▸ Label ───" header and the body. Tabbed (State): a "‹i/n› Label ───"
// header and the selected panel's body only.
func renderTabs(d Directive, c Ctx) ([]string, []Mark, error) {
	t := d.Tabs
	if t == nil || len(t.Panels) == 0 {
		return nil, nil, nil
	}
	w := c.Width
	focused := c.State.focused(c.Key)
	if st := c.State.tabs(c.Key); st.Tabbed {
		i := min(max(st.Selected, 0), len(t.Panels)-1)
		p := t.Panels[i]
		head := tabsFit("‹"+strconv.Itoa(i+1)+"/"+strconv.Itoa(len(t.Panels))+"› "+p.Label, w)
		lines := []string{tabsRule(c, head, w, focused)}
		body, bm, err := tabsInner(c, p.Body, w, "t"+strconv.Itoa(i))
		if err != nil {
			return nil, nil, err
		}
		lines = append(lines, body...)
		marks := []Mark{
			{Key: c.Key, Kind: MarkHeader, Line: 0, Height: 1, Index: i},
			{Key: c.Key, Kind: MarkPanel, Line: 0, Height: len(lines), Index: i},
		}
		return lines, append(marks, place(bm, 0, len(body), 1)...), nil
	}
	labels := make([]string, len(t.Panels))
	for i, p := range t.Panels {
		labels[i] = p.Label
	}
	lines := []string{c.Theme.focus(c.Theme.Muted, focused).Render(tabsFit(strings.Join(labels, " · "), w))}
	marks := []Mark{{Key: c.Key, Kind: MarkHeader, Line: 0, Height: 1}}
	for i, p := range t.Panels {
		lines = append(lines, "")
		at := len(lines)
		lines = append(lines, tabsRule(c, tabsFit("▸ "+p.Label, w), w, false))
		body, bm, err := tabsInner(c, p.Body, w, "t"+strconv.Itoa(i))
		if err != nil {
			return nil, nil, err
		}
		marks = append(marks, Mark{Key: c.Key, Kind: MarkPanel, Line: at, Height: 1 + len(body), Index: i})
		marks = append(marks, place(bm, 0, len(body), len(lines))...)
		lines = append(lines, body...)
	}
	return lines, marks, nil
}

// verdictColumn is a header and bullets, each line at most w cells.
// slot is the item slot letter ("p" pros, "c" cons).
func verdictColumn(c Ctx, w int, st lipgloss.Style, sign, title, slot string, items []Item) ([]string, []Mark, error) {
	lines := []string{st.Render(tabsFit(sign+" "+title, w))}
	bw := w - 2
	if bw < 1 {
		bw = 1
	}
	var marks []Mark
	ic := c
	ic.Width = bw
	for n, it := range items {
		body, bm, err := tabsInner(ic, it.Body, bw, slot+strconv.Itoa(n))
		if err != nil {
			return nil, nil, err
		}
		if len(body) == 0 {
			continue
		}
		marks = append(marks, place(bm, 0, len(body), len(lines))...)
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
	return lines, marks, nil
}

// renderVerdict draws pros and cons, side by side from 64 cells, else stacked.
func renderVerdict(d Directive, c Ctx) ([]string, []Mark, error) {
	v := d.Verdict
	if v == nil {
		return nil, nil, nil
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
		return nil, nil, nil
	}
	if hasP && hasC && c.Width >= verdictSideBySide {
		cw := (c.Width - 4) / 2
		p, pm, err := verdictColumn(c, cw, pst, "+", pt, "p", v.Pros)
		if err != nil {
			return nil, nil, err
		}
		k, km, err := verdictColumn(c, cw, cst, "−", ct, "c", v.Cons)
		if err != nil {
			return nil, nil, err
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
		// top aligned: each column's marks keep their rows
		marks := append(pm, km...)
		if v.ConsFirst {
			marks = append(km, pm...)
		}
		return rows, marks, nil
	}
	var p, k []string
	var pm, km []Mark
	var err error
	if hasP {
		if p, pm, err = verdictColumn(c, c.Width, pst, "+", pt, "p", v.Pros); err != nil {
			return nil, nil, err
		}
	}
	if hasC {
		if k, km, err = verdictColumn(c, c.Width, cst, "−", ct, "c", v.Cons); err != nil {
			return nil, nil, err
		}
	}
	first, second := p, k
	fm, sm := pm, km
	if v.ConsFirst {
		first, second = k, p
		fm, sm = km, pm
	}
	lines := first
	if len(first) > 0 && len(second) > 0 {
		lines = append(lines, "")
	}
	marks := append(fm, place(sm, 0, len(second), len(lines))...)
	return append(lines, second...), marks, nil
}
