package bubbleteandmd

import (
	"regexp"
	"strings"

	"github.com/iodesystems/bubbleteandmd/internal/directive"
)

// holdRows: while Frame streams, a GFM table in the tail with fewer body
// rows than this is not drawn; a one-line placeholder stands in. Column
// widths settle in the first rows, so the table appears once, with the
// widths it keeps (Carl 2026-11-11: no screen bounce).
const holdRows = 8

// holdText is the placeholder line.
const holdText = "table…"

var tableDelimLine = regexp.MustCompile(`^\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)

// heldTable reports whether tail (healed text of the last block) is a GFM
// table with a header row, a delimiter row and fewer than holdRows body
// rows. Heal has hidden a partial last row, so every counted row is whole.
func heldTable(tail string) bool {
	if !strings.HasPrefix(tail, "|") {
		return false
	}
	lines := strings.Split(strings.TrimRight(tail, "\n"), "\n")
	if len(lines) < 2 || !tableDelimLine.MatchString(lines[1]) {
		return false
	}
	body := 0
	for _, l := range lines[2:] {
		if strings.HasPrefix(l, "|") {
			body++
		}
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
	return s.piece(holdText+"\n", first)
}
