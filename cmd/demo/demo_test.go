package main

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"github.com/iodesystems/bubbleteandmd"
)

func TestMain(m *testing.M) {
	bubbleteandmd.DisableLexTimeout()
	os.Exit(m.Run())
}

// run feeds msg to m and runs every command it returns at once (renders
// are synchronous here; the test sends its own ticks).
func run(t *testing.T, m *model, msg tea.Msg) {
	t.Helper()
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		msg, queue = queue[0], queue[1:]
		_, cmd := m.Update(msg)
		queue = append(queue, exec(cmd)...)
	}
}

func exec(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, exec(c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

func key(code rune) tea.KeyPressMsg {
	k := tea.KeyPressMsg{Code: code}
	if code >= 'a' && code <= 'z' {
		k.Text = string(code)
	}
	return k
}

// testModel makes a model on scenario name whose clock is the time of the
// last tick sent with tickAt.
func testModel(t *testing.T, style ansi.StyleConfig, styleName, name string) (*model, *time.Time) {
	t.Helper()
	s, err := scenarioByName(name)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(config{style: style, styleName: styleName, rate: 100, wide: true, scen: s, seed: 42})
	now := new(time.Time)
	*now = time.Unix(0, 0)
	m.tick = func() tea.Cmd { return nil }
	m.clock = func() time.Time { return *now }
	run(t, m, m.Init())
	return m, now
}

// tickAt sends tick i, 100 ms after tick i-1.
func tickAt(t *testing.T, m *model, now *time.Time, i int) {
	t.Helper()
	*now = time.Unix(0, 0).Add(time.Duration(i) * 100 * time.Millisecond)
	run(t, m, tickMsg(*now))
}

func checkLines(t *testing.T, m *model, i int) {
	t.Helper()
	if v := m.View().Content; v == "" || strings.Count(v, "\n") != m.ph+chrome-1 {
		t.Fatalf("tick %d: view has %d lines, want %d", i, strings.Count(v, "\n")+1, m.ph+chrome)
	}
}

// streamToEnd ticks from tick i until both panes show the Final output.
func streamToEnd(t *testing.T, m *model, now *time.Time, i int) int {
	t.Helper()
	for ; m.pos < len(m.ends) || !m.panes[left].final || !m.panes[right].final; i++ {
		if i > 100000 {
			t.Fatal("stream did not end")
		}
		tickAt(t, m, now, i)
		checkLines(t, m, i)
	}
	return i
}

func TestModelHeadless(t *testing.T) {
	for _, name := range []string{"dark", "notty"} {
		t.Run(name, func(t *testing.T) {
			style := styles.DarkStyleConfig
			if name == "notty" {
				style = styles.NoTTYStyleConfig
			}
			m, now := testModel(t, style, name, "showcase")
			run(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
			keys := map[int]tea.KeyPressMsg{95: key(tea.KeyTab), 96: key(tea.KeyEnter), 97: key(tea.KeyDown), 98: key(tea.KeyUp), 99: key(tea.KeyRight)}
			for i := 0; m.pos < len(m.ends) || !m.panes[left].final || !m.panes[right].final; i++ {
				if i > 10000 {
					t.Fatal("stream did not end")
				}
				tickAt(t, m, now, i)
				if k, ok := keys[i]; ok {
					run(t, m, k)
				}
				if i == 50 {
					run(t, m, tea.WindowSizeMsg{Width: 100, Height: 30}) // resize mid-stream
				}
				checkLines(t, m, i)
			}
			for i, p := range m.panes {
				if p.err != nil || p.st.frames == 0 || p.view == "" || p.st.finalAt.IsZero() || p.st.unsafe != 0 {
					t.Fatalf("pane %d: err %v, %d frames, final at %v, unsafe %d", i, p.err, p.st.frames, p.st.finalAt, p.st.unsafe)
				}
			}
			if !m.focused {
				t.Fatal("tab did not focus the Doc")
			}
			if !m.summary || !strings.Contains(m.View().Content, "Summary") {
				t.Fatal("no summary at the end of the stream")
			}
			checkLines(t, m, -1)
			run(t, m, key('s'))
			if m.summary || strings.Contains(m.View().Content, "Summary") {
				t.Fatal("s did not close the summary")
			}

			// The streamed Doc at the bottom equals a fresh Doc's Final in
			// the same State, at the bottom.
			run(t, m, key(tea.KeyEnd))
			got := m.panes[left].view
			fresh, err := newDocEngine(style, name, m.pw, m.ph, true, bubbleteandmd.NewCache(64), m.doc().d)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fresh.d.Final(m.scen.src); err != nil {
				t.Fatal(err)
			}
			fresh.d.Do(bubbleteandmd.ActionBottom)
			if want := fresh.d.View(); got != want {
				t.Fatalf("left view != Doc.Final view\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestScenarios(t *testing.T) {
	for _, name := range scenarioNames {
		s, err := scenarioByName(name)
		if err != nil || s.name != name || s.src == "" || s.about == "" {
			t.Fatalf("%s: %q, %v", name, s.name, err)
		}
		if (name == "thread") != (s.start > 0) {
			t.Fatalf("%s: start %d", name, s.start)
		}
	}
	if _, err := scenarioByName("nope"); err == nil {
		t.Fatal("unknown scenario accepted")
	}
}

func TestScenarioSwitch(t *testing.T) {
	m, now := testModel(t, styles.DarkStyleConfig, "dark", "heal")
	run(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	for i := range 5 {
		tickAt(t, m, now, i)
	}
	run(t, m, key('c')) // heal → unsafe
	if m.scen.name != "unsafe" || m.pos != 0 {
		t.Fatalf("after c: scenario %s, pos %d", m.scen.name, m.pos)
	}
	streamToEnd(t, m, now, 5)
	if !strings.Contains(m.header(), "unsafe") {
		t.Fatal("header does not name the scenario")
	}

	// thread: the history is on the page before the first tick.
	m.next(4) // unsafe → long → code → forms → thread
	if m.scen.name != "thread" || m.first != 1 || m.pos != 1 || m.ends[0] != m.scen.start {
		t.Fatalf("thread: scenario %s, first %d, pos %d", m.scen.name, m.first, m.pos)
	}
	run(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	if m.panes[left].shown != 1 || m.panes[right].shown != 1 {
		t.Fatal("thread history not drawn before the stream")
	}
}

func TestFreeze(t *testing.T) {
	m, now := testModel(t, styles.DarkStyleConfig, "dark", "heal")
	run(t, m, tea.WindowSizeMsg{Width: 160, Height: 30})
	i := 0
	for ; m.pos < 60; i++ {
		tickAt(t, m, now, i)
	}
	run(t, m, key('p'))
	at := m.pos
	for range 5 {
		tickAt(t, m, now, i)
		i++
	}
	if m.pos != at || m.panes[left].shown != at || m.panes[right].shown != at {
		t.Fatalf("frozen at %d: pos %d, shown %d / %d", at, m.pos, m.panes[left].shown, m.panes[right].shown)
	}
	if h := m.header(); !strings.Contains(h, "FROZEN") || !strings.Contains(h, "both panes show it") {
		t.Fatalf("header: %q", h)
	}
	run(t, m, key('n'))
	if m.pos != at+1 || m.panes[left].shown != at+1 || m.panes[right].shown != at+1 {
		t.Fatalf("n: pos %d, shown %d / %d", m.pos, m.panes[left].shown, m.panes[right].shown)
	}
	run(t, m, key('p'))
	tickAt(t, m, now, i)
	tickAt(t, m, now, i+1)
	if m.pos <= at+1 {
		t.Fatal("p did not resume the stream")
	}
}

func TestUnsafe(t *testing.T) {
	m, now := testModel(t, styles.DarkStyleConfig, "dark", "unsafe")
	run(t, m, tea.WindowSizeMsg{Width: 200, Height: 50})
	streamToEnd(t, m, now, 0)
	l, r := m.panes[left], m.panes[right]
	if l.st.unsafe == 0 || r.st.unsafe == 0 || !strings.Contains(r.view, "␛") {
		t.Fatalf("unsafe: left %d B, right %d", l.st.unsafe, r.st.unsafe)
	}
	run(t, m, key('s')) // back to the panes
	v := m.View().Content
	for _, bad := range []string{"\x1b]", "\x1b[2J", "\x1b[?", "\x07", "‮"} {
		if strings.Contains(v, bad) {
			t.Fatalf("screen contains %q", bad)
		}
	}
}

func TestShrinkFlash(t *testing.T) {
	m, now := testModel(t, styles.NoTTYStyleConfig, "notty", "heal")
	run(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	p := m.panes[right]
	m.done(renderedMsg{pane: right, gen: p.gen, tok: 4, lines: 5, end: *now})
	m.done(renderedMsg{pane: right, gen: p.gen, tok: 5, lines: 3, end: *now})
	if p.st.shrank != 1 || !strings.Contains(m.title(right, p), "shrank 2 line") {
		t.Fatalf("shrank %d, title %q", p.st.shrank, m.title(right, p))
	}
	*now = now.Add(flashFor)
	if strings.Contains(m.title(right, p), "shrank") {
		t.Fatal("flash did not end")
	}
}

func TestVisible(t *testing.T) {
	got, n := visible("a\x1b[2Jb\tc\n‮d\x7f\u0085")
	if want := "a␛[2Jb\tc\n⟨U+202E⟩d␡⟨U+0085⟩"; got != want || n != 4 {
		t.Fatalf("got %q %d, want %q 4", got, n, want)
	}
	if s, n := visible("plain"); s != "plain" || n != 0 {
		t.Fatal("plain text changed")
	}
}

func TestSpark(t *testing.T) {
	var b [buckets]time.Duration
	b[0], b[buckets-1] = time.Millisecond, 4*time.Millisecond
	if got := spark(&b, 4, 4*time.Millisecond); got != "▃  █" {
		t.Fatalf("spark %q", got)
	}
}

func TestTokens(t *testing.T) {
	src := "héllo, wörld — streaming ✓\n"
	ends := tokens(src, 7)
	if ends[len(ends)-1] != len(src) {
		t.Fatal("last token does not end the source")
	}
	for i, e := range ends {
		if i > 0 && e <= ends[i-1] || !strings.HasPrefix(src, src[:e]) || !utf8Valid(src[:e]) {
			t.Fatalf("token %d ends at %d", i, e)
		}
	}
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "�") == s }
