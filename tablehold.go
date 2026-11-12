package bubbleteandmd

import (
	"regexp"
	"strings"

	"github.com/iodesystems/bubbleteandmd/internal/directive"
)

// holdRows: while Frame streams, a GFM table in the tail with fewer body
// rows than this (or a lone header line) is not drawn; a one-line
// placeholder stands in. Column
// widths settle in the first rows, so the table appears once, with the
// widths it keeps (Carl 2026-11-11: no screen bounce).
const holdRows = 8

// holdText is the placeholder line.
const holdText = "table…"

var tableDelimLine = regexp.MustCompile(`^\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)

// delimStart: a partial line that can still become a delimiter row.
var delimStart = regexp.MustCompile(`^[ \t|:-]*$`)

// heldTable reports whether tail (healed text of the last block) is held:
// a GFM table with a header row, a delimiter row and fewer than holdRows
// body rows, or a first line that starts with `|` and can still become a
// table header (Carl 2026-11-11, option A: a header never draws as a
// paragraph). It is released when the second line cannot be a delimiter
// row; the frame then grows from the one placeholder line to a paragraph.
// Heal has hidden a partial last row, so every counted row is whole.
// A body row is any line after the delimiter row up to the first blank
// line, as the table parser reads it: `x | y` with no leading pipe counts.
func heldTable(tail string) bool {
	if !strings.HasPrefix(tail, "|") {
		return false
	}
	lines := strings.Split(strings.TrimRight(tail, "\n"), "\n")
	if len(lines) == 1 {
		// header row alone; a blank line after it ends the block
		return !strings.HasSuffix(tail, "\n\n")
	}
	if !tableDelimLine.MatchString(lines[1]) {
		// the delimiter row can still be typing
		return len(lines) == 2 && !strings.HasSuffix(tail, "\n") && delimStart.MatchString(lines[1])
	}
	body := 0
	for _, l := range lines[2:] {
		if strings.TrimSpace(l) == "" {
			break
		}
		body++
	}
	return body < holdRows
}

// placeheld is a Renderer that draws its own placeholder line.
type placeheld interface {
	placeholder(text string, first bool) string
}

// placeholder is one muted line in the frame of a glamour paragraph piece.
func (r *DirectiveRenderer) placeholder(text string, first bool) string {
	return directive.Frame([]string{r.theme.Muted.Render(text)}, r.outer.width, r.outer.frame, first)
}

// holdPiece is the piece that stands for a held table.
func (s *Stream) holdPiece(first bool) (string, error) {
	if p, ok := s.r.(placeheld); ok {
		return p.placeholder(holdText, first), nil
	}
	p, err := s.piece(holdText+"\n", 0, first)
	return p.text, err
}
