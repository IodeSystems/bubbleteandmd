package main

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

func key(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

func TestModelHeadless(t *testing.T) {
	for _, name := range []string{"dark", "notty"} {
		t.Run(name, func(t *testing.T) {
			style := styles.DarkStyleConfig
			if name == "notty" {
				style = styles.NoTTYStyleConfig
			}
			m := newModel(config{style: style, styleName: name, rate: 100, wide: true, src: showcase, seed: 42})
			m.tick = func() tea.Cmd { return nil }
			run(t, m, m.Init())
			run(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
			t0 := time.Unix(0, 0)
			keys := map[int]tea.KeyPressMsg{95: key(tea.KeyTab), 96: key(tea.KeyEnter), 97: key(tea.KeyDown), 98: key(tea.KeyUp), 99: key(tea.KeyRight)}
			for i := 0; m.pos < len(m.ends) || !m.panes[left].final || !m.panes[right].final; i++ {
				if i > 10000 {
					t.Fatal("stream did not end")
				}
				run(t, m, tickMsg(t0.Add(time.Duration(i)*100*time.Millisecond)))
				if k, ok := keys[i]; ok {
					run(t, m, k)
				}
				if i == 50 {
					run(t, m, tea.WindowSizeMsg{Width: 100, Height: 30}) // resize mid-stream
				}
				if v := m.View().Content; v == "" || strings.Count(v, "\n") != m.ph+3 {
					t.Fatalf("tick %d: view has %d lines, want %d", i, strings.Count(v, "\n")+1, m.ph+4)
				}
			}
			for i, p := range m.panes {
				if p.err != nil || p.st.frames == 0 || p.view == "" {
					t.Fatalf("pane %d: err %v, %d frames", i, p.err, p.st.frames)
				}
			}
			if !m.focused {
				t.Fatal("tab did not focus the Doc")
			}

			// The streamed Doc at the bottom equals a fresh Doc's Final in
			// the same State, at the bottom.
			run(t, m, key(tea.KeyEnd))
			got := m.panes[left].view
			fresh, err := newDocEngine(style, name, m.pw, m.ph, true, bubbleteandmd.NewCache(64), m.doc().d)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fresh.d.Final(m.cfg.src); err != nil {
				t.Fatal(err)
			}
			fresh.d.Do(bubbleteandmd.ActionBottom)
			if want := fresh.d.View(); got != want {
				t.Fatalf("left view != Doc.Final view\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
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
