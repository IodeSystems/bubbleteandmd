package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2/ansi"
	"github.com/iodesystems/bubbleteandmd"
)

const (
	left  = 0
	right = 1
	// chrome: the header, per pane a title and three status lines, and
	// two help lines.
	chrome = 7
	// tickEvery is how often the stream releases tokens.
	tickEvery = 16 * time.Millisecond
	// flashFor is how long a pane title shows a shrink.
	flashFor = 800 * time.Millisecond
)

type config struct {
	style     ansi.StyleConfig
	styleName string
	width     int // per pane; 0: half the terminal
	rate      float64
	wide      bool
	scen      scenario
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
	end            time.Time // m.clock when the render returned
	unsafe         int       // see stats.unsafe
}

type pane struct {
	title string
	eng   engine
	busy  bool // a render is in flight: eng must not be touched
	// busyAt: when the render in flight started.
	busyAt time.Time
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
	scen   scenario
	ends   []int
	first  int         // tokens on the page before the stream (thread)
	relAt  []time.Time // when each token was released
	pos    int         // tokens released
	step   int         // ticks that released tokens (a frame a pane can draw)
	acc    float64     // part of a token released
	rate   float64
	paused bool
	// frozen: the stream holds (as paused) and the header shows the source
	// tail; n releases one token.
	frozen bool
	// began: the first tick of the stream; ended: the last token's release.
	began, ended time.Time
	// summary: the summary screen is up; summed: it came up on its own once.
	summary, summed bool
	last            time.Time
	w, h            int // terminal
	pw, ph          int // pane width and body height
	panes           [2]*pane
	cache           *bubbleteandmd.Cache
	km              bubbleteandmd.KeyMap
	keys            []tea.KeyPressMsg // for the Doc, held while it renders
	// focused: Focus has been called (the first tab).
	focused bool
	// tick schedules the next tick (tests: nil, they send ticks).
	tick func() tea.Cmd
	// clock is the time (tests: the last tick's).
	clock func() time.Time
}

func newModel(cfg config) *model {
	km := bubbleteandmd.DefaultKeyMap()
	km[bubbleteandmd.ActionToggle] = []string{"enter"} // space pauses the stream
	m := &model{
		cfg:   cfg,
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
		clock: time.Now,
	}
	m.load(cfg.scen)
	m.restart()
	return m
}

// load sets the reply to stream; restart starts it.
func (m *model) load(s scenario) {
	m.scen = s
	m.ends, m.first = nil, 0
	if s.start > 0 {
		m.ends, m.first = []int{s.start}, 1
	}
	for _, e := range tokens(s.src[s.start:], m.cfg.seed) {
		m.ends = append(m.ends, s.start+e)
	}
	m.relAt = make([]time.Time, len(m.ends))
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
	m.pos, m.step, m.acc, m.focused, m.keys = m.first, 0, 0, false, nil
	m.began, m.ended, m.summary, m.summed = time.Time{}, time.Time{}, false, false
	clear(m.relAt)
	for i, p := range m.panes {
		p.gen++
		p.shown, p.shownStep, p.final, p.view, p.st = 0, 0, false, "", stats{}
		if m.w > 0 {
			m.requestBuild(i, false)
		}
	}
}

// release adds n tokens at time t.
func (m *model) release(n int, t time.Time) {
	n = min(n, len(m.ends)-m.pos)
	if n <= 0 {
		return
	}
	for i := m.pos; i < m.pos+n; i++ {
		m.relAt[i] = t
	}
	m.pos += n
	m.step++
	if m.pos == len(m.ends) {
		m.ended = t
	}
}

func (m *model) halted() bool { return m.paused || m.frozen }

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
		if m.began.IsZero() {
			m.began = t
			for i := range m.pos {
				m.relAt[i] = t // thread: the page before the stream
			}
		}
		if !m.last.IsZero() && !m.halted() && m.pos < len(m.ends) {
			m.acc += m.rate * t.Sub(m.last).Seconds()
			n := int(m.acc)
			m.acc -= float64(n)
			m.release(n, t)
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
			s.cut = s.lines - msg.lines
			s.flash = msg.end.Add(flashFor)
		}
		if msg.tok > p.shown && !m.relAt[p.shown].IsZero() {
			s.age = max(s.age, msg.end.Sub(m.relAt[p.shown]))
		}
		if msg.step > p.shownStep+1 {
			s.dropped += msg.step - p.shownStep - 1
		}
		s.frames++
		s.lines = msg.lines
		s.last = msg.took
		s.total += msg.took
		s.max = max(s.max, msg.took)
		s.unsafe = msg.unsafe
		if b := min(msg.tok*buckets/max(len(m.ends), 1), buckets-1); msg.took > s.byPos[b] {
			s.byPos[b] = msg.took
		}
		s.recent = append(s.recent, span{msg.end.Add(-msg.took), msg.end})
		for len(s.recent) > 0 && s.recent[0].to.Before(msg.end.Add(-busyWindow)) {
			s.recent = s.recent[1:]
		}
		if msg.final && !p.final {
			s.finalAt = msg.end
		}
		p.shown, p.shownStep, p.final, p.view, p.err = msg.tok, msg.step, msg.final, msg.view, msg.err
		if !m.summed && m.panes[left].final && m.panes[right].final {
			m.summary, m.summed = true, true
		}
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
		p.busy, p.dirty, p.busyAt = true, false, m.clock()
		tok, step, gen, eng, clock := m.pos, m.step, p.gen, p.eng, m.clock
		src := ""
		if tok > 0 {
			src = m.scen.src[:m.ends[tok-1]]
		}
		cmds = append(cmds, func() tea.Msg {
			// Not timed: the count of unsafe bytes, and the pictures that
			// keep the naive pane from sending controls to this terminal.
			var unsafe int
			if i == left {
				unsafe = len(src) - len(bubbleteandmd.Sanitize(src))
			} else {
				src, unsafe = visible(src)
			}
			t0 := time.Now()
			view, lines, err := eng.render(src, end)
			took := time.Since(t0)
			return renderedMsg{pane: i, gen: gen, tok: tok, step: step, final: end, view: view, lines: lines,
				err: err, took: took, end: clock(), unsafe: unsafe}
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
	case "p":
		m.frozen = !m.frozen
		return nil
	case "n":
		if m.halted() {
			m.release(1, m.clock())
		}
		return m.kick()
	case "s":
		m.summary = !m.summary
		return nil
	case "c":
		m.next(1)
		return m.kick()
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
	if m.summary {
		return nil
	}
	if p := m.panes[left]; p.busy || p.rebuild {
		m.keys = append(m.keys, k)
		return nil
	}
	m.docKey(k)
	return m.kick()
}

// next switches to the scenario d steps on in scenarioNames and starts it.
func (m *model) next(d int) {
	at := 0
	for i, n := range scenarioNames {
		if n == m.scen.name {
			at = i
		}
	}
	name := scenarioNames[(at+d+len(scenarioNames))%len(scenarioNames)]
	s, err := scenarioByName(name)
	if err != nil {
		return
	}
	m.load(s)
	m.restart()
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
