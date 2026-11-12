package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
	flashStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("9"))
	dimStyle   = lipgloss.NewStyle().Faint(true)
	boldStyle  = lipgloss.NewStyle().Bold(true)
)

// fit cuts s to w cells and pads it to w.
func fit(s string, w int) string {
	s = xansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-xansi.StringWidth(s), 0))
}

func secs(d time.Duration) string { return fmt.Sprintf("%.1f s", d.Seconds()) }

// lag is how far pane p is behind the stream, in tokens and in time
// since the first token it has not drawn was released.
func (m *model) lag(p *pane) (int, time.Duration) {
	n := m.pos - p.shown
	if n <= 0 || m.relAt[p.shown].IsZero() {
		return max(n, 0), 0
	}
	return n, m.clock().Sub(m.relAt[p.shown])
}

// toFinal is the time from the stream's start to p's Final output, and
// from the last token to it; "" before the Final output.
func (m *model) toFinal(p *pane) string {
	if p.st.finalAt.IsZero() || m.began.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s (+%s after the last token)", secs(p.st.finalAt.Sub(m.began)), secs(p.st.finalAt.Sub(m.ended)))
}

// spark draws byPos in w cells, each the longest frame in its part of the
// reply, on a 0..scale bar.
func spark(byPos *[buckets]time.Duration, w int, scale time.Duration) string {
	const bars = "▁▂▃▄▅▆▇█"
	levels := []rune(bars)
	var b strings.Builder
	for c := range w {
		var v time.Duration
		for i := c * buckets / w; i < max((c+1)*buckets/w, c*buckets/w+1); i++ {
			v = max(v, byPos[i])
		}
		if v == 0 || scale == 0 {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(levels[min(int(float64(v)/float64(scale)*float64(len(levels))), len(levels)-1)])
	}
	return b.String()
}

func (m *model) scale() time.Duration {
	var s time.Duration
	for _, p := range m.panes {
		for _, v := range p.st.byPos {
			s = max(s, v)
		}
	}
	return s
}

func (m *model) title(i int, p *pane) string {
	t := " " + p.title
	switch {
	case i == left && p.st.unsafe > 0:
		t += fmt.Sprintf(" · sanitized %d B", p.st.unsafe)
	case i == right && p.st.unsafe > 0:
		t += fmt.Sprintf(" · %d controls shown as ␛ (not sent)", p.st.unsafe)
	}
	if m.clock().Before(p.st.flash) {
		f := fmt.Sprintf(" ▼ shrank %d line(s) ", p.st.cut)
		return titleStyle.Render(fit(t, max(m.pw-xansi.StringWidth(f), 0))) + flashStyle.Render(f)
	}
	return titleStyle.Render(fit(t, m.pw))
}

func (m *model) column(i int, p *pane, scale time.Duration) []string {
	rows := []string{m.title(i, p)}
	body := strings.Split(strings.TrimSuffix(p.view, "\n"), "\n")
	if p.err != nil {
		body = []string{"error: " + p.err.Error()}
	}
	for i := range m.ph {
		l := ""
		if i < len(body) {
			l = body[i]
		}
		rows = append(rows, fit(l, m.pw)+"\x1b[m")
	}
	s := p.st
	inFlight := time.Time{}
	if p.busy {
		inFlight = p.busyAt
	}
	lagTok, lagT := m.lag(p)
	final := m.toFinal(p)
	if final == "" {
		final = "—"
	}
	label := fmt.Sprintf(" ms/frame by reply position, 0–%.0f ms ", ms(scale))
	rows = append(rows,
		fit(fmt.Sprintf(" frame %.1f ms · avg %.1f · max %.1f · %d fps · total %.0f ms · busy %3.0f%% of a core",
			ms(s.last), ms(s.avg()), ms(s.max), s.fps(m.clock()), ms(s.total), 100*s.busy(m.clock(), inFlight)), m.pw),
		fit(fmt.Sprintf(" frames %d · dropped %d · lag %d tok / %s (max %.0f ms) · shrank %d× · final %s",
			s.frames, s.dropped, lagTok, secs(lagT), ms(s.age), s.shrank, final), m.pw),
		fit(dimStyle.Render(label)+spark(&s.byPos, max(m.pw-xansi.StringWidth(label), 1), scale), m.pw),
	)
	return rows
}

// tail is the end of the source the panes show, controls as pictures and
// newlines as ⏎.
func (m *model) tail(n int) string {
	if m.pos == 0 {
		return ""
	}
	s, _ := visible(m.scen.src[:m.ends[m.pos-1]])
	r := []rune(strings.ReplaceAll(s, "\n", "⏎"))
	return string(r[max(len(r)-n, 0):])
}

func (m *model) header() string {
	el := time.Duration(0)
	if !m.began.IsZero() {
		el = m.clock().Sub(m.began)
	}
	h := fmt.Sprintf(" %s: %s · %d/%d tok · %d kB · %.0f tok/s · %s", m.scen.name, m.scen.about, m.pos, len(m.ends),
		m.ends[max(m.pos-1, 0)]/1000, m.rate, secs(el))
	switch {
	case m.frozen:
		state := "naive still drawing"
		if m.panes[left].shown == m.pos && m.panes[right].shown == m.pos {
			state = "both panes show it"
		}
		h += fmt.Sprintf(" · FROZEN at tok %d (%s), source ends «%s»", m.pos, state, m.tail(40))
	case m.paused:
		h += " · paused"
	case m.pos == len(m.ends):
		h += " · stream done"
	}
	return boldStyle.Render(fit(h, m.w))
}

func (m *model) help() string {
	a := func(x bubbleteandmd.Action) string { return m.km.Label(x) }
	stream := " stream: q quit · space pause · p freeze · n one token · s summary · c next scenario · r restart · +/- rate"
	forms := fmt.Sprintf(" forms, left pane only (glamour shows llx forms as raw quotes): %s/%s focus · %s toggle · %s/%s tabs · %s/%s/%s/%s scroll",
		a(bubbleteandmd.ActionNext), a(bubbleteandmd.ActionPrev), a(bubbleteandmd.ActionToggle),
		a(bubbleteandmd.ActionLeft), a(bubbleteandmd.ActionRight), a(bubbleteandmd.ActionScrollUp),
		a(bubbleteandmd.ActionScrollDown), a(bubbleteandmd.ActionPageUp), a(bubbleteandmd.ActionPageDown))
	return dimStyle.Render(fit(stream, m.w)) + "\n" + dimStyle.Render(fit(forms, m.w))
}

// summaryRows is the summary screen, n rows of m.w cells.
func (m *model) summaryRows(n int) []string {
	l, r := m.panes[left], m.panes[right]
	row := func(name string, f func(p *pane) string) string {
		return fmt.Sprintf("   %-34s %22s %22s", name, f(l), f(r))
	}
	num := func(f func(s stats) int) func(p *pane) string {
		return func(p *pane) string { return fmt.Sprint(f(p.st)) }
	}
	msf := func(f func(s stats) time.Duration) func(p *pane) string {
		return func(p *pane) string { return fmt.Sprintf("%.1f", ms(f(p.st))) }
	}
	fin := func(after bool) func(p *pane) string {
		return func(p *pane) string {
			if p.st.finalAt.IsZero() {
				return "not yet"
			}
			if after {
				return secs(p.st.finalAt.Sub(m.ended))
			}
			return secs(p.st.finalAt.Sub(m.began))
		}
	}
	state := "stream done"
	if m.pos < len(m.ends) {
		state = fmt.Sprintf("stream at %d/%d tok", m.pos, len(m.ends))
	}
	rows := []string{
		"",
		boldStyle.Render(fmt.Sprintf("   Summary · %s · %s · %d bytes · %.0f tok/s", m.scen.name, state, len(m.scen.src), m.rate)),
		"",
		boldStyle.Render(fmt.Sprintf("   %-34s %22s %22s", "", "bubbleteandmd (Doc)", "naive glamour")),
		row("render time, total (ms)", msf(func(s stats) time.Duration { return s.total })),
		row("longest frame (ms)", msf(func(s stats) time.Duration { return s.max })),
		row("average frame (ms)", msf(func(s stats) time.Duration { return s.avg() })),
		row("token to screen, longest (ms)", msf(func(s stats) time.Duration { return s.age })),
		row("frames drawn", num(func(s stats) int { return s.frames })),
		row("stream steps not drawn (dropped)", num(func(s stats) int { return s.dropped })),
		row("height shrinks (bounce)", num(func(s stats) int { return s.shrank })),
		row("time to final output", fin(false)),
		row("final output after the last token", fin(true)),
		row("controls removed / shown as ␛", num(func(s stats) int { return s.unsafe })),
		"",
		dimStyle.Render("   Render time is wall time on the render goroutine, one render in flight per pane."),
		dimStyle.Render("   s back to the panes · c next scenario · r restart · q quit"),
	}
	out := make([]string, n)
	for i := range out {
		if i < len(rows) {
			out[i] = fit(rows[i], m.w)
		} else {
			out[i] = fit("", m.w)
		}
	}
	return out
}

func (m *model) content() string {
	if m.w == 0 {
		return "starting…"
	}
	var b strings.Builder
	b.WriteString(m.header() + "\n")
	if m.summary {
		for _, r := range m.summaryRows(m.ph + 4) {
			b.WriteString(r + "\n")
		}
	} else {
		scale := m.scale()
		l, r := m.column(left, m.panes[left], scale), m.column(right, m.panes[right], scale)
		for i := range l {
			b.WriteString(l[i] + "│" + r[i] + "\n")
		}
	}
	b.WriteString(m.help())
	return b.String()
}

func (m *model) View() tea.View {
	v := tea.NewView(m.content())
	v.AltScreen = true
	return v
}
