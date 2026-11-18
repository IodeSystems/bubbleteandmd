package directive

import (
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// Marker lines inside content (Carl 2026-11-18, a form beyond flowdown: "the
// tab is just recursive content"). flowdown opens a form only at the start
// of a quote. Here a line that is a marker alone (`[!PROS]`, `[!NOTE]
// Title`), at column 0 of a form quote's content or of a form body, outside
// code fences, starts that form too, as it would at the start of a quote:
//
//   - SplitQuotes: in a quote that opens with a marker (not `[!TABS]`, whose
//     content is panels), a later marker line at the quote's own level
//     closes the quote and opens the next: a blank line goes before it.
//   - BodyForms: in a form body (a tab panel, a callout's body, an item), a
//     marker line opens a quote that takes it and the lines under it up to a
//     blank line or the next marker line.
//
// Then a pros right before a cons pairs as two quotes do.

// markerLineRe is a marker alone on its line, with its title.
var markerLineRe = regexp.MustCompile(`^\[!([A-Za-z]+)\](?:[ \t].*)?$`)

// markerKind is the form a marker line opens, lowercase; "" when text is
// not a marker line of a known form.
func markerKind(text string) string {
	m := markerLineRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	k := strings.ToLower(m[1])
	if slices.Contains(CalloutKinds, k) || slices.Contains(ContainerKinds, k) {
		return k
	}
	return ""
}

// fenceScan follows code fences line by line.
type fenceScan struct{ fence string }

// code reports whether text is code: a fence line or a line inside a fence.
func (f *fenceScan) code(text string) bool {
	m := codeFenceRe.FindStringSubmatch(text)
	if f.fence != "" {
		if m != nil && strings.HasPrefix(m[1], f.fence) && strings.TrimSpace(text[len(m[0]):]) == "" {
			f.fence = ""
		}
		return true
	}
	if m != nil && !(m[1][0] == '`' && strings.Contains(text[len(m[0]):], "`")) {
		f.fence = m[1]
		return true
	}
	return false
}

// Shifts are the shifts of rewrites applied one after the other, the first
// rewrite's first.
type Shifts [][]MathShift

// Raw is the offset in the text as written of byte rel of the rewritten text.
func (s Shifts) Raw(rel int) int {
	for i := len(s) - 1; i >= 0; i-- {
		rel = RawOffset(s[i], rel)
	}
	return rel
}

// Prepare rewrites one block as the renderer reads it: SplitQuotes, then
// MathFences, then InlineMath (keeps every offset). math reports a `$$`
// formula. IDs and the renderer both read forms from src, so their keys
// agree.
func Prepare(block string) (src string, math bool, sh Shifts) {
	src, q := SplitQuotes(block)
	src, math, m := MathFences(src)
	return InlineMath(src), math, Shifts{q, m}
}

// SplitQuotes puts a blank line before each marker line inside a quote that
// opens with a marker, other than tabs: the quote ends there and the marker
// opens the next. Only top-level quotes as goldmark reads them count (not
// `>` lines in an HTML block or a fence); lines in code fences in the quote
// are left alone, and the scan of a quote stops at its first lazy line.
func SplitQuotes(src string) (string, []MathShift) {
	if !strings.Contains(src, "[!") {
		return src, nil
	}
	d := parseDoc(src)
	var b strings.Builder
	var shifts []MathShift
	done := 0
	for _, sp := range d.top {
		if _, ok := sp.node.(*ast.Blockquote); !ok {
			continue
		}
		var in fenceScan
		at := sp.from
		for i, line := range strings.SplitAfter(d.s[sp.from:sp.to], "\n") {
			text := strings.TrimRight(line, "\r\n")
			m := quotePrefix.FindStringIndex(text)
			if m == nil {
				break
			}
			c := text[m[1]:]
			if i == 0 {
				if k := markerKind(c); k == "" || k == "tabs" {
					break
				}
			} else if !in.code(c) {
				if k := markerKind(c); k != "" {
					b.WriteString(d.s[done:at])
					b.WriteString("\n")
					shifts = append(shifts, MathShift{b.Len(), 1})
					done = at
					if k == "tabs" {
						break
					}
				}
			}
			at += len(line)
		}
	}
	if shifts == nil {
		return src, nil
	}
	b.WriteString(d.s[done:])
	return b.String(), shifts
}

// BodyForms quotes each run of a form body that starts with a marker line:
// the marker line and the lines under it up to a blank line or the next
// marker line, a blank line put before it when the line above is text.
// Lines in code fences are left alone.
func BodyForms(md string) (string, []MathShift) {
	if !strings.Contains(md, "[!") {
		return md, nil
	}
	var b strings.Builder
	var shifts []MathShift
	write := func(s string) {
		b.WriteString(s)
		shifts = append(shifts, MathShift{b.Len(), len(s)})
	}
	var top, in fenceScan
	run, blank := false, true
	for _, line := range strings.SplitAfter(md, "\n") {
		text := strings.TrimRight(line, "\r\n")
		empty := strings.TrimSpace(text) == ""
		if run {
			code := in.code(text)
			switch {
			case !code && empty:
				run = false
			case !code && markerKind(text) != "":
				write("\n")
				write("> ")
				in = fenceScan{}
			case empty:
				write(">")
			default:
				write("> ")
			}
			b.WriteString(line)
			blank = empty
			continue
		}
		if !top.code(text) && markerKind(text) != "" {
			if !blank {
				write("\n")
			}
			write("> ")
			run, in = true, fenceScan{}
		}
		b.WriteString(line)
		blank = empty
	}
	if shifts == nil {
		return md, nil
	}
	return b.String(), shifts
}
