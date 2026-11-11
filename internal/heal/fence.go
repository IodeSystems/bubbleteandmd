// Ported from llx-markdown packages/llx-markdown/src/core/fence-scan.ts (MIT, see NOTICE).
// The TS caches the scan between calls; this port rescans, so it has no shared state.

package heal

import "strings"

type fence struct {
	marker    string
	indent    string // the opener's leading spaces
	bodyStart int
}

// parseFenceLine matches /^ {0,3}(`{3,}|~{3,})(.*)$/. JS's `.` stops at \r, U+2028 and U+2029,
// so a line ending in \r is not a fence line, as in the source. The TS has \s{0,3}; here the
// indent is spaces only, as CommonMark and blocks.Split read it: a tab is four columns, so
// "\t```" starts indented code, and closing it would add a fence the final text lacks.
func parseFenceLine(line string) (run, rest string, ok bool) {
	body := strings.TrimLeft(line, " ")
	if len(line)-len(body) > 3 || body == "" || (body[0] != '`' && body[0] != '~') {
		return "", "", false
	}
	n := 0
	for n < len(body) && body[n] == body[0] {
		n++
	}
	if n < 3 || strings.ContainsAny(body[n:], "\r  ") {
		return "", "", false
	}
	return body[:n], body[n:], true
}

// step is the fence state after one line, given the state before it.
func step(open *fence, line string, lineEnd, textLength int) *fence {
	run, rest, ok := parseFenceLine(line)
	if !ok {
		return open
	}
	if open == nil {
		// A backtick fence has no backtick in its info string: ```a``` is inline code.
		if run[0] == '`' && strings.Contains(rest, "`") {
			return nil
		}
		return &fence{marker: run, indent: line[:len(line)-len(strings.TrimLeft(line, " "))], bodyStart: min(lineEnd, textLength)}
	}
	if run[0] == open.marker[0] && len(run) >= len(open.marker) && trimJS(rest) == "" {
		return nil
	}
	return open
}

// openFence is the fence the text ends inside, or nil when every fence is closed. A closing fence
// must use the same character, be at least as long as the opener, and carry no info string.
func openFence(text string) *fence {
	var open *fence
	offset := 0
	for {
		nl := strings.IndexByte(text[offset:], '\n')
		if nl < 0 {
			break
		}
		end := offset + nl
		open = step(open, text[offset:end], end+1, len(text))
		offset = end + 1
	}
	// The unterminated last line is evaluated as well: it may still change.
	if offset < len(text) || offset == 0 {
		open = step(open, text[offset:], len(text)+1, len(text))
	}
	return open
}

// OpenFenceBody returns the body of the fence the text ends inside; ok is false when every fence
// is closed. Only the last fence can be unterminated, so a renderer uses this to tell the block
// still being written from finished blocks above it.
func OpenFenceBody(text string) (body string, ok bool) {
	f := openFence(text)
	if f == nil {
		return "", false
	}
	return text[f.bodyStart:], true
}

// IsGrowingFence reports whether source is the incomplete fence body still being written, as
// OpenFenceBody returned it (growing, ok).
func IsGrowingFence(growing string, ok bool, source string) bool {
	return ok && strings.TrimRightFunc(growing, isJSSpace) == trimRightJS(source)
}
