// Ported from llx-markdown packages/llx-markdown/src/core/headings.ts and the
// SECTION_NUMBER rule of src/core/remark-answer-blocks.ts (MIT, see NOTICE).

package directive

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
)

var (
	sectionNumberRe = regexp.MustCompile(`^(\d+(?:\.\d+)*\.?)[ \t]+`)
	slugStrip       = regexp.MustCompile("[`*_~]")
	slugRun         = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

// SectionNumber splits a number the author wrote at the start of a heading's
// text (`1.2 Water temperature`) from its title. False when there is none, or
// nothing after it (`2024`, `1.5x faster` are not numbers).
func SectionNumber(text string) (number, title string, ok bool) {
	m := sectionNumberRe.FindStringSubmatch(text)
	if m == nil || len(m[0]) == len(text) {
		return "", text, false
	}
	return m[1], text[len(m[0]):], true
}

// Slug is the id a heading's text gives: lowercase, formatting characters
// dropped, every other run of non-letters and non-digits one `-`.
func Slug(text string) string {
	s := slugStrip.ReplaceAllString(strings.ToLower(text), "")
	return strings.Trim(slugRun.ReplaceAllString(s, "-"), "-")
}

// Heading is one heading of a document's outline.
type Heading struct {
	Depth  int
	Text   string
	ID     string
	Offset int // byte offset of the heading's line
}

// Headings returns every heading in md with a unique id; a repeated id is
// numbered (`setup`, `setup-1`).
func Headings(md string) []Heading {
	d := parseDoc(md)
	var out []Heading
	used := map[string]bool{}
	_ = ast.Walk(d.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		text := d.labelText(h)
		base := Slug(text)
		if base == "" {
			base = "section"
		}
		id := base
		for i := 1; used[id]; i++ {
			id = base + "-" + strconv.Itoa(i)
		}
		used[id] = true
		offset := 0
		if first, _, ok := d.extent(h); ok {
			offset = d.lineStart(first)
		}
		out = append(out, Heading{Depth: h.Level, Text: text, ID: id, Offset: offset})
		return ast.WalkSkipChildren, nil
	})
	return out
}
