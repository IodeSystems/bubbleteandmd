package bubbleteandmd

import (
	"slices"
	"sort"
)

// Doc is one reply with interactive forms: a Stream that draws in a State
// the Doc holds, the stops (forms that take keys) of its last output, and
// the actions that change the State. It is not a tea.Model: Do and
// HandleKey report whether the key was consumed, and the host handles a key
// that was not (Carl 2026-11-11).
//
// The layout stays as streamed until an action changes it: Frame and Final
// draw the zero State, and the end of the stream changes nothing. Focus
// alone changes no line count or width (a same-width cue only). After an
// action that returns true, render again (Frame or Final with the same
// source) to show it; Stops and Focused are those of the last render.
//
// A Doc is not safe for concurrent use.
type Doc struct {
	s  *Stream
	st State
	// stops: of the last output; rows: drawn tree rows per key, in order.
	stops []Stop
	rows  map[string][]Mark
	// last: the key focused before Blur, which Focus returns to.
	last string
}

// StopKind is the kind of form a Stop is.
type StopKind int

const (
	StopDetails StopKind = iota + 1
	StopTabs
	StopTree
)

var stopNames = [...]string{"", "details", "tabs", "tree"}

func (k StopKind) String() string {
	if k < 0 || int(k) >= len(stopNames) {
		return "unknown"
	}
	return stopNames[k]
}

// Stop is one form that takes keys, as drawn in the last output: Key is
// its State key; Line and Height are the lines of its marks (details: the
// title lines; tabs: the header and the drawn panels; tree: the drawn
// rows), counted in that output.
type Stop struct {
	Key          string
	Kind         StopKind
	Line, Height int
}

// Action is a named thing a key does to a Doc.
type Action string

const (
	// ActionNext and ActionPrev move the focus to the next or previous
	// stop. Past the last (first) stop the Doc blurs and the action is not
	// consumed, so the host moves its own focus on.
	ActionNext Action = "focus.next"
	ActionPrev Action = "focus.prev"
	// ActionToggle folds or opens a details form or the tree folder at the
	// cursor, or switches tabs between stacked and one-tab view.
	ActionToggle Action = "toggle"
	// ActionLeft and ActionRight: tabs, the previous or next panel (from
	// stacked, the first press switches to one-tab view at the selected
	// panel); tree, the cursor to the previous or next drawn folder. Both
	// wrap.
	ActionLeft  Action = "move.left"
	ActionRight Action = "move.right"
	// ActionLeave blurs the Doc and is not consumed: the host still does
	// the key's job (esc).
	ActionLeave Action = "focus.leave"
)

// Actions is every Action, in the order a key is looked up and a help list
// shows them.
var Actions = []Action{ActionNext, ActionPrev, ActionToggle, ActionLeft, ActionRight, ActionLeave}

// NewDoc returns a Doc that renders through NewStream(r, key, c). r should
// be a StateRenderer (*DirectiveRenderer); with any other Renderer the Doc
// has no stops.
func NewDoc(r Renderer, key string, c *Cache) *Doc {
	d := &Doc{s: NewStream(r, key, c)}
	d.s.SetState(&d.st)
	return d
}

// Frame is Stream.Frame in the Doc's State; it updates the stops.
func (d *Doc) Frame(src string) (string, error) { return d.after(d.s.Frame(src)) }

// Final is Stream.Final in the Doc's State; it updates the stops.
func (d *Doc) Final(src string) (string, error) { return d.after(d.s.Final(src)) }

func (d *Doc) after(out string, err error) (string, error) {
	if err != nil {
		d.stops, d.rows = nil, nil
		return out, err
	}
	d.stops, d.rows = stopsOf(d.s.Marks())
	return out, nil
}

// stopsOf groups marks by form key into stops, in line order (the order
// the renderer drew them for equal lines: side-by-side columns).
func stopsOf(marks []Mark) ([]Stop, map[string][]Mark) {
	var stops []Stop
	at := map[string]int{}
	rows := map[string][]Mark{}
	for _, m := range marks {
		kind := StopDetails
		switch {
		case m.Kind == MarkRow:
			kind = StopTree
			rows[m.Key] = append(rows[m.Key], m)
		case m.Panels > 0:
			kind = StopTabs
		}
		i, ok := at[m.Key]
		if !ok {
			at[m.Key] = len(stops)
			stops = append(stops, Stop{Key: m.Key, Kind: kind, Line: m.Line, Height: m.Height})
			continue
		}
		s := &stops[i]
		end := max(s.Line+s.Height, m.Line+m.Height)
		s.Line = min(s.Line, m.Line)
		s.Height = end - s.Line
	}
	sort.SliceStable(stops, func(i, j int) bool { return stops[i].Line < stops[j].Line })
	return stops, rows
}

// Stops returns the stops of the last output, in focus order. The host
// scrolls with their lines (height 0: the host owns the scroll).
func (d *Doc) Stops() []Stop { return d.stops }

// Focused returns the focused stop of the last output, if any.
func (d *Doc) Focused() (Stop, bool) {
	if i := d.index(d.st.Focused); i >= 0 {
		return d.stops[i], true
	}
	return Stop{}, false
}

// State returns the Doc's State. The host may read it (State.Focused in
// notty, where focus has no colour); a change to it shows on the next
// render.
func (d *Doc) State() *State { return &d.st }

