// Ported from llx-markdown packages/llx-markdown/src/core/dollars.ts and math-delimiters.ts
// (MIT, see NOTICE), as of flowdown v0.7.1. Not the escaper: MathSpans gives the formulas
// literalDollars leaves, for the renderer (internal/directive InlineMath).

package heal

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

func isSpaceByte(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

// at is text[i], or 0 (never a character the callers test for) outside the text.
func at(text string, i int) byte {
	if i < 0 || i >= len(text) {
		return 0
	}
	return text[i]
}

// opensInlineMath reports whether a single `$` at index can open a formula: something other than
// a space follows it. At the end of the text it cannot.
func opensInlineMath(text string, index int) bool {
	if index+1 >= len(text) {
		return false
	}
	next := text[index+1]
	return next != '$' && !isSpaceByte(next)
}

// closesInlineMath reports whether a single `$` at index can close a formula: something other
// than a space before it, no digit after it, and not escaped.
func closesInlineMath(text string, index int) bool {
	if index == 0 {
		return false
	}
	before := text[index-1]
	if isSpaceByte(before) || before == '$' || isDigitByte(at(text, index+1)) {
		return false
	}
	slashes := 0
	for at(text, index-1-slashes) == '\\' {
		slashes++
	}
	return slashes%2 == 0
}

// window is up to 48 UTF-16 code units of text from start, as the source's slice(index+1, index+48).
func window(text string, start int) string {
	units := 0
	end := start
	for end < len(text) {
		r, w := utf8.DecodeRuneInString(text[end:])
		u := 1
		if r > 0xffff {
			u = 2
		}
		if units+u > 47 {
			break
		}
		units += u
		end += w
	}
	return text[start:end]
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// currencyAfter is /^\d[\d.,]*(?:[kKmMbB])?(?=[\s.,;:!?)\]'"`–—/-]|$)/. RE2 has no lookahead and
// the source backtracks into [\d.,]*, so every split is tried, longest first.
func currencyAfter(s string) bool {
	if s == "" || !isDigitByte(s[0]) {
		return false
	}
	n := 1
	for n < len(s) && (isDigitByte(s[n]) || s[n] == '.' || s[n] == ',') {
		n++
	}
	for pos := n; pos >= 1; pos-- {
		if pos < len(s) && strings.IndexByte("kKmMbB", s[pos]) >= 0 && currencyEnd(s, pos+1) {
			return true
		}
		if currencyEnd(s, pos) {
			return true
		}
	}
	return false
}

func currencyEnd(s string, pos int) bool {
	if pos >= len(s) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[pos:])
	return isJSSpace(r) || strings.ContainsRune(".,;:!?)]'\"`–—/-", r)
}

// variableAfter is /^[A-Z_][A-Z0-9_]+(?=[/:.,;)\]'"`]|$)/. Backtracking cannot help: a shorter
// run would end at a character the lookahead rejects.
func variableAfter(s string) bool {
	n := upperRun(s)
	if n < 2 {
		return false
	}
	return n == len(s) || strings.IndexByte("/:.,;)]'\"`", s[n]) >= 0
}

// longVariableAfter is /^[A-Z_][A-Z0-9_]{2,}(?=\s)/.
func longVariableAfter(s string) bool {
	n := upperRun(s)
	if n < 3 || n >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[n:])
	return isJSSpace(r)
}

// upperRun is the length of the match of [A-Z_][A-Z0-9_]*, or 0.
func upperRun(s string) int {
	if s == "" || !(s[0] >= 'A' && s[0] <= 'Z' || s[0] == '_') {
		return 0
	}
	n := 1
	for n < len(s) && (s[n] >= 'A' && s[n] <= 'Z' || isDigitByte(s[n]) || s[n] == '_') {
		n++
	}
	return n
}

// isMathSign reports a sign only math writes: what a formula that starts with a number shows
// itself by (MATH_SIGN, /[=^_\\{}<>]/).
func isMathSign(c rune) bool { return strings.ContainsRune(`=^_\{}<>`, c) }

// isLiteralDollar reports whether the `$` at index is a dollar sign: a price or a shell variable
// by its own look. A formula may start with a number (`$2 + 2 = 4$`, flowdown v0.7.1): a
// price-looking `$` still opens one when a `$` of no such look closes it in its paragraph and
// what lies between holds a math sign and no other `$`; "$5 and $10", "pay $5 now and 7$ later"
// and "from $5 for $n^2$" hold none.
func isLiteralDollar(text string, index int) bool {
	if !looksLiteral(text, index) {
		return false
	}
	if !currencyAfter(window(text, index+1)) || !opensInlineMath(text, index) {
		return true
	}
	end := inlineMathClose(text, index)
	if end == -1 {
		return true
	}
	inner := text[index+1 : end]
	return strings.Contains(inner, "$") || !strings.ContainsFunc(inner, isMathSign)
}

// looksLiteral reports whether the `$` at index is a dollar sign by its own look, whatever
// follows in the paragraph: a price or a shell variable.
func looksLiteral(text string, index int) bool {
	if at(text, index) != '$' || isEscaped(text, index) {
		return false
	}
	// `$$` is display math, never a dollar sign.
	if at(text, index-1) == '$' || at(text, index+1) == '$' {
		return false
	}
	after := window(text, index+1)
	return currencyAfter(after) || variableAfter(after) || longVariableAfter(after)
}

func isSingleDollar(text string, index int) bool {
	return at(text, index) == '$' && at(text, index-1) != '$' && at(text, index+1) != '$' &&
		!isEscaped(text, index)
}

var blankLine = regexp.MustCompile(`\n[ \t]*\n`)

// paragraphEnd is where the paragraph holding index ends: inline math never crosses a blank line.
func paragraphEnd(text string, index int) int {
	if loc := blankLine.FindStringIndex(text[index:]); loc != nil {
		return index + loc[0]
	}
	return len(text)
}

// inlineMathClose is the single `$` that closes the formula opened at open, or -1 when none in
// its paragraph does. A dollar sign by its look (`$5`) is text and closes nothing.
func inlineMathClose(text string, open int) int {
	end := paragraphEnd(text, open)
	for i := nextDollar(text, open+1); i != -1 && i < end; i = nextDollar(text, i+1) {
		if isSingleDollar(text, i) && !looksLiteral(text, i) && closesInlineMath(text, i) {
			return i
		}
	}
	return -1
}

// MathSpans returns each inline formula in prose as the offsets of its opening and closing `$`,
// read left to right as literalDollars reads them: every other single `$` is text (a price, a
// variable, one that cannot open, one that opens but finds no close in its paragraph). prose
// is Markdown with its code blanked, offsets kept; a `$$` run is skipped whole.
func MathSpans(prose string) [][2]int {
	var out [][2]int
	for i := nextDollar(prose, 0); i != -1; i = nextDollar(prose, i+1) {
		if at(prose, i+1) == '$' {
			for at(prose, i+1) == '$' {
				i++
			}
			continue
		}
		if !isSingleDollar(prose, i) || isLiteralDollar(prose, i) || !opensInlineMath(prose, i) {
			continue
		}
		if end := inlineMathClose(prose, i); end != -1 {
			out = append(out, [2]int{i, end})
			i = end
		}
	}
	return out
}

func nextDollar(text string, from int) int {
	if from > len(text) {
		return -1
	}
	if i := strings.IndexByte(text[from:], '$'); i >= 0 {
		return from + i
	}
	return -1
}
