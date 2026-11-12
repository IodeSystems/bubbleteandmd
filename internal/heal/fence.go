// Ported from llx-markdown packages/llx-markdown/src/core/fence-scan.ts (MIT, see NOTICE).
// The TS caches the scan between calls; this port rescans, so it has no shared state.

package heal

import "strings"

type fence struct {
	marker    string
	indent    string // the opener's leading spaces, after its quote prefix
	quote     string // the opener's quote prefix as written (`> `), "" outside a blockquote
	depth     int    // the number of `>` in quote
	bodyStart int
}

// unquote strips up to max blockquote prefixes (`>` after at most three spaces, and one space or
// tab after it) from line; max < 0 strips all. It returns how many it stripped and the rest. Not
// in the TS, which reads no fence inside a blockquote.
func unquote(line string, max int) (int, string) {
	depth := 0
	for max < 0 || depth < max {
		rest := strings.TrimLeft(line, " ")
		if len(line)-len(rest) > 3 || !strings.HasPrefix(rest, ">") {
			break
		}
		rest = rest[1:]
		if rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
			rest = rest[1:]
		}
		line = rest
		depth++
	}
	return depth, line
}

// partialCloser reports whether line, the unterminated last line inside f, can still become f's
// closer: f's quote prefixes, then spaces (at most three past the opener's indent), then fewer
// fence characters than the opener has, and nothing else. Not in the TS.
func (f *fence) partialCloser(line string) bool {
	depth, rest := unquote(line, f.depth)
	if depth < f.depth {
		return false
	}
	body := strings.TrimLeft(rest, " ")
	if len(rest)-len(body) > len(f.indent)+3 || len(body) >= len(f.marker) {
		return false
	}
	return strings.Trim(body, f.marker[:1]) == ""
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
// Inside a blockquote a fence reads its lines after the opener's quote prefixes; a line with fewer
// of them ends the quote and the fence with it (a fence takes no lazy continuation line).
func step(open *fence, line string, lineEnd, textLength int) *fence {
	if open != nil {
		if depth, inner := unquote(line, open.depth); depth == open.depth {
			run, rest, ok := parseFenceLine(inner)
			if ok && run[0] == open.marker[0] && len(run) >= len(open.marker) && trimJS(rest) == "" {
				return nil
			}
			return open
		}
	}
	depth, inner := unquote(line, -1)
	run, rest, ok := parseFenceLine(inner)
	if !ok {
		return nil
	}
	// A backtick fence has no backtick in its info string: ```a``` is inline code.
	if run[0] == '`' && strings.Contains(rest, "`") {
		return nil
	}
	return &fence{
		marker:    run,
		indent:    inner[:len(inner)-len(strings.TrimLeft(inner, " "))],
		quote:     line[:len(line)-len(inner)],
		depth:     depth,
		bodyStart: min(lineEnd, textLength),
	}
}

// openFence is the fence the text ends inside, or nil when every fence is closed. A closing fence
// must use the same character, be at least as long as the opener, and carry no info string.
func openFence(text string) *fence {
	open, _ := scanFences(text)
	return open
}

// scanFences returns the fence the text ends inside (nil when every fence is closed) and the
// offset where the last fence that ended ends: after its closing line, or at the start of the
// line that ended its blockquote; 0 when no fence ended. Text before that offset holds no
// construct heal completes.
func scanFences(text string) (open *fence, after int) {
	offset := 0
	for {
		nl := strings.IndexByte(text[offset:], '\n')
		if nl < 0 {
			break
		}
		end := offset + nl
		open, after = stepAt(open, after, text[offset:end], offset, end+1, len(text))
		offset = end + 1
	}
	// The unterminated last line is evaluated as well: it may still change.
	if offset < len(text) || offset == 0 {
		open, after = stepAt(open, after, text[offset:], offset, len(text)+1, len(text))
	}
	return open, after
}

// stepAt is step, and moves after past a fence that the line ends.
func stepAt(open *fence, after int, line string, lineStart, lineEnd, textLength int) (*fence, int) {
	next := step(open, line, lineEnd, textLength)
	if open != nil && next != open {
		if depth, _ := unquote(line, open.depth); depth == open.depth {
			after = min(lineEnd, textLength) // the closing line
		} else {
			after = lineStart // the line that ended the quote
		}
	}
	return next, after
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
