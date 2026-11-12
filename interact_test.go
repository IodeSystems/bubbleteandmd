package bubbleteandmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// docReply has every kind of stop, one nested in a tab panel, and a tree
// whose first row is a file.
const docReply = `Intro paragraph.

> [!DETAILS] More
> Hidden text.

` + "```tree\nREADME.md\nsrc/\n├── app/\n│   └── main.go\n└── lib/\n    └── util.go\n```\n" + `
> [!TABS] Install
#### macOS
Use brew.
#### Linux
> [!DETAILS] Distro notes
> apt or dnf.

#### Windows
Download it.
`

var docStyles = []string{"dark", "light", "notty"}

func newTestDoc(t *testing.T, style string, w int) *Doc {
	t.Helper()
	return NewDoc(newTestRenderer(t, style, w), style+"/"+strconv.Itoa(w), NewCache(64))
}

func docFinal(t *testing.T, d *Doc, src string) string {
	t.Helper()
	out, err := d.Final(src)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func stopKeys(ss []Stop) []string {
	ks := make([]string, len(ss))
	for i, s := range ss {
		ks[i] = s.Key
	}
	return ks
}

// widths is the cell width of each line: what must not change on focus.
func widths(s string) []int {
	lines := strings.Split(s, "\n")
	ws := make([]int, len(lines))
	for i, l := range lines {
		ws[i] = xansi.StringWidth(l)
	}
	return ws
}

// TestDocStops: one stop per drawn interactive form, in document order,
// with its kind and lines; a nested form after its parent.
func TestDocStops(t *testing.T) {
	for _, style := range docStyles {
		d := newTestDoc(t, style, 60)
		out := docFinal(t, d, docReply)
		ss := d.Stops()
		kinds := make([]StopKind, len(ss))
		for i, s := range ss {
			kinds[i] = s.Kind
		}
		want := []StopKind{StopDetails, StopTree, StopTabs, StopDetails}
		if !slices.Equal(kinds, want) {
			t.Fatalf("%s: stops %+v", style, ss)
		}
		if !strings.Contains(ss[3].Key, "/") {
			t.Errorf("%s: nested details key %q", style, ss[3].Key)
		}
		lines := strings.Split(xansi.Strip(out), "\n")
		for _, s := range ss {
			if s.Height < 1 || s.Line+s.Height > len(lines) {
				t.Fatalf("%s: stop %+v of %d lines", style, s, len(lines))
			}
		}
		if !strings.Contains(lines[ss[0].Line], "More") || !strings.Contains(lines[ss[2].Line], "macOS · Linux · Windows") ||
			!strings.Contains(lines[ss[3].Line], "Distro notes") || !strings.Contains(lines[ss[1].Line], "README.md") {
			t.Errorf("%s: stop lines %+v in\n%s", style, ss, strings.Join(lines, "\n"))
		}
		// the tabs stop spans the nested details
		if ss[3].Line < ss[2].Line || ss[3].Line+ss[3].Height > ss[2].Line+ss[2].Height {
			t.Errorf("%s: nested %+v outside tabs %+v", style, ss[3], ss[2])
		}
	}
}

// TestDocLayoutUntilInteraction: a Doc draws what a Stream with no State
// draws at every frame and at the end; focus on any stop keeps every line's
// width and the line count; Blur gives the bytes back.
func TestDocLayoutUntilInteraction(t *testing.T) {
	docs := []invCase{{"reply", docReply}}
	for _, c := range directiveDocs(t) {
		docs = append(docs, c)
	}
	for _, style := range docStyles {
		for _, w := range []int{36, 76} {
			r := newTestRenderer(t, style, w)
			for _, c := range docs {
				key := fmt.Sprintf("%s/%d", style, w)
				d := NewDoc(r, key, NewCache(64))
				s := NewStream(r, key, NewCache(64))
				for _, p := range corpus.LinePrefixes(c.text) {
					got, err := d.Frame(p)
					if err != nil {
						t.Fatal(err)
					}
					want, err := s.Frame(p)
					if err != nil {
						t.Fatal(err)
					}
					if got != want {
						t.Fatalf("%s/%s/%d: Frame at %d: %s", c.name, style, w, len(p), firstDiff(want, got))
					}
				}
				plain := docFinal(t, d, c.text)
				if want, _ := s.Final(c.text); plain != want {
					t.Fatalf("%s/%s/%d: Final: %s", c.name, style, w, firstDiff(want, plain))
				}
				if !d.Focus() {
					if len(d.Stops()) > 0 {
						t.Fatalf("%s: Focus false with stops", c.name)
					}
					continue
				}
				for n := 0; ; n++ {
					got := docFinal(t, d, c.text)
					if !slices.Equal(widths(got), widths(plain)) {
						f, _ := d.Focused()
						t.Fatalf("%s/%s/%d: focus on %+v moves lines:\n%s\n---\n%s", c.name, style, w, f, plain, got)
					}
					if style == "notty" && got == plain {
						f, _ := d.Focused()
						// no notty cue (decisions.md): stacked tabs, a
						// depth-0 tree row with no fold mark
						if f.Kind != StopTabs && f.Kind != StopTree {
							t.Errorf("%s/%d: notty focus on %+v has no cue", c.name, w, f)
						}
					}
					if !d.Do(ActionNext) {
						break
					}
				}
				if d.State().Focused != "" {
					t.Fatalf("%s: ActionNext past the last stop did not blur", c.name)
				}
				if got := docFinal(t, d, c.text); got != plain {
					t.Fatalf("%s/%s/%d: after Blur: %s", c.name, style, w, firstDiff(plain, got))
				}
			}
		}
	}
}

// TestDocFocusOrderStreamed: while a reply streams, stops arrive in
// document order and the stops before the tail block never reorder; the
// focus stays on its form as the reply grows.
func TestDocFocusOrderStreamed(t *testing.T) {
	docs := []invCase{{"reply", docReply}}
	docs = append(docs, stateDocs(t)...)
	for _, style := range []string{"dark", "notty"} {
		r := newTestRenderer(t, style, 70)
		for _, c := range docs {
			text := Sanitize(c.text)
			d := NewDoc(r, style, NewCache(64))
			var prev []string
			focused := ""
			for _, p := range corpus.LinePrefixes(text) {
				if _, err := d.Frame(p); err != nil {
					t.Fatal(err)
				}
				keys := stopKeys(d.Stops())
				bs := blocks.Split(p)
				tail := bs[len(bs)-1].Offset
				var stable []string
				for _, k := range prev {
					root, _, _ := strings.Cut(k, "/")
					if n, _ := strconv.Atoi(root); n < tail {
						stable = append(stable, k)
					}
				}
				if len(keys) < len(stable) || !slices.Equal(keys[:len(stable)], stable) {
					t.Fatalf("%s/%s at %d: stops %v, before %v", c.name, style, len(p), keys, prev)
				}
				prev = keys
				if focused == "" && len(keys) > 0 {
					d.Focus()
					focused = d.State().Focused
				}
				if d.State().Focused != focused {
					t.Fatalf("%s: focus moved %q → %q", c.name, focused, d.State().Focused)
				}
			}
			if _, err := d.Final(text); err != nil {
				t.Fatal(err)
			}
			_, ids := docForms(t, text)
			var want []string
			for _, id := range ids {
				if id.Interactive() {
					want = append(want, id.Key)
				}
			}
			if got := stopKeys(d.Stops()); !slices.Equal(got, want) {
				t.Errorf("%s/%s: final stops %v, IDs %v", c.name, style, got, want)
			}
		}
	}
}

func focusKind(t *testing.T, d *Doc, k StopKind, nth int) Stop {
	t.Helper()
	d.Blur()
	d.Reset()
	d.Focus()
	for {
		f, ok := d.Focused()
		if !ok {
			t.Fatalf("no %v stop %d", k, nth)
		}
		if f.Kind == k {
			if nth == 0 {
				return f
			}
			nth--
		}
		if !d.Do(ActionNext) {
			t.Fatalf("no %v stop", k)
		}
	}
}

// TestDocActions: what each action does to each kind of form, and whether
// it is consumed.
func TestDocActions(t *testing.T) {
	for _, style := range docStyles {
		d := newTestDoc(t, style, 60)
		plain := docFinal(t, d, docReply)
		lines := func(s string) int { return strings.Count(s, "\n") }

		// details: toggle folds, again opens; ←/→ are not its keys
		s := focusKind(t, d, StopDetails, 0)
		open := docFinal(t, d, docReply)
		if d.Do(ActionLeft) || d.Do(ActionRight) {
			t.Errorf("%s: details took ←/→", style)
		}
		if !d.Do(ActionToggle) || !d.st.Details[s.Key].Folded {
			t.Fatalf("%s: details toggle", style)
		}
		folded := docFinal(t, d, docReply)
		if lines(folded) >= lines(open) || strings.Contains(xansi.Strip(folded), "Hidden text.") {
			t.Errorf("%s: folded details still open:\n%s", style, folded)
		}
		d.Do(ActionToggle)
		if got := docFinal(t, d, docReply); got != open {
			t.Errorf("%s: details toggled twice: %s", style, firstDiff(open, got))
		}

		// tabs: → from stacked shows the selected panel, then moves; ←
		// wraps; toggle goes back to stacked
		s = focusKind(t, d, StopTabs, 0)
		stacked := docFinal(t, d, docReply)
		step := func(a Action, tabbed bool, sel int, label string) {
			t.Helper()
			if !d.Do(a) {
				t.Fatalf("%s: tabs %s not consumed", style, a)
			}
			if v := d.st.Tabs[s.Key]; v.Tabbed != tabbed || v.Selected != sel {
				t.Fatalf("%s: tabs after %s: %+v", style, a, v)
			}
			// notty draws the focused tabbed header «i/n»
			out := strings.NewReplacer("«", "‹", "»", "›").Replace(xansi.Strip(docFinal(t, d, docReply)))
			if label != "" && !strings.Contains(out, label) {
				t.Fatalf("%s: tabs after %s: no %q in\n%s", style, a, label, out)
			}
		}
		step(ActionRight, true, 0, "1/3› macOS")
		step(ActionRight, true, 1, "2/3› Linux")
		if f, _ := d.Focused(); f.Kind != StopTabs {
			t.Fatal("focus left the tabs")
		}
		step(ActionLeft, true, 0, "1/3› macOS")
		step(ActionLeft, true, 2, "3/3› Windows")
		step(ActionToggle, false, 2, "macOS · Linux · Windows")
		step(ActionToggle, true, 2, "3/3› Windows")
		d.Reset()
		d.Focus()
		focusKind(t, d, StopTabs, 0)
		if got := docFinal(t, d, docReply); got != stacked {
			t.Errorf("%s: Reset tabs: %s", style, firstDiff(stacked, got))
		}
		// enter from stacked: one-tab view too
		step(ActionToggle, true, 0, "1/3› macOS")

		// tree: cursor on a file (first row): toggle consumed, changes
		// nothing (the host must not read it as its own enter); → moves
		// to the next folder; toggle closes it; ← wraps to the last
		s = focusKind(t, d, StopTree, 0)
		if !d.Do(ActionToggle) {
			t.Errorf("%s: toggle on a file row not consumed", style)
		}
		if _, ok := d.st.Trees[s.Key]; ok {
			t.Errorf("%s: focus or toggle on a file made a tree entry", style)
		}
		if !d.Do(ActionRight) || !strings.HasPrefix(d.st.Trees[s.Key].Cursor, "src/#") {
			t.Fatalf("%s: tree → cursor %q", style, d.st.Trees[s.Key].Cursor)
		}
		treeOpen := docFinal(t, d, docReply)
		if !d.Do(ActionToggle) || !d.st.Trees[s.Key].Closed[d.st.Trees[s.Key].Cursor] {
			t.Fatalf("%s: tree toggle", style)
		}
		out := xansi.Strip(docFinal(t, d, docReply))
		if strings.Contains(out, "main.go") || !strings.Contains(out, "src/") {
			t.Errorf("%s: closed src/ shows its rows:\n%s", style, out)
		}
		// src/ closed: the only drawn folder; → wraps onto it
		if !d.Do(ActionRight) || !strings.HasPrefix(d.st.Trees[s.Key].Cursor, "src/#") {
			t.Errorf("%s: → with one folder: %q", style, d.st.Trees[s.Key].Cursor)
		}
		d.Do(ActionToggle)
		if got := docFinal(t, d, docReply); got != treeOpen {
			t.Errorf("%s: tree toggled twice: %s", style, firstDiff(treeOpen, got))
		}
		if !d.Do(ActionLeft) || !strings.HasPrefix(d.st.Trees[s.Key].Cursor, "src/lib/#") {
			t.Errorf("%s: ← from the first folder: %q", style, d.st.Trees[s.Key].Cursor)
		}
		if !d.Do(ActionLeft) || !strings.HasPrefix(d.st.Trees[s.Key].Cursor, "src/app/#") {
			t.Errorf("%s: ←: %q", style, d.st.Trees[s.Key].Cursor)
		}
		docFinal(t, d, docReply)

		d.Reset()
		if got := docFinal(t, d, docReply); got != plain {
			t.Errorf("%s: Reset: %s", style, firstDiff(plain, got))
		}
	}
}

// TestDocConsumed: what is consumed and what falls through to the host.
func TestDocConsumed(t *testing.T) {
	d := newTestDoc(t, "dark", 60)
	if d.Focus() {
		t.Fatal("Focus before any render: no stops")
	}
	docFinal(t, d, docReply)
	for _, a := range Actions {
		if d.Do(a) {
			t.Errorf("unfocused Doc consumed %s", a)
		}
	}
	km := DefaultKeyMap()
	key := func(code rune, mod tea.KeyMod) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code, Mod: mod} }
	tab, stab := key(tea.KeyTab, 0), key(tea.KeyTab, tea.ModShift)
	if d.HandleKey(tab, km) {
		t.Error("unfocused Doc took tab")
	}
	if !d.Focus() {
		t.Fatal("Focus")
	}
	if first := d.State().Focused; first != d.Stops()[0].Key {
		t.Errorf("Focus: %q", first)
	}
	if d.HandleKey(stab, km) || d.State().Focused != "" {
		t.Error("shift+tab on the first stop: not left, or consumed")
	}
	if !d.FocusLast() || !d.Do(ActionPrev) {
		t.Error("shift+tab from the last stop")
	}
	if !d.HandleKey(tab, km) {
		t.Error("tab to the last stop")
	}
	if d.HandleKey(tab, km) || d.State().Focused != "" {
		t.Error("tab past the last stop: not left, or consumed")
	}
	if !d.Focus() || d.State().Focused != stopKeys(d.Stops())[len(d.Stops())-1] {
		t.Error("Focus after Blur returns to the last focused stop")
	}
	for _, k := range []tea.KeyPressMsg{key(tea.KeyUp, 0), key(tea.KeyDown, 0), key(tea.KeyPgDown, 0), key('x', 0)} {
		if d.HandleKey(k, km) {
			t.Errorf("%s consumed", k.String())
		}
	}
	if d.State().Focused == "" {
		t.Error("an unbound key blurred the Doc")
	}
	// space toggles: its name is "space"
	d.Blur()
	d.Reset()
	d.Focus()
	if sp := (tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}); sp.String() != "space" || !d.HandleKey(sp, km) {
		t.Errorf("space %q", sp.String())
	}
	if k := d.Stops()[0].Key; !d.st.Details[k].Folded || !d.HandleKey(key(tea.KeyEnter, 0), km) || d.st.Details[k].Folded {
		t.Error("space folds, enter opens")
	}
	if d.HandleKey(key(tea.KeyRight, 0), km) {
		t.Error("→ on details consumed")
	}
	if d.HandleKey(key(tea.KeyEscape, 0), km) || d.State().Focused != "" {
		t.Error("esc: not left, or consumed")
	}
	// a rebound map: j/k move, tab is the host's
	custom := KeyMap{ActionNext: {"j"}, ActionPrev: {"k"}}
	d.Focus()
	if d.HandleKey(tab, custom) || !d.HandleKey(key('j', 0), custom) {
		t.Error("rebound keys")
	}
	if km.Label(ActionToggle) != "enter" || custom.Label(ActionToggle) != "" {
		t.Error("Label")
	}
	if a, ok := km.Action("space"); !ok || a != ActionToggle {
		t.Error("Action(space)")
	}
}

