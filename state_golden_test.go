package bubbleteandmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
	"github.com/iodesystems/bubbleteandmd/internal/join"
)

var update = flag.Bool("update", false, "rewrite golden files")

// init renders one code block in the dark style before any test runs.
// glamour v2.0.1 registers its chroma theme once per process, from the first
// style that renders a code block (ansi/codeblock.go), so a golden with code
// in it depends on which test ran first; this pins the theme to dark for
// every test of the package, also under -run.
func init() {
	r, err := glamour.NewTermRenderer(glamour.WithStyles(*styles.DefaultStyles["dark"]))
	if err == nil {
		_, _ = r.Render("```go\nx := 1\n```\n")
	}
}

// golden compares untie(got) with testdata/golden/<name>.golden; -update
// rewrites it.
func golden(t *testing.T, name, got string) {
	t.Helper()
	got = untie(got)
	p := filepath.Join("testdata", "golden", name+".golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("golden %s: %v (run with -update)", p, err)
	}
	if string(want) != got {
		t.Errorf("golden %s differs: %s", p, firstDiff(string(want), got))
	}
}

// TestDirectiveDocsGolden: each testdata/directives document as
// DirectiveRenderer.Render draws it (no state), per style and width.
func TestDirectiveDocsGolden(t *testing.T) {
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{36, 76} {
			r := newTestRenderer(t, style, w)
			for _, d := range directiveDocs(t) {
				out, err := r.Render(d.text)
				if err != nil {
					t.Fatal(err)
				}
				golden(t, fmt.Sprintf("docs/%s_%s_%d", strings.TrimPrefix(d.name, "dir-"), style, w), out)
			}
		}
	}
}

// stateDocs: testdata/directives and the corpus documents with forms.
func stateDocs(t *testing.T) []invCase {
	cs := directiveDocs(t)
	for _, d := range corpus.Docs() {
		if hasForm(d.Text) {
			cs = append(cs, invCase{"doc-" + d.Name, d.Text})
		}
	}
	return cs
}

// TestZeroStateGolden: RenderState with a nil or empty State draws the
// goldens Render drew before state existed, whole and block by block.
func TestZeroStateGolden(t *testing.T) {
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{36, 76} {
			r := newTestRenderer(t, style, w)
			for _, d := range directiveDocs(t) {
				name := fmt.Sprintf("docs/%s_%s_%d", strings.TrimPrefix(d.name, "dir-"), style, w)
				for _, st := range []*directive.State{nil, {}} {
					out, _, err := r.RenderState(d.text, 0, st)
					if err != nil {
						t.Fatal(err)
					}
					golden(t, name, out)
				}
				var whole, blocked []string
				for _, b := range blocks.Split(d.text) {
					in := join.Input(b.Source, b.Offset == 0)
					a, err := r.Render(in)
					if err != nil {
						t.Fatal(err)
					}
					s, _, err := r.RenderState(in, b.Offset, &directive.State{})
					if err != nil {
						t.Fatal(err)
					}
					whole, blocked = append(whole, a), append(blocked, s)
				}
				if strings.Join(whole, "") != strings.Join(blocked, "") {
					t.Errorf("%s: block renders differ with empty State", name)
				}
			}
		}
	}
}

// formsIn walks src as directive.IDs does and returns each form by key.
func formsIn(block string, at int, parent string, out map[string]directive.Directive) {
	src, _, shifts := directive.MathFences(block)
	for pos := 0; ; {
		d, ok := directive.Parse(src[pos:])
		if !ok {
			return
		}
		key := strconv.Itoa(at + directive.RawOffset(shifts, pos+d.Start))
		if parent != "" {
			key = parent + "@" + key
		}
		out[key] = d
		body := func(md, slot string) {
			for _, b := range blocks.Split(md) {
				formsIn(b.Source, b.Offset, key+"/"+slot, out)
			}
		}
		if d.After != "" {
			body(d.After, "a")
		}
		switch {
		case d.Callout != nil:
			body(d.Callout.Body, "b")
		case d.Details != nil:
			body(d.Details.Body, "b")
		case d.Tabs != nil:
			for i, p := range d.Tabs.Panels {
				body(p.Body, "t"+strconv.Itoa(i))
			}
		case d.Steps != nil:
			for i, s := range d.Steps.Items {
				body(s.Body, "s"+strconv.Itoa(i))
			}
		case d.Verdict != nil:
			for i, it := range d.Verdict.Pros {
				body(it.Body, "p"+strconv.Itoa(i))
			}
			for i, it := range d.Verdict.Cons {
				body(it.Body, "c"+strconv.Itoa(i))
			}
		}
		pos += d.End
	}
}

