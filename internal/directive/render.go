package directive

import (
	"errors"
	"fmt"
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
}

// errNotImplemented is returned by a form with no renderer yet.
var errNotImplemented = errors.New("not implemented")

// Render draws one directive as content lines, each at most c.Width cells,
// with no margin and no pad. Frame adds those.
func Render(d Directive, c Ctx) ([]string, error) {
	switch d.Kind {
	case KindCallout:
		return renderCallout(d, c)
	case KindDetails:
		return renderDetails(d, c)
	case KindAttribution:
		return renderAttribution(d, c)
	case KindSteps:
		return renderSteps(d, c)
	case KindStats:
		return renderStats(d, c)
	case KindSources:
		return renderSources(d, c)
	case KindTabs:
		return renderTabs(d, c)
	case KindVerdict:
		return renderVerdict(d, c)
	case KindTree:
		return renderTree(d, c)
	case KindCodeTitle:
		return renderCodeTitle(d, c)
	}
	return nil, fmt.Errorf("directive: no renderer for kind %v", d.Kind)
}
