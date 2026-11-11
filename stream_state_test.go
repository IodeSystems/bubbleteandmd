package bubbleteandmd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
	"github.com/iodesystems/bubbleteandmd/internal/join"
)

// TestStreamStateInvariant: with State S set, Final(p) is RenderState of
// Sanitize(p) with S, byte-equal, and Marks are its marks, at every line
// prefix of a Stream that grows (cache and near memo in use), and Frame is
// RenderState of the healed text (held tables, chunked fences aside); with no
// State, Final is Render.
func TestStreamStateInvariant(t *testing.T) {
	checks := 0
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{40, 78} {
			r := newTestRenderer(t, style, w)
			for _, d := range stateDocs(t) {
				text := Sanitize(d.text)
				_, ids := docForms(t, text)
				for si, st := range lineMapStates(ids) {
					s := NewStream(r, fmt.Sprintf("%s/%d", style, w), NewCache(256))
					s.SetState(st)
					for _, p := range corpus.LinePrefixes(text) {
						// Frame: RenderState of the healed text, but for a
						// held table or a chunked fence tail
						fr, err := s.Frame(p)
						if err != nil {
							t.Fatal(err)
						}
						if !s.stats.lastHeld && !s.stats.lastFence {
							want, marks, err := r.RenderState(heal.Close(Sanitize(p)), 0, st)
							if err != nil {
								t.Fatal(err)
							}
							if fr != want || !reflect.DeepEqual(s.Marks(), marks) {
								t.Fatalf("%s/%s/w%d/state %d: Frame at %d bytes: %s; marks\n got %+v\nwant %+v", d.name, style, w, si, len(p), firstDiff(want, fr), s.Marks(), marks)
							}
						}
						got, err := s.Final(p)
						if err != nil {
							t.Fatal(err)
						}
						want, marks, err := r.RenderState(Sanitize(p), 0, st)
						if err != nil {
							t.Fatal(err)
						}
						checks++
						if got != want {
							t.Fatalf("%s/%s/w%d/state %d at %d bytes: %s", d.name, style, w, si, len(p), firstDiff(want, got))
						}
						if !reflect.DeepEqual(s.Marks(), marks) {
							t.Fatalf("%s/%s/w%d/state %d at %d bytes: marks\n got %+v\nwant %+v", d.name, style, w, si, len(p), s.Marks(), marks)
						}
						if st == nil {
							plain, err := r.Render(p)
							if err != nil {
								t.Fatal(err)
							}
							if got != plain {
								t.Fatalf("%s: nil State Final differs from Render", d.name)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d prefix checks", checks)
}

// countState counts RenderState calls per input.
type countState struct {
	r *DirectiveRenderer
	n map[string]int
}

func (c countState) Render(s string) (string, error) { return c.r.Render(s) }

func (c countState) RenderState(md string, offset int, st *State) (string, []Mark, error) {
	c.n[md]++
	return c.r.RenderState(md, offset, st)
}

// TestStreamStateCache: a changed State renders the blocks it changes
// again and no other; the same State again comes from the cache.
func TestStreamStateCache(t *testing.T) {
	r := newTestRenderer(t, "dark", 60)
	text := "# Title\n\n> [!DETAILS] One\n> first body\n\npara between\n\n> [!DETAILS] Two\n> second body\n\nlast para\n\nend\n"
	_, ids := docForms(t, text)
	if len(ids) != 2 {
		t.Fatalf("want 2 forms, got %+v", ids)
	}
	one, two := ids[0].Key, ids[1].Key
	bs := blocks.Split(text)
	in := func(src string) string {
		for _, b := range bs {
			if strings.Contains(b.Source, src) {
				return join.Input(b.Source, b.Offset == 0)
			}
		}
		t.Fatalf("no block with %q", src)
		return ""
	}
	c := countState{r, map[string]int{}}
	s := NewStream(c, "k", NewCache(64))
	final := func(st *State) string {
		t.Helper()
		s.SetState(st)
		got, err := s.Final(text)
		if err != nil {
			t.Fatal(err)
		}
		want, marks, err := r.RenderState(text, 0, st)
		if err != nil {
			t.Fatal(err)
		}
		if got != want || !reflect.DeepEqual(s.Marks(), marks) {
			t.Fatalf("state %+v: Final or marks differ from RenderState", st)
		}
		return got
	}
	zero := final(nil)
	final(nil)
	if k := c.n[in("first body")]; k != 1 {
		t.Errorf("unchanged state: block One rendered %d times, want 1", k)
	}
	folded := &State{Details: map[string]DetailsState{one: {Folded: true}}}
	f1 := final(folded)
	if f1 == zero {
		t.Error("folding One did not change the output")
	}
	if k := c.n[in("first body")]; k != 2 {
		t.Errorf("One folded: block One rendered %d times, want 2", k)
	}
	if k := c.n[in("second body")]; k != 1 {
		t.Errorf("One folded: block Two rendered %d times, want 1", k)
	}
	// the host changes *st in place: read on the next call
	folded.Details[two] = DetailsState{Folded: true}
	f2 := final(folded)
	if f2 == f1 || c.n[in("second body")] != 2 {
		t.Errorf("Two folded in place: changed %v, block Two rendered %d times", f2 != f1, c.n[in("second body")])
	}
	// back to the zero state: the first renders, from the cache
	if got := final(&State{}); got != zero {
		t.Error("empty State differs from nil State")
	}
	if c.n[in("first body")] != 2 || c.n[in("second body")] != 2 {
		t.Errorf("zero state again: rendered One %d, Two %d times, want 2, 2", c.n[in("first body")], c.n[in("second body")])
	}
	// focus is in the key of the focused block only
	final(&State{Focused: one})
	if c.n[in("first body")] != 3 || c.n[in("second body")] != 2 {
		t.Errorf("focus One: rendered One %d, Two %d times, want 3, 2", c.n[in("first body")], c.n[in("second body")])
	}
}

// TestStateKey: entries outside the range and absent focus give "";
// nested keys count by their top-level form.
func TestStateKey(t *testing.T) {
	st := &State{
		Tabs:    map[string]TabsState{"10": {Tabbed: true, Selected: 1}},
		Details: map[string]DetailsState{"10/t0@3": {Folded: true}, "50": {}},
		Trees:   map[string]TreeState{"20": {Closed: map[string]bool{"a/": true, "b/": false}}},
		Focused: "50",
	}
	if k := stateKey(st, 0, 10); k != "" {
		t.Errorf("[0,10): %q", k)
	}
	a, b := stateKey(st, 10, 20), stateKey(st, 10, 21)
	if a == "" || !strings.Contains(a, `"10/t0@3"`) || strings.Contains(a, `"20"`) || !strings.Contains(b, `"a/"`) || strings.Contains(b, `"b/"`) {
		t.Errorf("[10,20): %q; [10,21): %q", a, b)
	}
	if k := stateKey(st, 50, 51); !strings.Contains(k, "f") {
		t.Errorf("focus not in key: %q", k)
	}
	if stateKey(nil, 0, 100) != "" || stateKey(&State{}, 0, 100) != "" {
		t.Error("empty state not empty key")
	}
}

// TestStateMathBeforeForm: a `$$` formula before a form in the same block
// moves the form in the rewritten source; its key is still the offset in
// the source as written (directive.IDs), and a State under that key
// draws.
func TestStateMathBeforeForm(t *testing.T) {
	r := newTestRenderer(t, "dark", 60)
	cases := []string{
		"Intro.\n\n$$ a^2 + b^2 $$\n> [!DETAILS] More\n> hidden body\n\nafter\n",
		"$$\nx = 1\n$$\n> [!DETAILS] More\n> hidden body\n\n```tree\nsrc/\n  a.go\n```\n",
		"Text $$ inline $$ here.\n\n$$ e $$\n$$ f $$\n> [!DETAILS] More\n> hidden body\n",
		// in a body: a tab panel (rewritten with the block) and a callout
		// body (rewritten in the body render)
		"> [!TABS] X\n#### A\n$$ a $$\n> [!DETAILS] More\n> hidden body\n#### B\nb\n",
		"> [!NOTE]\n> $$\n> a\n> $$\n> > [!DETAILS] More\n> > hidden body\n",
	}
	for i, text := range cases {
		var ids []directive.FormID
		moved := false
		for _, b := range blocks.Split(text) {
			bi := directive.IDs(b.Source, b.Offset)
			ids = append(ids, bi...)
			if src, _ := mathFences(b.Source); len(bi) > 0 && len(src) != len(b.Source) {
				moved = true
			}
		}
		key := ""
		want := map[string]bool{}
		for _, id := range ids {
			if id.Interactive() {
				want[id.Key] = true
			}
			if id.Kind == directive.KindDetails && key == "" {
				key = id.Key
			}
		}
		if key == "" || (!moved && !strings.Contains(text, "> $$")) {
			t.Fatalf("case %d: no details form moved by a formula (ids %+v)", i, ids)
		}
		_, marks, err := r.RenderState(text, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, m := range marks {
			got[m.Key] = true
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("case %d: mark keys %v, IDs keys %v", i, got, want)
		}
		st := &State{Details: map[string]DetailsState{key: {Folded: true}}}
		folded, fmarks, err := r.RenderState(text, 0, st)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(folded, "hidden body") {
			t.Errorf("case %d: State at IDs key %s did not fold the details", i, key)
		}
		s := NewStream(r, "k", NewCache(16))
		s.SetState(st)
		out, err := s.Final(text)
		if err != nil {
			t.Fatal(err)
		}
		if out != folded || !reflect.DeepEqual(s.Marks(), fmarks) {
			t.Errorf("case %d: Stream Final differs from RenderState", i)
		}
	}
}

// TestStateFastPaths: a growing fence tail and a growing table tail with
// State set: the fast paths are taken, Final equals RenderState, and the
// form before them draws in its state with its marks in Frame too.
func TestStateFastPaths(t *testing.T) {
	r := newTestRenderer(t, "dark", 78)
	head := "> [!DETAILS] Log\n> body\n\n"
	var code strings.Builder
	code.WriteString("```go\n")
	for i := range 120 {
		fmt.Fprintf(&code, "x%d := %d\n", i, i)
	}
	code.WriteString("```\n")
	var table strings.Builder
	table.WriteString("| a | b |\n|---|---|\n")
	for i := range 60 {
		fmt.Fprintf(&table, "| %d | v%d |\n", i, i)
	}
	for _, tail := range []string{code.String(), table.String()} {
		text := head + tail
		key := directive.IDs(head, 0)[0].Key
		st := &State{Details: map[string]DetailsState{key: {Folded: true}}, Focused: key}
		s := NewStream(r, "k", NewCache(16))
		s.SetState(st)
		for _, p := range corpus.LinePrefixes(text)[3:] {
			got, err := s.Frame(p)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got, "body") {
				t.Fatalf("Frame at %d: details not folded", len(p))
			}
			if len(s.Marks()) != 1 || s.Marks()[0].Key != key {
				t.Fatalf("Frame at %d: marks %+v", len(p), s.Marks())
			}
			fin, err := s.Final(p)
			if err != nil {
				t.Fatal(err)
			}
			want, marks, err := r.RenderState(Sanitize(p), 0, st)
			if err != nil {
				t.Fatal(err)
			}
			if fin != want || !reflect.DeepEqual(s.Marks(), marks) {
				t.Fatalf("Final at %d differs from RenderState", len(p))
			}
		}
		if s.stats.fenceSpliced+s.stats.tableSpliced == 0 {
			t.Errorf("no fast path taken for %.10q", tail)
		}
	}
}
