package directive

import (
	"fmt"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

// stateCase is one form drawn in one state.
type stateCase struct {
	name string
	d    func(t *testing.T) Directive
	st   *State
	ctx  func(t *testing.T, style string, w int) Ctx
}

const stateKey = "7"

func stateCases() []stateCase {
	tabs := func(*testing.T) Directive { return tabsDir() }
	details := func(*testing.T) Directive {
		return Directive{Kind: KindDetails, Details: &Details{Title: "More about it", Body: calloutMD}}
	}
	tree := func(t *testing.T) Directive { return firstDirective(t, codeTreeSrc) }
	rows := ParseTree(strings.TrimSuffix(strings.TrimPrefix(codeTreeSrc, "```tree\n"), "```\n"))
	path := func(name string) string {
		for _, r := range rows {
			if r.Name == name {
				return r.Path
			}
		}
		panic(name)
	}
	tabbed := func(sel int, focus bool) *State {
		s := &State{Tabs: map[string]TabsState{stateKey: {Tabbed: true, Selected: sel}}}
		if focus {
			s.Focused = stateKey
		}
		return s
	}
	return []stateCase{
		{"tabs-sel0", tabs, tabbed(0, false), tabsCtx},
		{"tabs-sel1", tabs, tabbed(1, false), tabsCtx},
		{"tabs-sellast", tabs, tabbed(2, false), tabsCtx},
		{"tabs-sel9", tabs, tabbed(9, false), tabsCtx}, // clamped to last
		{"tabs-focus", tabs, &State{Focused: stateKey}, tabsCtx},
		{"tabs-sel1-focus", tabs, tabbed(1, true), tabsCtx},
		{"details-folded", details, &State{Details: map[string]DetailsState{stateKey: {Folded: true}}}, calloutCtx},
		{"details-folded-focus", details, &State{Details: map[string]DetailsState{stateKey: {Folded: true}}, Focused: stateKey}, calloutCtx},
		{"details-focus", details, &State{Focused: stateKey}, calloutCtx},
		{"tree-marks", tree, &State{Trees: map[string]TreeState{stateKey: {}}}, codeCtx},
		{"tree-closed", tree, &State{Trees: map[string]TreeState{stateKey: {Closed: map[string]bool{path("app/"): true, path("guide/"): true}}}}, codeCtx},
		{"tree-closed-root", tree, &State{Trees: map[string]TreeState{stateKey: {Closed: map[string]bool{path("src/"): true}}}}, codeCtx},
		{"tree-focus", tree, &State{Focused: stateKey}, codeCtx},
		// cursor under a closed folder shows on the folder
		{"tree-focus-hidden", tree, &State{Focused: stateKey, Trees: map[string]TreeState{stateKey: {Closed: map[string]bool{path("docs/"): true}, Cursor: path("intro.md")}}}, codeCtx},
		{"tree-focus-row", tree, &State{Focused: stateKey, Trees: map[string]TreeState{stateKey: {Cursor: path("util.go")}}}, codeCtx},
	}
}

// TestStateGolden: each form in each state, 3 styles × 2 widths.
func TestStateGolden(t *testing.T) {
	for _, sc := range stateCases() {
		for _, style := range []string{"dark", "light", "notty"} {
			for _, w := range []int{36, 76} {
				c := sc.ctx(t, style, w)
				c.State, c.Key = sc.st, stateKey
				lines, marks, err := Render(sc.d(t), c)
				if err != nil {
					t.Fatal(err)
				}
				checkWidth(t, lines, w)
				if style == "notty" && strings.Contains(strings.Join(lines, ""), "\x1b") {
					t.Errorf("%s: notty has SGR", sc.name)
				}
				var ms strings.Builder
				for _, m := range marks {
					fmt.Fprintf(&ms, "%s %s line %d height %d index %d %s\n", m.Key, m.Kind, m.Line, m.Height, m.Index, m.Path)
					if m.Line < 0 || m.Height < 1 || m.Line+m.Height > len(lines) {
						t.Errorf("%s/%s/%d: mark out of range: %+v of %d lines", sc.name, style, w, m, len(lines))
					}
				}
				golden(t, fmt.Sprintf("state/%s_%s_%d", sc.name, style, w), strings.Join(lines, "\n")+"\n--- marks\n"+ms.String())
			}
		}
	}
}

// TestZeroStateEqual: a nil State, an empty State, and a State with
// entries for other keys draw the bytes of the static layout.
func TestZeroStateEqual(t *testing.T) {
	for _, sc := range stateCases() {
		for _, style := range []string{"dark", "notty"} {
			c := sc.ctx(t, style, 76)
			want, _, err := Render(sc.d(t), c)
			if err != nil {
				t.Fatal(err)
			}
			for _, st := range []*State{{}, sc.st} {
				c.State, c.Key = st, "other"
				got, _, err := Render(sc.d(t), c)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Join(got, "\n") != strings.Join(want, "\n") {
					t.Errorf("%s/%s: state for another key changed the form", sc.name, style)
				}
			}
		}
	}
}

// TestStateMarksText: each mark's line holds its header text.
func TestStateMarksText(t *testing.T) {
	for _, sc := range stateCases() {
		c := sc.ctx(t, "dark", 76)
		c.State, c.Key = sc.st, stateKey
		d := sc.d(t)
		lines, marks, err := Render(d, c)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range marks {
			l := xansi.Strip(lines[m.Line])
			var want string
			switch {
			case m.Kind == MarkRow:
				want = d.Tree.Rows[m.Index].Name
			case d.Kind == KindDetails:
				want = d.Details.Title
			case strings.HasPrefix(l, "‹"):
				want = fmt.Sprintf("‹%d/%d› %s", m.Index+1, len(d.Tabs.Panels), d.Tabs.Panels[m.Index].Label)
			case m.Kind == MarkPanel:
				want = "▸ " + d.Tabs.Panels[m.Index].Label
			default:
				want = d.Tabs.Panels[0].Label
			}
			if !strings.Contains(l, want) {
				t.Errorf("%s: %s mark at line %d: %q lacks %q", sc.name, m.Kind, m.Line, l, want)
			}
		}
	}
}

// TestFocusGlyphWidth: each notty focus glyph swap keeps the cell width.
func TestFocusGlyphWidth(t *testing.T) {
	for _, p := range [][2]string{{"▸", "▶"}, {"▾", "▼"}, {"‹", "«"}, {"›", "»"}, {"──", "━━"}} {
		if a, b := xansi.StringWidth(p[0]), xansi.StringWidth(p[1]); a != b || a == 0 {
			t.Errorf("%s=%d %s=%d", p[0], a, p[1], b)
		}
	}
}
