// Ported from llx-markdown packages/llx-markdown/src/core/dollars.ts and math-delimiters.ts
// (MIT, see NOTICE). Only what the healer reads is ported: not literalDollars or the escaper.

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

// isLiteralDollar reports whether the `$` at index is a dollar sign by its own look: a price or a
// shell variable.
func isLiteralDollar(text string, index int) bool {
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
// its paragraph does. A dollar sign (`$5`) is text and closes nothing.
func inlineMathClose(text string, open int) int {
	end := paragraphEnd(text, open)
	for i := nextDollar(text, open+1); i != -1 && i < end; i = nextDollar(text, i+1) {
		if isSingleDollar(text, i) && !isLiteralDollar(text, i) && closesInlineMath(text, i) {
			return i
		}
	}
	return -1
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
