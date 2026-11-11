package directive

import (
	"regexp"
	"strings"
)

// mathFenceOpen is a `$$` formula's first line (remark-math: up to three
// spaces of indent); codeFenceRe is a code fence line.
var (
	mathFenceOpen = regexp.MustCompile(`^ {0,3}\$\$`)
	codeFenceRe   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
)

// MathShift: from byte At of the rewritten text on, offsets are Delta more
// than in the text as written (summed over the shifts at or before).
type MathShift struct{ At, Delta int }

// RawOffset is the offset in the text as written of byte rel of the text
// MathFences rewrote with shifts.
func RawOffset(shifts []MathShift, rel int) int {
	d := 0
	for _, s := range shifts {
		if s.At > rel {
			break
		}
		d += s.Delta
	}
	return rel - d
}

// MathFences rewrites each top-level `$$ … $$` formula in src as a ```math
// fence (Carl B7: a formula shows as its TeX source, as code). Lines inside a
// code fence are left alone. A formula not closed runs to the end of src.
// ok reports whether src held a formula; shifts are where the rewrite moved
// the text after each formula, in order. The renderer draws forms from the
// rewritten text and IDs reads them from it, so their keys agree.
func MathFences(src string) (out string, ok bool, shifts []MathShift) {
	if !strings.Contains(src, "$$") {
		return src, false, nil
	}
	lines := strings.SplitAfter(src, "\n")
	var b strings.Builder
	fence := ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		text := strings.TrimRight(line, "\r\n")
		if fence != "" {
			if m := codeFenceRe.FindStringSubmatch(text); m != nil && strings.HasPrefix(m[1], fence) && strings.TrimSpace(text[len(m[0]):]) == "" {
				fence = ""
			}
			b.WriteString(line)
			continue
		}
		if m := codeFenceRe.FindStringSubmatch(text); m != nil && !(m[1][0] == '`' && strings.Contains(text[len(m[0]):], "`")) {
			fence = m[1]
			b.WriteString(line)
			continue
		}
		loc := mathFenceOpen.FindStringIndex(text)
		if loc == nil {
			b.WriteString(line)
			continue
		}
		ok = true
		start, outAt := i, b.Len()
		eol := line[len(text):]
		var body []string
		rest := text[loc[1]:]
		closed := false
		if j := closeDollars(rest); j >= 0 {
			// one line: $$ x $$
			body = append(body, strings.TrimSpace(rest[:j]))
			closed = true
		} else if strings.TrimSpace(rest) != "" {
			body = append(body, strings.TrimSpace(rest))
		}
		for !closed && i+1 < len(lines) {
			i++
			l := strings.TrimRight(lines[i], "\r\n")
			eol = lines[i][len(l):]
			if j := closeDollars(l); j >= 0 {
				if strings.TrimSpace(l[:j]) != "" {
					body = append(body, l[:j])
				}
				closed = true
				break
			}
			body = append(body, l)
		}
		code := strings.Join(body, "\n")
		f := longFence(code)
		b.WriteString(f + "math\n")
		if code != "" {
			b.WriteString(code + "\n")
		}
		b.WriteString(f)
		if eol == "" {
			eol = "\n"
		}
		b.WriteString(eol)
		read := 0
		for _, l := range lines[start : i+1] {
			read += len(l)
		}
		if d := b.Len() - outAt - read; d != 0 {
			shifts = append(shifts, MathShift{b.Len(), d})
		}
	}
	return b.String(), ok, shifts
}

// closeDollars returns where the first unescaped `$$` in s starts, or -1.
func closeDollars(s string) int {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '$' && s[i+1] == '$' {
			return i
		}
	}
	return -1
}

// longFence is a backtick fence longer than any backtick run at a line start of code.
func longFence(code string) string {
	n := 3
	for _, l := range strings.Split(code, "\n") {
		t := strings.TrimLeft(l, " ")
		k := 0
		for k < len(t) && t[k] == '`' {
			k++
		}
		if k >= n {
			n = k + 1
		}
	}
	return strings.Repeat("`", n)
}