// TestDocNoStateRenderer: with a Renderer that is not a StateRenderer a
// Doc has no stops and takes no key.
func TestDocNoStateRenderer(t *testing.T) {
	d := NewDoc(plainOnly{newTestRenderer(t, "dark", 60)}, "p", nil)
	docFinal(t, d, docReply)
	if len(d.Stops()) != 0 || d.Focus() || d.Do(ActionNext) {
		t.Error("stops with a plain Renderer")
	}
}

// TestDocTreeHiddenCursor: a Cursor that names a row hidden under a closed
// folder acts on that folder, the row the renderer draws the cursor on
// (audit 2026-11-11: Doc used the first row, a file, and toggle did
// nothing).
func TestDocTreeHiddenCursor(t *testing.T) {
	d := newTestDoc(t, "notty", 60)
	docFinal(t, d, docReply)
	s := focusKind(t, d, StopTree, 0)
	const app, main = "src/app/#2", "src/app/main.go#3"
	d.st.Trees = map[string]TreeState{s.Key: {Closed: map[string]bool{app: true}, Cursor: main}}
	out := xansi.Strip(docFinal(t, d, docReply))
	if !strings.Contains(out, "▶ app/") || strings.Contains(out, "main.go") {
		t.Fatalf("renderer: cursor not on the closed app/ row:\n%s", out)
	}
	if !d.Do(ActionToggle) || d.st.Trees[s.Key].Closed[app] || d.st.Trees[s.Key].Cursor != app {
		t.Fatalf("toggle: %+v", d.st.Trees[s.Key])
	}
	if out := xansi.Strip(docFinal(t, d, docReply)); !strings.Contains(out, "main.go") {
		t.Errorf("app/ not opened:\n%s", out)
	}
}

