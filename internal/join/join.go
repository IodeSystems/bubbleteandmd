// Package join puts separately rendered glamour blocks together so that the
// result equals glamour's render of the whole document.
//
// The rule (verified in internal/spike): a block that is not first in the
// document is rendered after Sentinel, so its first node has a previous
// sibling and glamour gives it the margin it has in the whole document. Each
// render starts with the document prefix P and ends with the suffix S (the
// two halves of the render of ""); both are stripped from each piece, and the
// joined pieces are wrapped in P…S once. An empty piece is skipped. When the
// previous piece does not end in "\n", the next piece's first line starts
// with cells the previous piece already wrote; they are dropped.
package join

import (
	"errors"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Sentinel goes before every block that is not first in the document. An
// HTML comment renders as nothing, but is a previous sibling.
const Sentinel = "<!-- -->\n\n"

// ErrAffixes is returned when the render of "" is not two equal halves.
var ErrAffixes = errors.New("join: render of empty document is not two equal halves")

// ErrPiece is returned when a render does not start with P and end with S.
var ErrPiece = errors.New("join: render lacks document prefix or suffix")

// Affixes returns the document prefix and suffix, given empty, the render
// of "".
func Affixes(empty string) (p, s string, err error) {
	h := len(empty) / 2
	if len(empty)%2 != 0 || empty[:h] != empty[h:] {
		return "", "", ErrAffixes
	}
	return empty[:h], empty[h:], nil
}

// Input is the text to render for block src; first is true for the block
// at offset 0 of the document.
func Input(src string, first bool) string {
	if first {
		return src
	}
	return Sentinel + src
}

// Strip removes p and s from a render.
func Strip(out, p, s string) (string, error) {
	if !strings.HasPrefix(out, p) || !strings.HasSuffix(out, s) || len(out) < len(p)+len(s) {
		return "", ErrPiece
	}
	return out[len(p) : len(out)-len(s)], nil
}

// Join returns p + pieces + s, skipping empty pieces and dropping from each
// piece's first line the cells the piece before it left on its last line.
func Join(p, s string, pieces []string) string {
	n := len(p) + len(s)
	for _, x := range pieces {
		n += len(x)
	}
	var b strings.Builder
	b.Grow(n)
	b.WriteString(p)
	prev := ""
	for _, piece := range pieces {
		if piece == "" {
			continue
		}
		if prev != "" && !strings.HasSuffix(prev, "\n") {
			last := prev[strings.LastIndexByte(prev, '\n')+1:]
			nl := strings.IndexByte(piece, '\n')
			if nl < 0 {
				nl = len(piece)
			}
			piece = DropCells(piece[:nl], ansi.StringWidth(last)) + piece[nl:]
		}
		prev = piece
		b.WriteString(piece)
	}
	b.WriteString(s)
	return b.String()
}

// DropCells removes the first n visible cells of s and the SGR sequences
// that style them; sequences after the nth cell are kept. A cell styled as
// ESC[..m c ESC[m takes its opening and closing sequences with it. An empty
// styled run (ESC[..m ESC[m, nothing between) is not a cell: glamour writes
// one where an element starts, and the whole render has it too, so it is
// kept. Bytes count as cells: glamour pads with single-width spaces.
func DropCells(s string, n int) string {
	var kept strings.Builder
	i := 0
	for n > 0 && i < len(s) {
		if j := sgrEnd(s, i); j > 0 {
			if k := sgrEnd(s, j); k > 0 && s[j:k] == "\x1b[m" {
				kept.WriteString(s[i:k])
				i = k
				continue
			}
			i = j
			continue
		}
		i++
		n--
		if strings.HasPrefix(s[i:], "\x1b[m") {
			i += 3
		}
	}
	if kept.Len() == 0 {
		return s[i:]
	}
	return kept.String() + s[i:]
}

// sgrEnd returns the end of the CSI sequence at s[i:], or 0 when there is
// none there.
func sgrEnd(s string, i int) int {
	if i+1 >= len(s) || s[i] != 0x1b || s[i+1] != '[' {
		return 0
	}
	j := i + 2
	for j < len(s) && s[j] != 'm' {
		j++
	}
	return j + 1
}
