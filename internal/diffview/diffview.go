// Package diffview reads the lines of a ```diff fence: what each line is
// (added, removed, hunk header, file header, context), which removed and
// added lines pair up, and which words of a pair changed. It does not
// render.
//
// Line kinds follow highlight.js's diff grammar, which llx-markdown uses
// (styles/code.css: additions green, deletions red, meta and comment
// faint), plus git's lowercase `index` and `\ No newline` lines.
package diffview

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind is what one diff line is.
type Kind uint8

const (
	Context Kind = iota // a line both sides have, or any other text
	Added               // "+…" (and "!…", a changed line of a context diff)
	Removed             // "-…"
	Hunk                // "@@…"
	Header              // "---…", "+++…", "*** ", "diff --git", "index ", "Index", "===", `\ ` (no newline at end)
)

// Classify returns the kind of one line (no "\n").
func Classify(line string) Kind {
	switch {
	case strings.HasPrefix(line, "@@"):
		return Hunk
	case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"),
		strings.HasPrefix(line, "*** "),
		strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "Index"), strings.HasPrefix(line, "==="),
		strings.HasPrefix(line, `\ `):
		return Header
	case strings.HasPrefix(line, "+"), strings.HasPrefix(line, "!"):
		return Added
	case strings.HasPrefix(line, "-"):
		return Removed
	}
	return Context
}

// Code reports whether lines of kind k hold code after a one-character
// marker: context, added and removed lines.
func (k Kind) Code() bool { return k == Context || k == Added || k == Removed }

// Caps on the word diff: a pair with a line longer than MaxRunes, or in a
// run (of removed or of added lines) longer than MaxRun, gets no word marks.
const (
	MaxRunes = 500
	MaxRun   = 50
)

// Pairs returns, for each line, the index of the line it pairs with, or -1.
// A run of removed lines followed directly by a run of added lines pairs
// index-wise: the i-th removed line with the i-th added line. Runs longer
// than MaxRun do not pair.
func Pairs(kinds []Kind) []int {
	out := make([]int, len(kinds))
	for i := range out {
		out[i] = -1
	}
	for i := 0; i < len(kinds); {
		if kinds[i] != Removed {
			i++
			continue
		}
		d := i
		for i < len(kinds) && kinds[i] == Removed {
			i++
		}
		a := i
		for i < len(kinds) && kinds[i] == Added {
			i++
		}
		nd, na := a-d, i-a
		if nd > MaxRun || na > MaxRun {
			continue
		}
		for k := 0; k < min(nd, na); k++ {
			out[d+k], out[a+k] = a+k, d+k
		}
	}
	return out
}

// Range is a byte range [From, To) of a line.
type Range struct{ From, To int }

// Words returns the byte ranges of a and b that the other line does not
// have: the tokens (words, runs of spaces, single other characters) left
// out of a longest common subsequence of the two token lists. nil, nil
// when either line is longer than MaxRunes, or when the lines share no
// token but spaces (nothing to point at: the whole line changed).
func Words(a, b string) (ra, rb []Range) {
	if utf8.RuneCountInString(a) > MaxRunes || utf8.RuneCountInString(b) > MaxRunes {
		return nil, nil
	}
	ta, tb := tokens(a), tokens(b)
	n, m := len(ta), len(tb)
	// lcs[i][j]: LCS length of ta[i:] and tb[j:].
	lcs := make([][]int32, n+1)
	cells := make([]int32, (n+1)*(m+1))
	for i := range lcs {
		lcs[i] = cells[i*(m+1) : (i+1)*(m+1)]
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[ta[i].From:ta[i].To] == b[tb[j].From:tb[j].To] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	shared := false
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[ta[i].From:ta[i].To] == b[tb[j].From:tb[j].To]:
			if strings.TrimSpace(a[ta[i].From:ta[i].To]) != "" {
				shared = true
			}
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ra = add(ra, ta[i])
			i++
		default:
			rb = add(rb, tb[j])
			j++
		}
	}
	for ; i < n; i++ {
		ra = add(ra, ta[i])
	}
	for ; j < m; j++ {
		rb = add(rb, tb[j])
	}
	if !shared {
		return nil, nil
	}
	return ra, rb
}

// add appends r, merged into the last range when they touch.
func add(rs []Range, r Range) []Range {
	if k := len(rs); k > 0 && rs[k-1].To == r.From {
		rs[k-1].To = r.To
		return rs
	}
	return append(rs, r)
}

// tokens cuts s into words (letters, digits, '_'), runs of spaces, and
// single other characters.
func tokens(s string) []Range {
	var out []Range
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		j := i + n
		switch {
		case word(r):
			for j < len(s) {
				r2, n2 := utf8.DecodeRuneInString(s[j:])
				if !word(r2) {
					break
				}
				j += n2
			}
		case r == ' ' || r == '\t':
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
		}
		out = append(out, Range{i, j})
		i = j
	}
	return out
}

func word(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