func (d *Doc) index(key string) int {
	if key == "" {
		return -1
	}
	return slices.IndexFunc(d.stops, func(s Stop) bool { return s.Key == key })
}

// Focus focuses the stop focused before the last Blur when it is still a
// stop, else the first. False when there is no stop.
func (d *Doc) Focus() bool {
	if len(d.stops) == 0 {
		return false
	}
	if d.st.Focused != "" && d.index(d.st.Focused) >= 0 {
		return true
	}
	key := d.stops[0].Key
	if d.index(d.last) >= 0 {
		key = d.last
	}
	d.st.Focused = key
	return true
}

// FocusLast focuses the last stop (the host's focus arrives from below).
// False when there is no stop.
func (d *Doc) FocusLast() bool {
	if len(d.stops) == 0 {
		return false
	}
	d.st.Focused = d.stops[len(d.stops)-1].Key
	return true
}

// Blur takes the focus away; Focus returns to the same stop.
func (d *Doc) Blur() {
	if d.st.Focused != "" {
		d.last = d.st.Focused
	}
	d.st.Focused = ""
}

// Reset clears all state: the layout as streamed, nothing focused.
func (d *Doc) Reset() {
	d.st = State{}
	d.last = ""
}

// Do applies an action to the focused stop and reports whether it was
// consumed. With no focus nothing is consumed (the host calls Focus to give
// the Doc the keys). An action that leaves the Doc (ActionNext past the
// last stop, ActionPrev before the first, ActionLeave) blurs it and returns
// false, so the host still acts on the key.
func (d *Doc) Do(a Action) bool {
	if d.st.Focused == "" {
		return false
	}
	i := d.index(d.st.Focused)
	switch a {
	case ActionNext, ActionPrev:
		j := i + 1
		if a == ActionPrev {
			j = i - 1
		}
		if i < 0 { // focused form not drawn: start again at an edge
			j = 0
			if a == ActionPrev {
				j = len(d.stops) - 1
			}
		}
		if j < 0 || j >= len(d.stops) {
			d.Blur()
			return false
		}
		d.st.Focused = d.stops[j].Key
		return true
	case ActionLeave:
		d.Blur()
		return false
	}
	if i < 0 {
		return false
	}
	s := d.stops[i]
	switch s.Kind {
	case StopDetails:
		if a != ActionToggle {
			return false
		}
		if d.st.Details == nil {
			d.st.Details = map[string]DetailsState{}
		}
		v := d.st.Details[s.Key]
		v.Folded = !v.Folded
		d.st.Details[s.Key] = v
		return true
	case StopTabs:
		return d.tabs(s.Key, a)
	case StopTree:
		return d.tree(s.Key, a)
	}
	return false
}

func (d *Doc) tabs(key string, a Action) bool {
	n := 0
	for _, m := range d.s.Marks() {
		if m.Key == key && m.Panels > 0 {
			n = m.Panels
			break
		}
	}
	if n == 0 {
		return false
	}
	if d.st.Tabs == nil {
		d.st.Tabs = map[string]TabsState{}
	}
	v := d.st.Tabs[key]
	v.Selected = min(max(v.Selected, 0), n-1)
	switch a {
	case ActionToggle:
		v.Tabbed = !v.Tabbed
	case ActionLeft, ActionRight:
		if !v.Tabbed {
			v.Tabbed = true // the first press shows the selected panel
		} else if a == ActionRight {
			v.Selected = (v.Selected + 1) % n
		} else {
			v.Selected = (v.Selected - 1 + n) % n
		}
	default:
		return false
	}
	d.st.Tabs[key] = v
	return true
}

// tree: ←/→ move the cursor over the drawn folder rows (wrapping), enter
// opens or closes the folder at the cursor. The cursor is the State's
// Cursor path; "" is the first row, as the renderer reads it.
func (d *Doc) tree(key string, a Action) bool {
	rows := d.rows[key]
	var folders []Mark
	for _, r := range rows {
		if r.Folder {
			folders = append(folders, r)
		}
	}
	if len(rows) == 0 || len(folders) == 0 {
		return false
	}
	v := d.st.Trees[key]
	cur := 0 // index in rows
	for i, r := range rows {
		if r.Path == v.Cursor {
			cur = i
			break
		}
	}
	switch a {
	case ActionToggle:
		if !rows[cur].Folder {
			return false
		}
		closed := make(map[string]bool, len(v.Closed)+1)
		for p, c := range v.Closed {
			if c {
				closed[p] = true
			}
		}
		p := rows[cur].Path
		if closed[p] {
			delete(closed, p)
		} else {
			closed[p] = true
		}
		v.Closed, v.Cursor = closed, p
	case ActionLeft, ActionRight:
		// right: the first folder below the cursor, else the first;
		// left: the last folder above it, else the last.
		line := rows[cur].Line
		j := slices.IndexFunc(folders, func(f Mark) bool { return f.Line > line })
		if a == ActionRight && j < 0 {
			j = 0
		}
		if a == ActionLeft {
			j = len(folders) - 1
			for j > 0 && folders[j].Line >= line {
				j--
			}
			if folders[j].Line >= line {
				j = len(folders) - 1
			}
		}
		v.Cursor = folders[j].Path
	default:
		return false
	}
	if d.st.Trees == nil {
		d.st.Trees = map[string]TreeState{}
	}
	d.st.Trees[key] = v
	return true
}
