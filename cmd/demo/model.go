package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd"
)

const (
	left  = 0
	right = 1
	// chrome: a title line, two status lines per pane and the help line.
	chrome = 4
	// tickEvery is how often the stream releases tokens.
	tickEvery = 16 * time.Millisecond
)

type config struct {
	style     ansi.StyleConfig
	styleName string
	width     int // per pane; 0: half the terminal
	rate      float64
	wide      bool
	src       string
	seed      uint32
}

type tickMsg time.Time

// renderedMsg is the result of one pane render.
type renderedMsg struct {
	pane, gen, tok int
	step           int // the stream step tok was released at
	final          bool
	view           string
	lines          int
	err            error
	took           time.Duration
}

type pane struct {
	title string
	eng   engine
	busy  bool // a render is in flight: eng must not be touched
	// rebuild: a new engine waits for the render in flight; keep: copy the
	// Doc State into it (a resize, not a restart).
	rebuild, keep bool
	gen           int  // results of an older gen are dropped
	dirty         bool // render again with no new tokens (a Doc key)
	shown         int  // tokens in the last output
	shownStep     int  // the stream step of the last output
	final         bool // the last output is Final's
	view          string
	err           error
	st            stats
}

type model struct {
	cfg    config
	ends   []int
	pos    int     // tokens released
	step   int     // ticks that released tokens (a frame a pane can draw)
	acc    float64 // part of a token released
	rate   float64
	paused bool
	last   time.Time
	w, h   int // terminal
	pw, ph int // pane width and body height
	panes  [2]*pane
	cache  *bubbleteandmd.Cache
	km     bubbleteandmd.KeyMap
	keys   []tea.KeyPressMsg // for the Doc, held while it renders
	// focused: Focus has been called (the first tab).
	focused bool
	// tick schedules the next tick (tests: nil, they send ticks).
	tick func() tea.Cmd
}

func newModel(cfg config) *model {
	km := bubbleteandmd.DefaultKeyMap()
	km[bubbleteandmd.ActionToggle] = []string{"enter"} // space pauses the stream
	return &model{
		cfg:   cfg,
		ends:  tokens(cfg.src, cfg.seed),
		rate:  cfg.rate,
		cache: bubbleteandmd.NewCache(512),
		km:    km,
		panes: [2]*pane{
			{title: "bubbleteandmd: Doc, heal + block cache"},
			{title: "naive: glamour, whole reply per token"},
		},
		tick: func() tea.Cmd {
			return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
		},
	}
}

func (m *model) Init() tea.Cmd { return m.tick() }

func (m *model) doc() *docEngine {
	e, _ := m.panes[left].eng.(*docEngine)
	return e
}

// build makes pane i's engine for the current size; it must not be busy.
func (m *model) build(i int, keep bool) {
	p := m.panes[i]
	p.rebuild, p.dirty, p.err = false, true, nil
	var err error
	if i == left {
		var old *bubbleteandmd.Doc
		if e := m.doc(); keep && e != nil {
			old = e.d
		}
		p.eng, err = newDocEngine(m.cfg.style, m.cfg.styleName, m.pw, m.ph, m.cfg.wide, m.cache, old)
	} else {
		p.eng, err = newNaiveEngine(m.cfg.style, m.pw, m.ph)
	}
	if err != nil {
		p.eng, p.err = nil, err
	}
}

// requestBuild builds pane i now, or after its render in flight.
func (m *model) requestBuild(i int, keep bool) {
	p := m.panes[i]
	if p.busy {
		p.keep = (p.keep || !p.rebuild) && keep
		p.rebuild = true
		return
	}
	m.build(i, keep)
}

func (m *model) restart() {
	m.pos, m.step, m.acc, m.focused, m.keys = 0, 0, 0, false, nil
	for i, p := range m.panes {
		p.gen++
		p.shown, p.shownStep, p.final, p.view, p.st = 0, 0, false, "", stats{}
		if m.w > 0 {
			m.requestBuild(i, false)
		}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		pw := m.cfg.width
		if pw <= 0 {
			pw = (m.w - 1) / 2
		}
		pw, ph := max(pw, 10), max(m.h-chrome, 1)
		if pw != m.pw || ph != m.ph {
			m.pw, m.ph = pw, ph
			for i := range m.panes {
				m.requestBuild(i, true)
			}
		}
		return m, m.kick()
	case tickMsg:
		t := time.Time(msg)
		if !m.last.IsZero() && !m.paused && m.pos < len(m.ends) {
			m.acc += m.rate * t.Sub(m.last).Seconds()
			n := int(m.acc)
			m.acc -= float64(n)
			if n > 0 {
				m.pos = min(m.pos+n, len(m.ends))
				m.step++
			}
		}
		m.last = t
		return m, tea.Batch(m.kick(), m.tick())
	case renderedMsg:
		m.done(msg)
		return m, m.kick()
	case tea.KeyPressMsg:
		return m, m.key(msg)
	}
	return m, nil
}

