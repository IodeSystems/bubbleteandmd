package directive

import (
	"strconv"

	"github.com/iodesystems/bubbleteandmd/internal/blocks"
)

// FormID names one form for per-form state (folded or open, selected tab,
// folded tree rows). It is derived from the source text alone, so the same
// form keeps the same ID while the document grows.
type FormID struct {
	// Key is unique per form in a document: the byte offset of the form's
	// first byte; for a form inside a body, the parent's Key, the body's slot
	// and the offset in that body.
	Key string
	// Offset is the byte offset of the form's first byte. Top-level forms:
	// absolute in the document. Nested forms: the parent's Offset plus the
	// offset in the parent's body. A body is dequoted Markdown, so a nested
	// Offset is stable but not an exact position in the document.
	Offset int
	Kind   Kind
	// Parent is the Key of the enclosing form, "" at top level.
	Parent string
	// Panels is the number of tabs, for a tabs form.
	Panels int
	// Paths are the TreeRow.Path of every folder row, for a tree.
	Paths []string
}

// Interactive reports whether the form takes state: a fold, tabs or a tree.
func (f FormID) Interactive() bool {
	return f.Kind == KindDetails || f.Kind == KindTabs || f.Kind == KindTree
}

// IDs returns the forms of one top-level block, the block starting at byte
// offset in the document, in document order, each form before the forms in
// its bodies. Forms are found as the renderer finds them: Parse, then Parse
// again after the form's End. Bodies of callouts, folded sections, tabs,
// steps and pros and cons are read the same way, after blocks.Split.
func IDs(block string, offset int) []FormID {
	return idsIn(block, offset, "", 0)
}

// idsIn reads src, which starts at byte at of its body. parent is the Key of
// the form the body belongs to; base is that form's Offset.
func idsIn(src string, at int, parent string, base int) []FormID {
	var out []FormID
	for pos := 0; ; {
		d, ok := Parse(src[pos:])
		if !ok {
			return out
		}
		rel := at + pos + d.Start
		id := FormID{Key: strconv.Itoa(rel), Offset: base + rel, Kind: d.Kind, Parent: parent}
		if parent != "" {
			id.Key = parent + "@" + id.Key
		}
		if d.Tabs != nil {
			id.Panels = len(d.Tabs.Panels)
		}
		if d.Tree != nil {
			for _, r := range d.Tree.Rows {
				if r.Folder {
					id.Paths = append(id.Paths, r.Path)
				}
			}
		}
		out = append(out, id)
		out = append(out, bodies(d, id)...)
		pos += d.End
	}
}

// bodies reads the Markdown bodies of d, each under its slot.
func bodies(d Directive, id FormID) []FormID {
	var out []FormID
	if d.After != "" {
		out = body(d.After, id, "a")
	}
	switch {
	case d.Callout != nil:
		out = append(out, body(d.Callout.Body, id, "b")...)
	case d.Details != nil:
		out = append(out, body(d.Details.Body, id, "b")...)
	case d.Tabs != nil:
		for i, p := range d.Tabs.Panels {
			out = append(out, body(p.Body, id, "t"+strconv.Itoa(i))...)
		}
	case d.Steps != nil:
		for i, s := range d.Steps.Items {
			out = append(out, body(s.Body, id, "s"+strconv.Itoa(i))...)
		}
	case d.Verdict != nil:
		for i, it := range d.Verdict.Pros {
			out = append(out, body(it.Body, id, "p"+strconv.Itoa(i))...)
		}
		for i, it := range d.Verdict.Cons {
			out = append(out, body(it.Body, id, "c"+strconv.Itoa(i))...)
		}
	}
	return out
}

// body reads the forms in one body, split into blocks as the renderer does.
func body(md string, parent FormID, slot string) []FormID {
	var out []FormID
	for _, b := range blocks.Split(md) {
		out = append(out, idsIn(b.Source, b.Offset, parent.Key+"/"+slot, parent.Offset)...)
	}
	return out
}
