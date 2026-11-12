package bubbleteandmd

import "strings"

// Viewport mode (layer 3 L5b, Carl 2026-11-11). Height 0 (the default) is
// unconstrained: Frame and Final return the whole output, the scroll
// actions are not consumed, and the host scrolls with Stops. Height h > 0:
// the Doc owns the scroll. Frame, Final and View return only lines
// [Offset, Offset+h) of the output, and the scroll actions move Offset.

// SetHeight sets the number of lines the Doc shows (0 or less: all of
// them). A view at the bottom stays at the bottom. The view opens on the
// first output it shows: at the top when the output is taller than h,
// else at the bottom (it then follows a stream as it grows). That output
// is the current one when SetHeight comes after a render, else the next
// render's, so the order of SetHeight and Final does not matter. Width is
// the host's: for a new width make a new Renderer and Doc and copy the
// State (Doc.State).
func (d *Doc) SetHeight(h int) {
	h = max(h, 0)
	bottom := d.h > 0 && d.off >= d.maxOff()
	if d.h == 0 || h == 0 {
		// from height 0 onto an output: it opens now, at the top (below:
		// off stays 0), which is also the bottom when it fits
		d.opened = h > 0 && d.out != ""
	}
	d.h = h
	switch {
	case h == 0:
		d.off = 0
	case bottom:
		d.off = d.maxOff()
	default:
		d.off = min(d.off, d.maxOff())
	}
}

// Height is the height set by SetHeight (0: unconstrained).
func (d *Doc) Height() int { return d.h }

// Offset is the first output line the view shows (0 when the height is 0).
// Stop lines are counted in the whole output: a stop shows when its lines
// are in [Offset, Offset+Height).
func (d *Doc) Offset() int { return d.off }

// Lines is the number of lines in the last output (all of it, not the
// view).
func (d *Doc) Lines() int { return d.total }

// View returns the view of the last output: all of it at height 0, else
// lines [Offset, Offset+Height), a substring of the output (fewer lines
// when the output is shorter). A scroll action changes it with no new
// render.
func (d *Doc) View() string {
	if d.h == 0 {
		return d.out
	}
	return d.out[lineStart(d.out, d.off):lineStart(d.out, d.off+d.h)]
}

// reveal is what a render scrolls into view.
type reveal int

const (
	revealNone   reveal = iota
	revealStop          // the focused stop (focus changed)
	revealCursor        // the tree row at the cursor (cursor moved)
)

// place takes a new output: a view at the bottom stays at the bottom (the
// reply grows under a view that is not, and nothing in it moves); after a
// focus change the focused stop is scrolled into view. The first output
// at a height opens the view: at the top when it is taller than the
// height, else at the bottom.
func (d *Doc) place(out string) {
	bottom := d.off >= d.maxOff()
	d.out, d.total = out, outLines(out)
	if d.h == 0 {
		d.reveal = revealNone
		return
	}
	if !d.opened {
		bottom = d.total <= d.h
		d.opened = true
	}
	if bottom {
		d.off = d.maxOff()
	}
	if d.reveal != revealNone {
		if line, n, ok := d.target(d.reveal); ok {
			d.show(line, n)
		}
		d.reveal = revealNone
	}
	d.off = min(max(d.off, 0), d.maxOff())
}

// target is the lines to show: the focused stop, or for revealCursor the
// row at the focused tree's cursor.
func (d *Doc) target(r reveal) (line, n int, ok bool) {
	s, ok := d.Focused()
	if !ok {
		return 0, 0, false
	}
	if rows := d.rows[s.Key]; r == revealCursor && s.Kind == StopTree && len(rows) > 0 {
		row := rows[cursorRow(rows, d.st.Trees[s.Key].Cursor)]
		return row.Line, row.Height, true
	}
	return s.Line, s.Height, true
}

// show scrolls the least that brings lines [line, line+n) into view; when
// they do not fit, their top line is the view's top.
func (d *Doc) show(line, n int) {
	switch {
	case line < d.off:
		d.off = line
	case line+n > d.off+d.h:
		d.off = min(line, line+n-d.h)
	}
}

// scroll does a scroll action. At an edge it changes nothing.
func (d *Doc) scroll(a Action) {
	switch a {
	case ActionScrollUp:
		d.off--
	case ActionScrollDown:
		d.off++
	case ActionPageUp:
		d.off -= d.h
	case ActionPageDown:
		d.off += d.h
	case ActionTop:
		d.off = 0
	case ActionBottom:
		d.off = d.maxOff()
	}
	d.off = min(max(d.off, 0), d.maxOff())
	// A reveal still pending (focus changed, no render since) is dropped:
	// the user's own scroll wins.
	d.reveal = revealNone
}

func isScroll(a Action) bool {
	switch a {
	case ActionScrollUp, ActionScrollDown, ActionPageUp, ActionPageDown, ActionTop, ActionBottom:
		return true
	}
	return false
}

func (d *Doc) maxOff() int { return max(d.total-d.h, 0) }

// outLines is the number of lines in s: a last line with no "\n" counts.
func outLines(s string) int {
	n := strings.Count(s, "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// lineStart is the byte offset of line i of s (len(s) past the last).
func lineStart(s string, i int) int {
	at := 0
	for ; i > 0; i-- {
		j := strings.IndexByte(s[at:], '\n')
		if j < 0 {
			return len(s)
		}
		at += j + 1
	}
	return at
}
