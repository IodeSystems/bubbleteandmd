// Ported from llx-markdown packages/llx-markdown/src/core (MIT, see NOTICE).

package heal

import "strings"

// jsSpaceClass is the set JavaScript's \s and String.prototype.trim use. Go's \s is ASCII only.
const jsSpaceClass = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

func isJSSpace(r rune) bool {
	switch {
	case r >= '\t' && r <= '\r', r == ' ', r == 0xa0, r == 0x1680,
		r >= 0x2000 && r <= 0x200a, r == 0x2028, r == 0x2029,
		r == 0x202f, r == 0x205f, r == 0x3000, r == 0xfeff:
		return true
	}
	return false
}

func trimJS(s string) string      { return strings.TrimFunc(s, isJSSpace) }
func trimRightJS(s string) string { return strings.TrimRightFunc(s, isJSSpace) }

// blank is a run of spaces as long as span in bytes: positions in the masked string stay valid
// indexes into the original.
func blank(span string) string { return strings.Repeat(" ", len(span)) }

// isEscaped reports whether the byte at index is backslash-escaped (an odd run of `\` before it).
func isEscaped(text string, index int) bool {
	slashes := 0
	for i := index - 1; i >= 0 && text[i] == '\\'; i-- {
		slashes++
	}
	return slashes%2 == 1
}
