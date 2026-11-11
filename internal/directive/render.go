package directive

import (
	"errors"
	"fmt"
	"strings"
)

// Ctx is what a form renderer gets.
type Ctx struct {
	// Width is the content width in cells: no margin, no pad.
	Width int
	Theme Theme
	// Inner renders a Markdown body through the full renderer at the given
	// content width and returns unframed lines (no margin, no pad), each at
	// most width cells.
	Inner func(md string, width int) (string, error)
	// Body is Inner for a form body, the text IDs reads forms in: slot names
	// the body as FormID keys do ("a" After, "b" callout or details body,
	// "t<i>" tab panel, "s<i>" step, "p<i>"/"c<i>" pros/cons item). Forms in
	// it take their state from State under Key+"/"+slot+"@"+offset, and
	// their marks come back, lines counted in the returned text. Nil: Inner
	// is used, the body's forms draw as streamed and give no marks.
	Body func(md string, width int, slot string) (string, []Mark, error)
	// State is the document's form state; nil draws every form as streamed.
	State *State
	// Key is this form's FormID.Key.
	Key string
}

// body renders a form body through Body, or Inner when Body is nil.
func (c Ctx) body(md string, width int, slot string) (string, []Mark, error) {
	if c.Body != nil {
		return c.Body(md, width, slot)
	}
	s, err := c.Inner(md, width)
	return s, nil, err
}

// errNotImplemented is returned by a form with no renderer yet.
var errNotImplemented = errors.New("not implemented")

// Render draws one directive as content lines, each at most c.Width cells,
// with no margin and no pad. Frame adds those. The Markdown quoted after a
// list form (Directive.After) is drawn below it, after a blank line. marks
// are the interactive parts drawn, lines counted in lines, nested forms'
// marks included.
func Render(d Directive, c Ctx) (lines []string, marks []Mark, err error) {
	switch d.Kind {
	case KindCallout:
		lines, marks, err = renderCallout(d, c)
	case KindDetails:
		lines, marks, err = renderDetails(d, c)
	case KindAttribution:
		lines, err = renderAttribution(d, c)
	case KindSteps:
		lines, marks, err = renderSteps(d, c)
	case KindStats:
		lines, err = renderStats(d, c)
	case KindSources:
		lines, err = renderSources(d, c)
	case KindTabs:
		lines, marks, err = renderTabs(d, c)
	case KindVerdict:
		lines, marks, err = renderVerdict(d, c)
	case KindTree:
		lines, marks, err = renderTree(d, c)
	case KindCodeTitle:
		lines, err = renderCodeTitle(d, c)
	default:
		return nil, nil, fmt.Errorf("directive: no renderer for kind %v", d.Kind)
	}
	if err != nil || strings.TrimSpace(d.After) == "" {
		return lines, marks, err
	}
	after, am, err := c.body(d.After, c.Width, "a")
	if err != nil {
		return nil, nil, err
	}
	lead := len(after) - len(strings.TrimLeft(after, "\n"))
	after = strings.Trim(after, "\n")
	if after == "" {
		return lines, marks, nil
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	al := strings.Split(after, "\n")
	marks = append(marks, place(am, lead, len(al), len(lines))...)
	return append(lines, al...), marks, nil
}
