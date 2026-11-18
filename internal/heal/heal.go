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
	partialImage   = regexp.MustCompile(`\[?!\[[^\]]*(?:\](?:\([^)]*)?)?$`)
	badgeImage     = regexp.MustCompile(`\[(!\[[^\]]*\]\([^)]*\))\]?$`)
	partialLink    = regexp.MustCompile(`(?:^|[^!])\[(?:[^\]]*|!\[[^\]]*\]\([^)]*\))\]\([^)]*$`)
	codeSpan       = regexp.MustCompile("`[^`]*`")
	closedFormula  = regexp.MustCompile(`\$\$[\s\S]*?\$\$|\$[^$\n]+\$`)
	displayFormula = regexp.MustCompile(`\$\$[\s\S]*?\$\$`)
	tableRowStart  = regexp.MustCompile(`^[` + jsSpaceClass + `]{0,3}\|`)
	tableDelimCell = regexp.MustCompile(`^:?-{3,}:?$`)
	calloutPartial = regexp.MustCompile(`^[` + jsSpaceClass + `]{0,3}>[ \t]?(?:\[(?:!\w*)?)?$`)
	quoteLine      = regexp.MustCompile(`^[` + jsSpaceClass + `]{0,3}>[ \t]?([^\r\n\x{2028}\x{2029}]*)$`)
	markerHead     = regexp.MustCompile(`^\[!([A-Za-z]+)\]`)
	partialOpener  = regexp.MustCompile("^(?: {0,3}>[ \t]?)* {0,3}(?:`{1,2}|~{1,2}|[0-9]{1,9}|</?[A-Za-z][A-Za-z0-9-]*|</?)$")
	// bareBlockMarker is flowdown v0.7.1's BARE_BLOCK_MARKER.
	bareBlockMarker = regexp.MustCompile(`^[ \t]*(?:>[ \t]?)*(?:[-*+=]+|#{1,6}|\d{1,9}[.)]?|>)[ \t]*$`)
	quotePrefix     = regexp.MustCompile(`^[ \t]*(?:>[ \t]?)*`)
	bulletLine      = regexp.MustCompile(`^ {0,3}[-*+][ \t]+\S`)
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
	// A fence inside a blockquote closes with the opener's quote prefix; the TS reads no fence
	// there and heals its body as prose (a lone "`" line after "> ```text\n> code\n").
	// The unterminated last line, when it can still become the closer (`  ```, `>`), is the closer
	// arriving: drawn as a code line, it would vanish when the closer completes. It is dropped.
	// Not in the TS.
	f, fenceEnd := scanFences(text)
	if f != nil {
		closer := f.quote + f.indent + f.marker
		nl := strings.LastIndexByte(text, '\n')
		if nl+1 >= f.bodyStart && f.partialCloser(text[nl+1:]) {
			text = text[:nl+1]
		}
		if strings.HasSuffix(text, "\n") {
			return text + closer
		}
		return text + "\n" + closer
	}

	return closeProse(text, fenceEnd)
}

// closeProse is Close outside a fence. Text before fenceEnd (the end of the last closed fence)
// holds nothing it completes.
func closeProse(text string, fenceEnd int) string {
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
	if d := dropTrailingPartialOpener(text); d != text {
		return d
	}
	// The text above a block marker held back may itself end mid-construct (`**bold` on the
	// line before).
	if d := dropTrailingBareBlockMarker(text); d != text {
		return closeProse(d, min(fenceEnd, len(d)))
	}

	// Inline constructs cannot span a blank line or a bullet item's first line, so only the last
	// paragraph can dangle. Nor can they start in a fence: the tail starts after the last closed
	// fence when that is later (not in the TS, which counted a code block's backticks and drew a
	// "`" line under it).
	at := max(lastParagraph(text), fenceEnd)
	head, tail := text[:at], text[at:]
	if tail == "" {
		return text
	}
	// What is completed goes where the text stops, before the white space it may end in: a
	// closer after a space or line break closes nothing (`**bold **` draws its stars).
	end := len(trimRightJS(tail))
	tail, after := tail[:end], tail[end:]
	orig := tail

	tail = dropTrailingPartialImage(tail)
	tail = withholdPartialMath(tail)
	tail = closeTrailingLink(tail)
	tail = balanceInlineCode(tail)
	if after == "" {
		tail = hideTrailingLoneDelimiter(tail)
	}
	tail = balanceDoubleDelimiter(tail, "**")
	tail = balanceDoubleDelimiter(tail, "~~")
	tail = balanceSingleEmphasis(tail, '*')
	tail = balanceSingleEmphasis(tail, '_')
	if tail == orig {
		return text
	}
	// What was hidden may leave a marker bare (`- *` is `- `): it is held back as one typed so,
	// and a quote line it leaves (`> - *` is `>`) as one typed so.
	healed := head + tail + after
	if d := dropTrailingBareBlockMarker(healed); d != healed {
		return closeProse(d, min(fenceEnd, len(d)))
	}
	return healed
}

