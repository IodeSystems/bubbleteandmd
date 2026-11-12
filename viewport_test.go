package bubbleteandmd

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// longReply: paragraphs, so lines above the tail do not change as it grows.
func longReply(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "Paragraph %d has a few words in it.\n\n", i)
	}
	return b.String()
}

// window is lines [off, off+h) of out.
func window(out string, off, h int) string {
	return out[lineStart(out, off):lineStart(out, off+h)]
}

// TestDocViewportWindow: with a height, Frame and Final return the slice
// of the whole output at Offset, and a view never scrolled follows the
// bottom when the first frame fits, else stays at the top (h1: the first
// frame is 3 lines).
func TestDocViewportWindow(t *testing.T) {
	docs := []invCase{{"reply", docReply}, {"long", longReply(30)}}
	for _, style := range []string{"dark", "notty"} {
		r := newTestRenderer(t, style, 50)
		for _, c := range docs {
			for _, h := range []int{1, 5, 12} {
				d := NewDoc(r, style, NewCache(64))
				d.SetHeight(h)
				s := NewStream(r, style, NewCache(64))
				top := -1 // first frame taller than h: 1
				check := func(at string, got, full string) {
					t.Helper()
					n := outLines(full)
					if top < 0 {
						top = 0
						if n > h {
							top = 1
						}
					}
					want := max(n-h, 0)
					if top == 1 {
						want = 0
					}
					if d.Lines() != n || d.Offset() != want {
						t.Fatalf("%s/%s/h%d %s: offset %d lines %d, want %d", c.name, style, h, at, d.Offset(), d.Lines(), want)
					}
					if want := window(full, d.Offset(), h); got != want || d.View() != want {
						t.Fatalf("%s/%s/h%d %s: %s", c.name, style, h, at, firstDiff(want, got))
					}
					if outLines(got) != min(h, n) {
						t.Fatalf("%s/%s/h%d %s: %d lines", c.name, style, h, at, outLines(got))
					}
				}
				for _, p := range corpus.LinePrefixes(c.text) {
					got, err := d.Frame(p)
					if err != nil {
						t.Fatal(err)
					}
					full, _ := s.Frame(p)
					check(fmt.Sprintf("Frame %d", len(p)), got, full)
				}
				got := docFinal(t, d, c.text)
				full, _ := s.Final(c.text)
				check("Final", got, full)
				d.SetHeight(0)
				if d.View() != full || d.Offset() != 0 {
					t.Fatalf("%s/%s: SetHeight(0) view is not the whole output", c.name, style)
				}
			}
		}
	}
}

// TestDocViewportFollow: a view at the bottom stays there as the reply
// grows; a view scrolled up stays put and its lines do not change; back
// at the bottom it follows again.
func TestDocViewportFollow(t *testing.T) {
	const h = 6
	src := longReply(40)
	prefixes := corpus.LinePrefixes(src)
	d := newTestDoc(t, "dark", 50)
	d.SetHeight(h)
	i := 0
	for ; d.Lines() < 4*h; i++ {
		if _, err := d.Frame(prefixes[i]); err != nil {
			t.Fatal(err)
		}
	}
	if d.Offset() != d.Lines()-h {
		t.Fatalf("not at the bottom: %d of %d", d.Offset(), d.Lines())
	}
	for range 2 * h {
		if !d.Do(ActionScrollUp) {
			t.Fatal("↑ not consumed")
		}
	}
	off, view := d.Offset(), d.View()
	if off != d.Lines()-3*h {
		t.Fatalf("scrolled to %d of %d", off, d.Lines())
	}
	for ; i < len(prefixes); i++ {
		got, err := d.Frame(prefixes[i])
		if err != nil {
			t.Fatal(err)
		}
		if d.Offset() != off || got != view {
			t.Fatalf("scrolled-up view moved at %d: offset %d→%d: %s", i, off, d.Offset(), firstDiff(view, got))
		}
	}
	if !d.Do(ActionBottom) || d.Offset() != d.Lines()-h {
		t.Fatal("end: not at the bottom")
	}
	n := d.Lines()
	if _, err := d.Final(src + "One more.\n\nAnd more.\n"); err != nil {
		t.Fatal(err)
	}
	if d.Lines() <= n || d.Offset() != d.Lines()-h {
		t.Fatalf("at the bottom, not followed: %d of %d (was %d)", d.Offset(), d.Lines(), n)
	}
}

