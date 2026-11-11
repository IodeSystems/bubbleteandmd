package bubbleteandmd

import (
	"slices"
	"strconv"
	"strings"

	"github.com/iodesystems/bubbleteandmd/internal/directive"
)

// State is the per-form state of one document (folded details, selected
// tab, closed tree folders, the focused form), keyed by form key: the byte
// offset of a top-level form in the document, "<parent key>/<slot>@<offset>"
// for a form in a body. The zero State and a nil *State draw the layout as
// streamed (Carl 2026-11-11). Only the host changes it.
type State = directive.State

// TabsState, DetailsState and TreeState are the state of one form.
type (
	TabsState    = directive.TabsState
	DetailsState = directive.DetailsState
	TreeState    = directive.TreeState
)

// Mark is where an interactive part of a form is drawn: the form's Key,
// what part (MarkKind), its first line and its height in lines.
type Mark = directive.Mark

// MarkKind is what a Mark points at.
type MarkKind = directive.MarkKind

const (
	MarkHeader = directive.MarkHeader
	MarkPanel  = directive.MarkPanel
	MarkRow    = directive.MarkRow
)

// StateRenderer is an optional interface of a Renderer: it draws each form
// in its State and tells where the interactive parts are. RenderState(md,
// offset, nil) returns what Render(md) returns. offset is the document
// offset of md's text (after join's sentinel, when md has it), so form keys
// are document keys; mark lines are counted in the output.
// *DirectiveRenderer implements it.
type StateRenderer interface {
	Renderer
	RenderState(md string, offset int, st *State) (string, []Mark, error)
}

// stateKey is the part of st that can change the render of the text at
// document bytes [lo, hi): the entries whose top-level form starts there,
// and the focus when it does. "" when there is none. Keys are sorted, so
// equal state gives an equal string.
func stateKey(st *State, lo, hi int) string {
	if st == nil {
		return ""
	}
	in := func(key string) bool {
		root, _, _ := strings.Cut(key, "/")
		n, err := strconv.Atoi(root)
		return err == nil && n >= lo && n < hi
	}
	var b strings.Builder
	for _, k := range sortedKeys(st.Tabs) {
		if in(k) {
			v := st.Tabs[k]
			b.WriteString("t" + strconv.Quote(k) + strconv.FormatBool(v.Tabbed) + strconv.Itoa(v.Selected) + ";")
		}
	}
	for _, k := range sortedKeys(st.Details) {
		if in(k) {
			b.WriteString("d" + strconv.Quote(k) + strconv.FormatBool(st.Details[k].Folded) + ";")
		}
	}
	for _, k := range sortedKeys(st.Trees) {
		if in(k) {
			v := st.Trees[k]
			b.WriteString("r" + strconv.Quote(k) + strconv.Quote(v.Cursor))
			for _, p := range sortedKeys(v.Closed) {
				if v.Closed[p] {
					b.WriteString(strconv.Quote(p))
				}
			}
			b.WriteString(";")
		}
	}
	if st.Focused != "" && in(st.Focused) {
		b.WriteString("f" + strconv.Quote(st.Focused))
	}
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}