// dropTrailingBareBlockMarker holds back a last line that is so far only a block marker (`-`,
// `*`, `+`, `##`, `12.`, `>`, `> -`) or a run of `-` or `=`, until the first word of what it
// opens. Drawn at once it is an empty list item, heading or quote; and a `-` under a paragraph
// makes the paragraph a setext heading until the item's text arrives. From flowdown v0.7.1.
//
// Not flowdown: in a quote only the marker is held back, and the quote prefix stays (`> #` is
// `>`, `> > ##` is `> >`), so the frame is the one before the marker came (`> [!TABS]\n>`),
// which the caller heals on. A line of only quote prefixes is held back whole, as flowdown.
func dropTrailingBareBlockMarker(text string) string {
	nl := strings.LastIndexByte(text, '\n')
	// A marker is short: a long last line is read no further.
	line := text[nl+1:]
	if len(text)-nl > 16 || !bareBlockMarker.MatchString(line) {
		return text
	}
	prefix := strings.TrimRight(quotePrefix.FindString(line), " \t")
	if strings.HasSuffix(prefix, ">") && strings.TrimRight(line, " \t") != prefix {
		return text[:nl+1] + prefix
	}
	return text[:nl+1]
}

// lastParagraph is where the last paragraph of text starts: after its last blank line, or at its
// last line that starts a bullet item, which ends the paragraph above it. In a long tight list a
// frame heals the item being written, not the list; and a lone `*` in an earlier item (`2*3`) is
// not paired with one in the last. From flowdown v0.7.1.
func lastParagraph(text string) int {
	from := strings.LastIndex(text, "\n\n") + 2
	if from < 2 {
		from = 0
	}
	for at := strings.LastIndexByte(text, '\n'); at >= from; at = strings.LastIndexByte(text[:at], '\n') {
		if bulletLine.MatchString(text[at+1 : min(len(text), at+16)]) {
			return at + 1
		}
	}
	return from
}

// dropTrailingPartialImage hides a trailing half-written image (`![alt](partial-url`, `![alt]`
// or `![alt`), and the `[` before it when it is a badge's (`[![alt](src)](href)`). A badge whose
// image is written and whose link is not (`[![alt](src)` or `[![alt](src)]`) is drawn as its
// image, without the brackets that make it a link a few tokens later (flowdown v0.7.1).
func dropTrailingPartialImage(tail string) string {
	if loc := partialImage.FindStringIndex(tail); loc != nil {
		tail = tail[:loc[0]]
	}
	if m := badgeImage.FindStringSubmatchIndex(tail); m != nil {
		tail = tail[:m[0]] + tail[m[2]:m[3]]
	}
	return tail
}

// closeTrailingLink gives a trailing `[text](partial-url` its `)`. The text may be a badge's
// image.
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
// trailing opener is stripped; one with content after it gets closed, before any trailing white
// space: a closer after a space does not close (`**bold **` draws its stars). closeProse sets the
// text's own trailing white space aside first, as flowdown v0.7.1 does; white space a fix before
// this one left at the end (a hidden image) is dropped.
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
	// Half the closer typed (`**bold*`): complete it, not add a whole one (`**bold***`). Not in
	// the TS.
	return trimRightJS(strings.TrimSuffix(tail, delim[:1])) + delim
}

// hideTrailingLoneDelimiter hides a `*` or `_` that ends the tail after white space or at its
// start: the character after it, which says whether it opens emphasis, is not written yet. Drawn
// as a literal, it would join the line above as emphasis a token later (`   *` under a STEPS
// title). Not when white space followed it (closeProse): then it is text. Not in the TS, which
// keeps it.
func hideTrailingLoneDelimiter(tail string) string {
	n := len(tail)
	if n == 0 || tail[n-1] != '*' && tail[n-1] != '_' {
		return tail
	}
	if n > 1 && (tail[n-2] == '*' || tail[n-2] == '_' || !isJSSpace(rune(tail[n-2]))) {
		return tail
	}
	return tail[:n-1]
}

// dropTrailingPartialOpener hides an unterminated last line that is only the start of a fence
// opener (two backticks, `~`) or of an ordered list marker (`2`), after any quote prefixes and at
// most three spaces: as a paragraph it drew taller than the empty fence or list item it becomes.
// Not in the TS.
func dropTrailingPartialOpener(text string) string {
	nl := strings.LastIndexByte(text, '\n')
	if !partialOpener.MatchString(text[nl+1:]) {
		return text
	}
	return text[:nl+1]
}

// balanceSingleEmphasis balances `*` or `_`, counting only emphasis-capable lone delimiters:
// runs of two or more belong to `**`/`__`; code and formulas are masked; `*` counts only when it
// touches a word character; `_` only when the word character is on one side (no intraword
// emphasis). An odd count appends one closer (before trailing white space, as
// balanceDoubleDelimiter), or strips a bare trailing opener.
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
	return trimRightJS(tail) + string(char)
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
// `> [!`, `> [!WARN`) until the `]` arrives. A complete marker stays. A bare `>` or `> ` last line
// is hidden as well (not in the TS): drawn as an empty quote line, it vanished when `[` arrived.
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
