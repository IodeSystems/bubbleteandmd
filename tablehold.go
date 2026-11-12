package bubbleteandmd

import (
	"regexp"
	"strings"

	"github.com/iodesystems/bubbleteandmd/internal/directive"
)

// Tables draw row by row as they stream (Carl 2026-11-11): each row is one
// line (wide.go cuts a wide table at the width), so a change of column
// widths moves cells sideways and never changes the height. Only the
// header-only state is held: a lone header line drawn as a paragraph can
// wrap to more lines than the table's header later takes.

// holdText is the placeholder line.
const holdText = "table…"

// delimStart: a partial line that can still become a delimiter row.
var delimStart = regexp.MustCompile(`^[ \t|:-]*$`)

// heldTable reports whether tail (text of the last block before heal: heal
// hides a partial body row with the "\n" before it, so a table with no
// whole body row would read as a delimiter row still typing) is held:
// a first line that starts with `|` and can still become a table header,
// alone or with a second line, not yet ended by "\n", that can still become
// the delimiter row (Carl 2026-11-11, option A: a header never draws as a
// paragraph). Once the delimiter row has its "\n", the table draws. It is
// released when the
// second line cannot be a delimiter row; the frame then grows from the one
// placeholder line to a paragraph.
func heldTable(tail string) bool {
	if !strings.HasPrefix(tail, "|") {
		return false
	}
	lines := strings.Split(strings.TrimRight(tail, "\n"), "\n")
	if len(lines) == 1 {
		// header row alone; a blank line after it ends the block
		return !strings.HasSuffix(tail, "\n\n")
	}
	// the delimiter row can still be typing: until its "\n", a cell count
	// below the header's reads as a paragraph
	return len(lines) == 2 && !strings.HasSuffix(tail, "\n") && delimStart.MatchString(lines[1])
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