// TestDocStateNewWidth: on a width change the host makes a new Doc and
// copies the State; form keys are byte offsets, so the new Doc draws the
// same forms folded, selected and focused as a Doc at that width that got
// the same keys.
func TestDocStateNewWidth(t *testing.T) {
	act := func(d *Doc) {
		docFinal(t, d, docReply)
		d.Focus() // details
		d.Do(ActionToggle)
		docFinal(t, d, docReply)
		d.Do(ActionNext) // tree
		d.Do(ActionRight)
		d.Do(ActionToggle) // src/ closed
		docFinal(t, d, docReply)
		d.Do(ActionNext) // tabs
		d.Do(ActionRight)
		d.Do(ActionRight) // one-tab view, Linux
		docFinal(t, d, docReply)
	}
	for _, style := range docStyles {
		old := newTestDoc(t, style, 76)
		act(old)
		nd := newTestDoc(t, style, 36)
		*nd.State() = *old.State()
		got := docFinal(t, nd, docReply)
		ref := newTestDoc(t, style, 36)
		act(ref)
		if want := docFinal(t, ref, docReply); got != want {
			t.Errorf("%s: copied state: %s", style, firstDiff(want, got))
		}
		plain := xansi.Strip(got)
		if strings.Contains(plain, "Hidden text.") || strings.Contains(plain, "main.go") || !strings.Contains(plain, "2/3") {
			t.Errorf("%s: state not drawn:\n%s", style, plain)
		}
		if f, ok := nd.Focused(); !ok || f.Kind != StopTabs {
			t.Errorf("%s: focus %v %v", style, f, ok)
		}
	}
}

type plainOnly struct{ r Renderer }

func (p plainOnly) Render(s string) (string, error) { return p.r.Render(s) }
