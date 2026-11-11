// Package directive reads llx-markdown's directive forms out of one top-level
// block: callouts, steps, tabs, stats, pros and cons, folded sections, sources,
// quotes with a source, file trees and titled code fences. It parses only; it
// does not render. Bodies stay Markdown source, for the renderer to render again.
//
// Ported from llx-markdown packages/llx-markdown/src/dialect.ts,
// src/core/remark-answer-containers.ts and src/core/remark-answer-blocks.ts
// (MIT, see NOTICE). Offsets are byte offsets, not UTF-16 units.
package directive

// Kind names a directive form.
type Kind int

const (
	KindNone Kind = iota
	KindCallout
	KindSteps
	KindTabs
	KindStats
	KindVerdict
	KindDetails
	KindSources
	KindAttribution
	KindTree
	KindCodeTitle
)

var kindNames = [...]string{"none", "callout", "steps", "tabs", "stats", "verdict", "details", "sources", "attribution", "tree", "code-title"}

func (k Kind) String() string {
	if k < 0 || int(k) >= len(kindNames) {
		return "unknown"
	}
	return kindNames[k]
}

// CalloutKinds are GitHub's alert kinds, as llx-markdown reads them.
var CalloutKinds = []string{"note", "tip", "important", "warning", "caution"}

// ContainerKinds are llx-markdown's container forms.
var ContainerKinds = []string{"steps", "tabs", "stats", "pros", "cons", "details", "sources"}

// Directive is one form found in a block.
type Directive struct {
	Kind Kind
	// Title is the form's title: the marker line's text, a code fence's file
	// name, or the first side's title of a verdict. Empty for none.
	Title string
	// Start and End are the byte range of the form in the block, marker
	// included. Text before Start and from End on is ordinary Markdown.
	Start, End int
	// After is Markdown quoted with a list form after its list (`> [!STEPS]`,
	// `> 1. …`, `>`, `> more`). It belongs to the form and renders after it.
	After string

	// Exactly one of these is set, the one Kind names.
	Callout     *Callout
	Steps       *Steps
	Tabs        *Tabs
	Stats       *Stats
	Verdict     *Verdict
	Details     *Details
	Sources     *Sources
	Attribution *Attribution
	Tree        *Tree
	CodeTitle   *CodeTitle
}

// Callout is `> [!WARNING] Title` over a quoted body.
type Callout struct {
	Kind  string // one of CalloutKinds
	Title string // plain text; empty when none or when the marker line carries formatting
	Body  string // Markdown
}

// Steps is `> [!STEPS] Title` over a list.
type Steps struct {
	Title   string
	Ordered bool
	Start   int // first number of an ordered list
	Items   []Step
}

// Step is one stage. An item that does not open with bold has only Body.
type Step struct {
	Title string // inline Markdown inside the bold lead
	When  string // inline Markdown inside the emphasis right after it
	Body  string // Markdown after the head
}

// Tabs is `> [!TABS] Title` over headed sections, one panel per heading.
type Tabs struct {
	Title  string
	Panels []Panel
}

// Panel is one tab.
type Panel struct {
	Label string // the heading's text without formatting; its number when empty
	Body  string // Markdown
}

// Stats is `> [!STATS] Title` over a list of figures.
type Stats struct {
	Title string
	Items []Stat
}

// Stat is one figure. An item that does not open with bold has only Label.
type Stat struct {
	Value string // inline Markdown inside the bold lead
	Label string // inline Markdown after it
}

// Verdict is `> [!PROS]`, `> [!CONS]`, or the two one after the other.
// Pros or Cons is nil for a side not written.
type Verdict struct {
	ProsTitle, ConsTitle string
	Pros, Cons           []Item
	ConsFirst            bool // the cons were written above the pros
}

// Item is one list item.
type Item struct {
	Body string // Markdown
}

// Details is `> [!DETAILS] Title` over a quoted body, folded.
type Details struct {
	Title string // empty when none or when the marker line carries formatting
	Body  string // Markdown
}

// Sources is `> [!SOURCES] Title` over a list of links.
type Sources struct {
	Title string
	Items []Source
}

// Source is one reference. An item that does not open with a link has only
// Note; a relative link has no Host.
type Source struct {
	Title string // the link's label as inline Markdown; the site for a bare URL
	URL   string
	Host  string // the site, `www.` dropped
	Note  string // inline Markdown after the link
}

// Attribution is a quote whose last line starts with `— ` or `-- `.
type Attribution struct {
	Quote  string // Markdown
	Source string // inline Markdown after the dash
}

// Tree is a ```tree fence.
type Tree struct {
	Rows []TreeRow
}

// CodeTitle is a code fence that names its file.
type CodeTitle struct {
	Title string
	Lang  string
	Body  string // the code, verbatim
}
