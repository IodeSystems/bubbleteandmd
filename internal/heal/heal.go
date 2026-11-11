// Package heal stabilizes the tail of a markdown text that is still streaming.
//
// While a reply streams, the accumulated markdown often ends mid-construct: an unterminated
// fence, a dangling `**` or backtick, a half-typed link. Close completes or hides those trailing
// constructs so the view does not flash raw markers and then jump. It is for streaming text
// only; the final text renders verbatim.
//
// Ported from llx-markdown packages/llx-markdown/src/core/heal.ts (MIT, see NOTICE). Indexes are
// byte offsets, not UTF-16 units; masks are as long as their source in bytes.
package heal

import (
	"regexp"
	"strings"
)

var (
	partialImage   = regexp.MustCompile(`!\[[^\]]*(?:\]\([^)]*)?$`)
	partialLink    = regexp.MustCompile(`(?:^|[^!])\[[^\]]*\]\([^)]*$`)
	codeSpan       = regexp.MustCompile("`[^`]*`")
	closedFormula  = regexp.MustCompile(`\$\$[\s\S]*?\$\$|\$[^$\n]+\$`)
	displayFormula = regexp.MustCompile(`\$\$[\s\S]*?\$\$`)
	tableRowStart  = regexp.MustCompile(`^[` + jsSpaceClass + `]{0,3}\|`)
	tableDelimCell = regexp.MustCompile(`^:?-{3,}:?$`)
	calloutPartial = regexp.MustCompile(`^[` + jsSpaceClass + `]{0,3}>[ \t]?\[(?:!\w*)?$`)
	quoteLine      = regexp.MustCompile(`^[` + jsSpaceClass + `]{0,3}>[ \t]?([^\r\n\x{2028}\x{2029}]*)$`)
	markerHead     = regexp.MustCompile(`^\[!([A-Za-z]+)\]`)
)

// containerKinds are llx-markdown's dialect.ts CONTAINER_KINDS.
var containerKinds = map[string]bool{
	"steps": true, "tabs": true, "stats": true, "pros": true, "cons": true, "details": true, "sources": true,
}

// Close completes or hides the unfinished tail of text. Text with nothing to complete comes back
// as the same string.
func Close(text string) string {
	if text == "" {
		return text
	}

	// Inside an unterminated fence: close it and skip inline fixes, since everything after the
	// opener is code. The closer takes the opener's indent: in a list item, a closer at column 0
	// is outside the item, so it ends the item and opens a new empty fence, one row more than
	// the fence's own closing line draws. The TS closes at column 0.
	if f := openFence(text); f != nil {
		if strings.HasSuffix(text, "\n") {
			return text + f.indent + f.marker
		}
		return text + "\n" + f.indent + f.marker
	}

	// A half-typed table row, a callout marker still being typed and a form marker with no
	// content yet are hidden; that is the whole fix for this frame.
	if d := dropTrailingPartialTableRow(text); d != text {
		return d
	}
	if d := dropTrailingPartialCalloutMarker(text); d != text {
		return d
	}
	if d := dropTrailingBareFormMarker(text); d != text {
		return d
	}

	// Inline constructs cannot span a blank line, so only the last paragraph can dangle.
	splitAt := strings.LastIndex(text, "\n\n")
	head, tail := "", text
	if splitAt != -1 {
		head, tail = text[:splitAt+2], text[splitAt+2:]
	}
	if tail == "" {
		return text
	}
	orig := tail

	tail = dropTrailingPartialImage(tail)
	tail = withholdPartialMath(tail)
	tail = closeTrailingLink(tail)
	tail = balanceInlineCode(tail)
	tail = balanceDoubleDelimiter(tail, "**")
	tail = balanceDoubleDelimiter(tail, "~~")
	tail = balanceSingleEmphasis(tail, '*')
	tail = balanceSingleEmphasis(tail, '_')
	if tail == orig {
		return text
	}
	return head + tail
}

// dropTrailingPartialImage hides a trailing half-written image (`![alt](partial-url` or `![alt`).
func dropTrailingPartialImage(tail string) string {
	if loc := partialImage.FindStringIndex(tail); loc != nil {
		return tail[:loc[0]]
	}
	return tail
}

// closeTrailingLink gives a trailing `[text](partial-url` its `)`.
func closeTrailingLink(tail string) string {
	if partialLink.MatchString(tail) {
		return tail + ")"
	}
	return tail
}

// maskInert blanks code spans and closed formulas, keeping positions. Emphasis has no meaning
// inside either.
func maskInert(tail string) string {
	return closedFormula.ReplaceAllStringFunc(codeSpan.ReplaceAllStringFunc(tail, blank), blank)
}

// withholdPartialMath cuts a formula still being typed from its opening `$`. Dollars pair by the
// same rule as the renderer (a price, a shell variable or a `$` with a space after it opens
// nothing). Only the first `$` left open is withheld, and only while no `$` follows it.
func withholdPartialMath(tail string) string {
	code := codeSpan.ReplaceAllStringFunc(tail, blank)
	lastDisplay, displays := -1, 0
	for i := strings.Index(code, "$$"); i != -1; {
		if !isEscaped(code, i) {
			lastDisplay = i
			displays++
		}
		next := strings.Index(code[i+2:], "$$")
		if next == -1 {
			break
		}
		i += 2 + next
	}
	if displays%2 == 1 {
		return trimRightJS(tail[:lastDisplay])
	}

	inline := displayFormula.ReplaceAllStringFunc(code, blank)
	for i := nextDollar(inline, 0); i != -1; i = nextDollar(inline, i+1) {
		if isEscaped(inline, i) || isLiteralDollar(inline, i) {
			continue
		}
		// The last character: the one after it, which says whether it opens, is not written yet.
		last := i == len(inline)-1
		if !last && !opensInlineMath(inline, i) {
			continue
		}
		closeAt := -1
		if !last {
			closeAt = inlineMathClose(inline, i)
		}
		if closeAt != -1 {
			i = closeAt
			continue
		}
		if nextDollar(inline, i+1) != -1 {
			return tail
		}
		return trimRightJS(tail[:i])
	}
	return tail
}