// docForms is every form of doc by key, and doc's FormIDs.
func docForms(t *testing.T, doc string) (map[string]directive.Directive, []directive.FormID) {
	forms := map[string]directive.Directive{}
	var ids []directive.FormID
	for _, b := range blocks.Split(doc) {
		formsIn(b.Source, b.Offset, "", forms)
		ids = append(ids, directive.IDs(b.Source, b.Offset)...)
	}
	if len(forms) != len(ids) {
		t.Fatalf("formsIn found %d forms, IDs %d", len(forms), len(ids))
	}
	for _, id := range ids {
		if d, ok := forms[id.Key]; !ok || d.Kind != id.Kind {
			t.Fatalf("IDs key %s (%v) not found by formsIn", id.Key, id.Kind)
		}
	}
	return forms, ids
}

// TestStateKeysMatchIDs: with no state every interactive form is drawn, so
// the keys in the marks are exactly the interactive keys directive.IDs
// gives, each with marks of its kind.
func TestStateKeysMatchIDs(t *testing.T) {
	docs := stateDocs(t)
	for _, sc := range corpus.Scenarios() {
		if sc.Name == "forms-20k" {
			docs = append(docs, invCase{"scenario-" + sc.Name, sc.Text})
		}
	}
	for _, f := range corpus.Fuzz(1, 100) {
		if hasForm(f) {
			docs = append(docs, invCase{"fuzz", f})
		}
	}
	checked, nested := 0, 0
	r := newTestRenderer(t, "dark", 78)
	for _, d := range docs {
		text := Sanitize(d.text)
		_, ids := docForms(t, text)
		_, marks, err := r.RenderState(text, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]map[directive.MarkKind]int{}
		for _, m := range marks {
			if got[m.Key] == nil {
				got[m.Key] = map[directive.MarkKind]int{}
			}
			got[m.Key][m.Kind]++
		}
		want := map[string]bool{}
		for _, id := range ids {
			if !id.Interactive() {
				if got[id.Key] != nil {
					t.Errorf("%s: marks for %v form %s", d.name, id.Kind, id.Key)
				}
				continue
			}
			want[id.Key] = true
			g := got[id.Key]
			checked++
			if id.Parent != "" {
				nested++
			}
			switch id.Kind {
			case directive.KindDetails:
				if g[directive.MarkHeader] != 1 || len(g) != 1 {
					t.Errorf("%s: details %s marks %v", d.name, id.Key, g)
				}
			case directive.KindTabs:
				if id.Panels > 0 && (g[directive.MarkHeader] != 1 || g[directive.MarkPanel] != id.Panels || len(g) != 2) {
					t.Errorf("%s: tabs %s (%d panels) marks %v", d.name, id.Key, id.Panels, g)
				}
			case directive.KindTree:
				if g[directive.MarkRow] < len(id.Paths) || len(g) > 1 {
					t.Errorf("%s: tree %s (%d folders) marks %v", d.name, id.Key, len(id.Paths), g)
				}
			}
		}
		for k := range got {
			if !want[k] {
				t.Errorf("%s: mark key %s is not an interactive IDs key", d.name, k)
			}
		}
	}
	if nested == 0 {
		t.Error("no nested interactive form checked")
	}
	t.Logf("%d interactive forms (%d nested) checked in %d docs", checked, nested, len(docs))
}