// TestDocViewportFocus: a focus change scrolls the focused stop into view;
// a stop taller than the view shows its top; a tree cursor move shows the
// cursor row.
func TestDocViewportFocus(t *testing.T) {
	for _, style := range docStyles {
		for _, h := range []int{2, 4} {
			d := newTestDoc(t, style, 60)
			d.SetHeight(h)
			docFinal(t, d, docReply)
			if d.Lines() <= h || d.Offset() != 0 {
				t.Fatalf("%s/h%d: %d lines, offset %d: want taller than h, open at the top", style, h, d.Lines(), d.Offset())
			}
			if !d.Focus() {
				t.Fatal("Focus")
			}
			for {
				docFinal(t, d, docReply)
				s, _ := d.Focused()
				if s.Height > h {
					if d.Offset() != s.Line {
						t.Errorf("%s/h%d: tall %s stop at %d, offset %d", style, h, s.Kind, s.Line, d.Offset())
					}
				} else if s.Line < d.Offset() || s.Line+s.Height > d.Offset()+h {
					t.Errorf("%s/h%d: %s stop %d+%d not in view at %d", style, h, s.Kind, s.Line, s.Height, d.Offset())
				}
				if s.Kind == StopTree {
					for range 3 {
						if !d.Do(ActionRight) {
							t.Fatal("→ on tree")
						}
						docFinal(t, d, docReply)
						cur := d.State().Trees[s.Key].Cursor
						for _, r := range d.rows[s.Key] {
							if r.Path == cur && (r.Line < d.Offset() || r.Line >= d.Offset()+h) {
								t.Errorf("%s/h%d: cursor row %d not in view at %d", style, h, r.Line, d.Offset())
							}
						}
					}
				}
				if !d.Do(ActionNext) {
					break
				}
			}
			// scrolled away and back: FocusLast scrolls to the last stop.
			d.Do(ActionTop)
			d.FocusLast()
			docFinal(t, d, docReply)
			if s, _ := d.Focused(); s.Line < d.Offset() || s.Line >= d.Offset()+h {
				t.Errorf("%s/h%d: FocusLast stop %d not in view at %d", style, h, s.Line, d.Offset())
			}
		}
	}
}

// TestDocViewportKeys: the scroll keys are consumed only with a height,
// with or without focus, and at an edge.
func TestDocViewportKeys(t *testing.T) {
	km := DefaultKeyMap()
	key := func(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }
	up, down, pgup, pgdown, home, end := key(tea.KeyUp), key(tea.KeyDown), key(tea.KeyPgUp), key(tea.KeyPgDown), key(tea.KeyHome), key(tea.KeyEnd)
	src := longReply(20)

	d := newTestDoc(t, "dark", 50)
	full := docFinal(t, d, src)
	for _, k := range []tea.KeyPressMsg{up, down, pgup, pgdown, home, end} {
		if d.HandleKey(k, km) {
			t.Errorf("height 0: %s consumed", k.String())
		}
	}
	if d.View() != full {
		t.Error("height 0: view is not the whole output")
	}

	const h = 5
	d.SetHeight(h)
	docFinal(t, d, src)
	bottom := d.Lines() - h
	steps := []struct {
		k    tea.KeyPressMsg
		want int
	}{
		{home, 0}, {up, 0}, {pgup, 0}, {down, 1}, {pgdown, 1 + h}, {up, h}, {end, bottom},
		{down, bottom}, {pgdown, bottom}, {pgup, bottom - h}, {end, bottom},
	}
	for _, s := range steps {
		if !d.HandleKey(s.k, km) {
			t.Errorf("%s not consumed", s.k.String())
		}
		if d.Offset() != s.want {
			t.Errorf("%s: offset %d, want %d", s.k.String(), d.Offset(), s.want)
		}
		if want := window(full, d.Offset(), h); d.View() != want {
			t.Errorf("%s: view %s", s.k.String(), firstDiff(want, d.View()))
		}
	}
	// focused: scroll keys still scroll, and do not blur.
	d = newTestDoc(t, "dark", 60)
	d.SetHeight(3)
	docFinal(t, d, docReply)
	d.Focus()
	if !d.HandleKey(home, km) || d.Offset() != 0 || d.State().Focused == "" {
		t.Error("focused: home")
	}
	// fits in the view: consumed, nothing moves.
	d = newTestDoc(t, "dark", 50)
	d.SetHeight(100)
	docFinal(t, d, "Short.\n")
	if !d.HandleKey(down, km) || d.Offset() != 0 {
		t.Error("short reply: ↓")
	}
}

