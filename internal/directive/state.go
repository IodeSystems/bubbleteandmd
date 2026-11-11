package directive

// State is the per-form state of one document, keyed by FormID.Key. The
// zero State (and a nil *State) is the layout as streamed: tabs stacked,
// details open, trees open with no fold marks, nothing focused (Carl
// 2026-11-11). A key with no entry draws as the zero state of its kind.
type State struct {
	Tabs    map[string]TabsState
	Details map[string]DetailsState
	// Trees: a tree with an entry draws fold marks on its folders ("▾ "
	// open, "▸ " closed), also when Closed is empty. A tree with no entry
	// draws as streamed.
	Trees map[string]TreeState
	// Focused is the Key of the focused form, "" for none.
	Focused string
}

// TabsState is the state of a tabs form.
type TabsState struct {
	// Tabbed shows one panel, the Selected one, under a "‹i/n› Label"
	// header. Not tabbed: all panels stacked.
	Tabbed bool
	// Selected is the panel index; out of range is clamped.
	Selected int
}

// DetailsState is the state of a details form.
type DetailsState struct {
	Folded bool // only the "▸ Title" line
}

// TreeState is the state of a tree.
type TreeState struct {
	// Closed holds the TreeRow.Path of each closed folder; rows under one
	// are hidden.
	Closed map[string]bool
	// Cursor is the TreeRow.Path of the row focus shows when the tree is
	// focused; "" is the first row. A hidden cursor row shows on its
	// nearest shown folder.
	Cursor string
}

func (s *State) tabs(key string) TabsState {
	if s == nil {
		return TabsState{}
	}
	return s.Tabs[key]
}

func (s *State) details(key string) DetailsState {
	if s == nil {
		return DetailsState{}
	}
	return s.Details[key]
}

func (s *State) tree(key string) (TreeState, bool) {
	if s == nil {
		return TreeState{}, false
	}
	t, ok := s.Trees[key]
	return t, ok
}

func (s *State) focused(key string) bool {
	return s != nil && s.Focused != "" && s.Focused == key
}

// MarkKind is what a Mark points at.
type MarkKind int

const (
	// MarkHeader is a form's header: the labels line (stacked tabs) or the
	// "‹i/n› Label" line (tabbed), the title lines of a details form.
	MarkHeader MarkKind = iota + 1
	// MarkPanel is one drawn tab panel: its header line and its body.
	MarkPanel
	// MarkRow is one drawn tree row.
	MarkRow
)

var markNames = [...]string{"", "header", "panel", "row"}

func (k MarkKind) String() string {
	if k < 0 || int(k) >= len(markNames) {
		return "unknown"
	}
	return markNames[k]
}

// Mark is where an interactive part of a form is drawn.
type Mark struct {
	Key  string // FormID.Key of the form
	Kind MarkKind
	// Line is the first line, counted in the lines or text the mark comes
	// with; Height is the number of lines, at least 1.
	Line, Height int
	// Index is the panel index (MarkPanel; MarkHeader of a tabbed tabs
	// form: the selected one) or the row index in Tree.Rows (MarkRow).
	Index int
	// Path is the TreeRow.Path (MarkRow).
	Path string
}

// place keeps the marks of a text whose first drop lines were dropped and
// whose next kept lines were kept, and moves them to start at line at.
func place(ms []Mark, drop, kept, at int) []Mark {
	var out []Mark
	for _, m := range ms {
		if m.Line < drop || m.Line >= drop+kept {
			continue
		}
		m.Line += at - drop
		out = append(out, m)
	}
	return out
}