// lineMapStates are states that change every interactive form of a doc:
// zero, then each kind toggled, with focus moving over the forms.
func lineMapStates(ids []directive.FormID) []*directive.State {
	var inter []directive.FormID
	for _, id := range ids {
		if id.Interactive() {
			inter = append(inter, id)
		}
	}
	out := []*directive.State{nil}
	for v := 0; v < 3; v++ {
		st := &directive.State{Tabs: map[string]directive.TabsState{}, Details: map[string]directive.DetailsState{}, Trees: map[string]directive.TreeState{}}
		for i, id := range inter {
			switch id.Kind {
			case directive.KindTabs:
				sel := []int{0, 1, id.Panels - 1}[v]
				st.Tabs[id.Key] = directive.TabsState{Tabbed: v != 1 || i%2 == 0, Selected: sel}
			case directive.KindDetails:
				st.Details[id.Key] = directive.DetailsState{Folded: (i+v)%2 == 0}
			case directive.KindTree:
				ts := directive.TreeState{Closed: map[string]bool{}}
				for j, p := range id.Paths {
					if (j+v)%2 == 1 {
						ts.Closed[p] = true
					}
				}
				if len(id.Paths) > 0 {
					ts.Cursor = id.Paths[len(id.Paths)-1]
				}
				st.Trees[id.Key] = ts
			}
		}
		if len(inter) > 0 {
			st.Focused = inter[v%len(inter)].Key
		}
		out = append(out, st)
	}
	return out
}

// headText is a prefix of the text the line of m must hold.
func headText(d directive.Directive, m directive.Mark, st *directive.State) string {
	cut := func(s string, n int) string {
		if r := []rune(s); len(r) > n {
			return string(r[:n])
		}
		return s
	}
	var ts directive.TabsState
	if st != nil {
		ts = st.Tabs[m.Key]
	}
	switch {
	case m.Kind == directive.MarkRow:
		row := d.Tree.Rows[m.Index]
		name := row.Name
		if !row.Folder {
			return cut(name, 6)
		}
		if !strings.HasSuffix(name, "/") {
			name += "/"
		}
		if st != nil {
			if t, ok := st.Trees[m.Key]; ok {
				name = map[bool]string{true: "▸ ", false: "▾ "}[t.Closed[row.Path]] + name
			}
		}
		return cut(name, 6)
	case d.Kind == directive.KindDetails:
		title := d.Details.Title
		if title == "" {
			title = "Details"
		}
		glyph := "▾ "
		if st != nil && st.Details[m.Key].Folded {
			glyph = "▸ "
		}
		return glyph + cut(strings.Fields(title)[0], 4)
	case ts.Tabbed:
		return fmt.Sprintf("‹%d/%d› ", m.Index+1, len(d.Tabs.Panels))
	case m.Kind == directive.MarkPanel:
		return "▸ " + cut(d.Tabs.Panels[m.Index].Label, 4)
	default:
		return cut(d.Tabs.Panels[0].Label, 4)
	}
}

// focusGlyphs maps the notty focus glyphs back to the unfocused ones.
var focusGlyphs = strings.NewReplacer("▶", "▸", "▼", "▾", "«", "‹", "»", "›")

// TestStateLineMap: in every state, each mark's line holds its header
// text and its lines exist, whole and block by block as Stream renders.
func TestStateLineMap(t *testing.T) {
	marks := 0
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{36, 76} {
			r := newTestRenderer(t, style, w)
			for _, d := range stateDocs(t) {
				text := Sanitize(d.text)
				forms, ids := docForms(t, text)
				for si, st := range lineMapStates(ids) {
					type unit struct {
						in  string
						off int
					}
					units := []unit{{text, 0}}
					for _, b := range blocks.Split(text) {
						units = append(units, unit{join.Input(b.Source, b.Offset == 0), b.Offset})
					}
					for _, u := range units {
						out, ms, err := r.RenderState(u.in, u.off, st)
						if err != nil {
							t.Fatal(err)
						}
						// notty focus swaps glyphs in place; undo it for the text check
						lines := strings.Split(focusGlyphs.Replace(xansi.Strip(out)), "\n")
						for _, m := range ms {
							marks++
							if m.Line < 0 || m.Height < 1 || m.Line+m.Height > len(lines) {
								t.Errorf("%s/%s/w%d/state %d: mark %+v outside %d lines", d.name, style, w, si, m, len(lines))
								continue
							}
							want := headText(forms[m.Key], m, st)
							if !strings.Contains(lines[m.Line], want) {
								t.Errorf("%s/%s/w%d/state %d: %s mark %s at line %d: %q lacks %q", d.name, style, w, si, m.Kind, m.Key, m.Line, lines[m.Line], want)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d marks checked", marks)
}