func (m *model) done(msg renderedMsg) {
	p := m.panes[msg.pane]
	p.busy = false
	if msg.gen == p.gen {
		s := &p.st
		if msg.tok > p.shown && msg.lines < s.lines {
			s.shrank++
		}
		if msg.step > p.shownStep+1 {
			s.dropped += msg.step - p.shownStep - 1
		}
		s.frames++
		s.lines = msg.lines
		s.last = msg.took
		s.total += msg.took
		s.max = max(s.max, msg.took)
		p.shown, p.shownStep, p.final, p.view, p.err = msg.tok, msg.step, msg.final, msg.view, msg.err
	}
	if p.rebuild {
		m.build(msg.pane, p.keep)
	}
	if msg.pane == left && !p.busy {
		keys := m.keys
		m.keys = nil
		for _, k := range keys {
			m.docKey(k)
		}
	}
}

// kick starts a render in each idle pane that is behind the stream.
func (m *model) kick() tea.Cmd {
	if m.w == 0 {
		return nil
	}
	var cmds []tea.Cmd
	for i, p := range m.panes {
		end := m.pos == len(m.ends)
		if p.busy || p.eng == nil || !(p.dirty || p.shown < m.pos || end && !p.final) {
			continue
		}
		p.busy, p.dirty = true, false
		tok, step, gen, eng := m.pos, m.step, p.gen, p.eng
		src := ""
		if tok > 0 {
			src = m.cfg.src[:m.ends[tok-1]]
		}
		cmds = append(cmds, func() tea.Msg {
			t0 := time.Now()
			view, lines, err := eng.render(src, end)
			return renderedMsg{pane: i, gen: gen, tok: tok, step: step, final: end, view: view, lines: lines, err: err, took: time.Since(t0)}
		})
	}
	return tea.Batch(cmds...)
}

func (m *model) key(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "space":
		m.paused = !m.paused
		return nil
	case "r":
		m.restart()
		return m.kick()
	case "+", "=":
		m.rate *= 1.5
		return nil
	case "-":
		m.rate = max(m.rate/1.5, 1)
		return nil
	}
	if p := m.panes[left]; p.busy || p.rebuild {
		m.keys = append(m.keys, k)
		return nil
	}
	m.docKey(k)
	return m.kick()
}

// docKey gives k to the Doc; the left pane is idle.
func (m *model) docKey(k tea.KeyPressMsg) {
	e, p := m.doc(), m.panes[left]
	if e == nil {
		return
	}
	if !m.focused && k.String() == "tab" {
		m.focused = e.d.Focus()
		p.dirty = m.focused
		return
	}
	a, ok := m.km.Action(k.String())
	if !ok {
		return
	}
	consumed := e.d.HandleKey(k, m.km)
	switch a {
	case bubbleteandmd.ActionScrollUp, bubbleteandmd.ActionScrollDown, bubbleteandmd.ActionPageUp,
		bubbleteandmd.ActionPageDown, bubbleteandmd.ActionTop, bubbleteandmd.ActionBottom:
		if consumed {
			p.view = e.d.View() // a scroll needs no render
		}
	default:
		// a focus move, toggle or tab switch shows on the next render; so
		// does a blur (tab past the last form, esc), which is not consumed
		p.dirty = true
	}
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
	dimStyle   = lipgloss.NewStyle().Faint(true)
)

// fit cuts s to w cells and pads it to w.
func fit(s string, w int) string {
	s = xansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-xansi.StringWidth(s), 0))
}

func (m *model) column(p *pane) []string {
	rows := []string{titleStyle.Render(fit(" "+p.title, m.pw))}
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
	rows = append(rows,
		fit(fmt.Sprintf(" last %.1f ms · avg %.1f · max %.1f", ms(s.last), ms(s.avg()), ms(s.max)), m.pw),
		fit(fmt.Sprintf(" frames %d · dropped %d · lag %d tok · shrank %d×", s.frames, s.dropped, m.pos-p.shown, s.shrank), m.pw),
	)
	return rows
}

func (m *model) help() string {
	state := fmt.Sprintf("%d/%d tok · %.0f tok/s", m.pos, len(m.ends), m.rate)
	if m.paused {
		state += " · paused"
	} else if m.pos == len(m.ends) {
		state += " · done"
	}
	km := m.km
	a := func(x bubbleteandmd.Action) string { return km.Label(x) }
	keys := fmt.Sprintf("q quit · space pause · r restart · +/- rate · %s/%s forms · %s toggle · %s/%s tabs · %s/%s scroll · %s/%s page",
		a(bubbleteandmd.ActionNext), a(bubbleteandmd.ActionPrev), a(bubbleteandmd.ActionToggle),
		a(bubbleteandmd.ActionLeft), a(bubbleteandmd.ActionRight), a(bubbleteandmd.ActionScrollUp),
		a(bubbleteandmd.ActionScrollDown), a(bubbleteandmd.ActionPageUp), a(bubbleteandmd.ActionPageDown))
	return dimStyle.Render(fit(" "+state+" │ "+keys, m.w))
}

func (m *model) content() string {
	if m.w == 0 {
		return "starting…"
	}
	l, r := m.column(m.panes[left]), m.column(m.panes[right])
	var b strings.Builder
	for i := range l {
		b.WriteString(l[i] + "│" + r[i] + "\n")
	}
	b.WriteString(m.help())
	return b.String()
}

func (m *model) View() tea.View {
	v := tea.NewView(m.content())
	v.AltScreen = true
	return v
}