// TestDocSetHeight: a view at the bottom stays at the bottom when the
// height changes; from height 0 the view starts at the top.
func TestDocSetHeight(t *testing.T) {
	d := newTestDoc(t, "dark", 50)
	docFinal(t, d, longReply(20))
	d.SetHeight(5)
	if d.Offset() != 0 {
		t.Errorf("from 0: offset %d", d.Offset())
	}
	d.Do(ActionBottom)
	d.SetHeight(8)
	if d.Offset() != d.Lines()-8 {
		t.Errorf("at bottom, taller: offset %d of %d", d.Offset(), d.Lines())
	}
	d.Do(ActionTop)
	d.Do(ActionScrollDown)
	d.SetHeight(4)
	if d.Offset() != 1 {
		t.Errorf("not at bottom: offset %d", d.Offset())
	}
	d.SetHeight(-1)
	if d.Height() != 0 || d.Offset() != 0 {
		t.Error("negative height")
	}
}

// TestDocViewportOpen: the view opens on its first output at the top when
// that output is taller than the height, at the bottom when it fits; the
// same with SetHeight before or after Final (audit 2026-11-11: before, a
// SetHeight before the first render opened at the bottom).
func TestDocViewportOpen(t *testing.T) {
	const h = 5
	for _, src := range []string{longReply(20), "Short.\n"} {
		before := newTestDoc(t, "dark", 50)
		before.SetHeight(h)
		docFinal(t, before, src)
		after := newTestDoc(t, "dark", 50)
		docFinal(t, after, src)
		after.SetHeight(h)
		for _, d := range []*Doc{before, after} {
			if d.Offset() != 0 {
				t.Errorf("%d lines: offset %d, want 0", d.Lines(), d.Offset())
			}
			// a later render does not move a view that opened at the top.
			docFinal(t, d, src+"More.\n")
			if want := 0; d.Lines() > h && d.Offset() != want {
				t.Errorf("%d lines, re-render: offset %d, want %d", d.Lines(), d.Offset(), want)
			}
		}
		if before.View() != after.View() {
			t.Errorf("order changes the view: %s", firstDiff(after.View(), before.View()))
		}
	}
	// streaming: the first frames fit, so the view follows the bottom.
	d := newTestDoc(t, "dark", 50)
	d.SetHeight(h)
	for _, p := range corpus.LinePrefixes(longReply(20)) {
		if _, err := d.Frame(p); err != nil {
			t.Fatal(err)
		}
	}
	if d.Lines() <= h || d.Offset() != d.Lines()-h {
		t.Errorf("stream: offset %d of %d, want the bottom", d.Offset(), d.Lines())
	}
	// back to 0 and to h again: opens again on the current output (top).
	d.SetHeight(0)
	d.SetHeight(h)
	if d.Offset() != 0 {
		t.Errorf("0 → h: offset %d", d.Offset())
	}
}

// TestDocViewportScrollWinsReveal: a scroll between a focus change and the
// next render cancels the pending reveal; the user's scroll wins.
func TestDocViewportScrollWinsReveal(t *testing.T) {
	d := newTestDoc(t, "dark", 60)
	d.SetHeight(2)
	docFinal(t, d, docReply)
	d.Focus()
	d.FocusLast() // the last stop, far below the top
	if !d.Do(ActionTop) {
		t.Fatal("home not consumed")
	}
	docFinal(t, d, docReply)
	s, _ := d.Focused()
	if d.Offset() != 0 || s.Line < 2 {
		t.Errorf("offset %d (stop at %d), want 0: the scroll is kept", d.Offset(), s.Line)
	}
	// with no scroll between, the same focus change reveals the stop.
	d.FocusLast()
	docFinal(t, d, docReply)
	if d.Offset() == 0 {
		t.Error("FocusLast with no scroll: stop not revealed")
	}
}