// balanceInlineCode: an odd number of backticks means an open span. A bare trailing opener is
// stripped; an opener with content gets closed.
func balanceInlineCode(tail string) string {
	if strings.Count(strings.ReplaceAll(tail, "\\`", ""), "`")%2 == 0 {
		return tail
	}
	if strings.HasSuffix(tail, "`") {
		return tail[:len(tail)-1]
	}
	return tail + "`"
}

// balanceDoubleDelimiter balances `**` or `~~`, ignoring code spans and formulas. A bare
// trailing opener is stripped; one with content after it gets closed.
func balanceDoubleDelimiter(tail, delim string) string {
	masked := maskInert(tail)
	count := 0
	for i := strings.Index(masked, delim); i != -1; {
		count++
		next := strings.Index(masked[i+len(delim):], delim)
		if next == -1 {
			break
		}
		i += len(delim) + next
	}
	if count%2 == 0 {
		return tail
	}
	if strings.HasSuffix(tail, delim) {
		return tail[:len(tail)-len(delim)]
	}
	return tail + delim
}

// balanceSingleEmphasis balances `*` or `_`, counting only emphasis-capable lone delimiters:
// runs of two or more belong to `**`/`__`; code and formulas are masked; `*` counts only when it
// touches a word character; `_` only when the word character is on one side (no intraword
// emphasis). An odd count appends one closer, or strips a bare trailing opener.
func balanceSingleEmphasis(tail string, char byte) string {
	masked := maskInert(tail)
	active := 0
	for i := 0; i < len(masked); {
		if masked[i] != char {
			i++
			continue
		}
		j := i
		for j < len(masked) && masked[j] == char {
			j++
		}
		if j-i == 1 {
			prev := i > 0 && isWordByte(masked[i-1])
			next := j < len(masked) && isWordByte(masked[j])
			if char == '*' && (prev || next) || char == '_' && prev != next {
				active++
			}
		}
		i = j
	}
	if active%2 == 0 {
		return tail
	}
	if tail[len(tail)-1] == char {
		return tail[:len(tail)-1]
	}
	return tail + string(char)
}

// dropTrailingPartialTableRow hides only a partial row of an established GFM table. A line
// starting with `|` outside a table may be prose and stays.
func dropTrailingPartialTableRow(text string) string {
	if strings.HasSuffix(text, "\n") {
		return text
	}
	nl := strings.LastIndex(text, "\n")
	if nl == -1 || !tableRowStart.MatchString(text[nl+1:]) {
		return text
	}
	// Walk the preceding lines from the end; a line i needs a line i-1 before it.
	end := nl
	for {
		start := strings.LastIndex(text[:end], "\n") + 1
		if start == 0 {
			return text
		}
		line, prevEnd := text[start:end], start-1
		prevStart := strings.LastIndex(text[:prevEnd], "\n") + 1
		cells := strings.TrimSuffix(strings.TrimPrefix(trimJS(line), "|"), "|")
		if isDelimiterRow(cells) {
			if tableRowStart.MatchString(text[prevStart:prevEnd]) {
				return text[:nl]
			}
			return text
		}
		if !tableRowStart.MatchString(line) {
			return text
		}
		end = prevEnd
	}
}

func isDelimiterRow(cells string) bool {
	for _, cell := range strings.Split(cells, "|") {
		if !tableDelimCell.MatchString(trimJS(cell)) {
			return false
		}
	}
	return true
}

// dropTrailingPartialCalloutMarker hides a quote line still typing its callout marker (`> [`,
// `> [!`, `> [!WARN`) until the `]` arrives. A complete marker stays.
func dropTrailingPartialCalloutMarker(text string) string {
	nl := strings.LastIndex(text, "\n")
	if !calloutPartial.MatchString(text[nl+1:]) {
		return text
	}
	if nl == -1 {
		return ""
	}
	return text[:nl]
}

// dropTrailingBareFormMarker hides a form's marker (`> [!STEPS]`) that arrived before any
// content, while only blank space follows it.
func dropTrailingBareFormMarker(text string) string {
	trimmed := trimRightJS(text)
	nl := strings.LastIndex(trimmed, "\n")
	quoted := quoteLine.FindStringSubmatch(trimmed[nl+1:])
	if quoted == nil || !isContainerMarker(trimJS(quoted[1])) {
		return text
	}
	if nl == -1 {
		return ""
	}
	return trimmed[:nl+1]
}

// isContainerMarker is dialect.ts parseContainerMarker(text) != null: `[!KIND]` of a form kind.
func isContainerMarker(text string) bool {
	m := markerHead.FindStringSubmatch(text)
	return m != nil && containerKinds[strings.ToLower(m[1])]
}
