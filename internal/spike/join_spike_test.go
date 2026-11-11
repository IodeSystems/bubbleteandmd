package spike_test

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd/internal/join"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

var spikeDocs = []struct{ kind, md string }{
	{"paragraphs", "First para with *em* and **strong** and `code`, long enough to wrap at forty columns for sure.\n\nSecond para.\nSoft break line.\n\nThird [link](http://x.y) para.\n"},
	{"atx-heading", "# One\n\ntext\n\n## Two\n\n### Three\n\n#### Four\n\ntail\n"},
	{"setext-heading", "Title\n=====\n\nbody\n\nSub\n---\n\nmore\n"},
	{"bullet-list", "- a\n- b\n- c with a long line that should wrap at forty columns for sure yes\n\nafter\n"},
	{"ordered-list", "1. one\n2. two\n3. three\n\nafter\n"},
	{"nested-list", "- a\n  - a1\n  - a2\n    1. deep\n    2. deeper\n- b\n\nafter\n"},
	{"loose-list", "- a\n\n- b\n\n- c\n\nafter\n"},
	{"blockquote", "> quote one\n> still\n\n> outer\n>\n> > inner\n> > deeper\n\nafter\n"},
	{"fenced-code", "intro\n\n```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n\n```\nplain\n```\n\nafter\n"},
	{"indented-code", "para\n\n    indented code\n    more\n\nafter\n"},
	{"table", "| a | b |\n|---|:-:|\n| 1 | 2 |\n| longer cell | x |\n\nafter\n"},
	{"hr", "above\n\n---\n\nbelow\n\n***\n\nend\n"},
	{"html-block", "before\n\n<div>\nraw html\n</div>\n\n<!-- comment -->\n\nafter\n"},
	{"deflist", "Term\n: Definition one\n: Definition two\n\nTerm2\n: Def\n\nafter\n"},
	{"linkref-def", "See [ref] and [other].\n\n[ref]: http://example.com/a\n[other]: http://example.com/b \"T\"\n\nafter [ref]\n"},
	{"heading-after-list", "- a\n- b\n\n# Heading\n\n1. x\n2. y\n## H2\ntext\n"},
	{"code-after-para", "para\n```go\nx := 1\n```\n\npara2\n\n    ind\n"},
}

func startOf(src []byte, n ast.Node) int {
	var find func(ast.Node) int
	find = func(n ast.Node) int {
		if n.Type() == ast.TypeBlock {
			if l := n.Lines(); l != nil && l.Len() > 0 {
				s := l.At(0).Start
				for s > 0 && src[s-1] != '\n' {
					s--
				}
				if _, ok := n.(*ast.FencedCodeBlock); ok && s > 0 {
					s--
					for s > 0 && src[s-1] != '\n' {
						s--
					}
				}
				return s
			}
		}
		if t, ok := n.(*ast.Text); ok {
			s := t.Segment.Start
			for s > 0 && src[s-1] != '\n' {
				s--
			}
			return s
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if s := find(c); s >= 0 {
				return s
			}
		}
		return -1
	}
	return find(n)
}

func endOf(src []byte, n ast.Node) int {
	end := -1
	_ = ast.Walk(n, func(c ast.Node, enter bool) (ast.WalkStatus, error) {
		if enter && c.Type() == ast.TypeBlock {
			if l := c.Lines(); l != nil && l.Len() > 0 {
				e := l.At(l.Len() - 1).Stop
				if _, ok := c.(*ast.FencedCodeBlock); ok {
					// skip closing fence line if present
					j := e
					for j < len(src) && src[j] != '\n' {
						j++
					}
					if j < len(src) {
						k := j + 1
						for k < len(src) && src[k] != '\n' {
							k++
						}
						ln := strings.TrimSpace(string(src[j+1 : k]))
						if strings.HasPrefix(ln, "```") || strings.HasPrefix(ln, "~~~") {
							e = k
						}
					}
				}
				end = max(end, e)
			}
		}
		return ast.WalkContinue, nil
	})
	return end
}

func cut(src []byte) ([]string, string) {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList), goldmark.WithParserOptions(parser.WithAutoHeadingID()))
	pctx := parser.NewContext()
	doc := md.Parser().Parse(text.NewReader(src), parser.WithContext(pctx))
	var refs strings.Builder
	for _, r := range pctx.References() {
		fmt.Fprintf(&refs, "[%s]: <%s>", r.Label(), r.Destination())
		if len(r.Title()) > 0 {
			fmt.Fprintf(&refs, " \"%s\"", r.Title())
		}
		refs.WriteString("\n")
	}
	var starts []int
	prevEnd := 0
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		s := startOf(src, c)
		if s < 0 { // no source lines (thematic break): first non-blank line after previous block
			s = prevEnd
			for s < len(src) {
				e := s
				for e < len(src) && src[e] != '\n' {
					e++
				}
				if strings.TrimSpace(string(src[s:e])) != "" {
					break
				}
				s = e + 1
			}
		}
		starts = append(starts, s)
		if e := endOf(src, c); e >= 0 {
			prevEnd = e
		} else {
			prevEnd = s
			for prevEnd < len(src) && src[prevEnd] != '\n' {
				prevEnd++
			}
		}
	}
	starts[0] = 0
	var out []string
	for i, s := range starts {
		e := len(src)
		if i+1 < len(starts) {
			e = starts[i+1]
		}
		out = append(out, string(src[s:e]))
	}
	return out, refs.String()
}

func firstDiff(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	lo := max(i-8, 0)
	hi := func(s string) string { return s[lo:min(i+24, len(s))] }
	return fmt.Sprintf("@%d whole=%q join=%q", i, hi(a), hi(b))
}

func TestSpikeJoin(t *testing.T) {
	styles := []string{"dark", "light", "notty"}
	widths := []int{40, 80}
	type mode struct {
		name     string
		sentinel string
		trim     bool
		fix      bool
	}
	modes := []mode{
		{"keep+sentinel", "<!-- -->\n\n", false, false},
		{"trim+sentinel", "<!-- -->\n\n", true, false},
		{"keep+none", "", false, false},
		{"fix", "<!-- -->\n\n", false, true},
	}
	tally := map[string]int{}
	total := map[string]int{}
	for _, st := range styles {
		for _, w := range widths {
			r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(st), glamour.WithWordWrap(w))
			if err != nil {
				t.Fatal(err)
			}
			empty, _ := r.Render("")
			h := len(empty) / 2
			t.Logf("%s/%d empty=%q halvesEqual=%v", st, w, empty, len(empty)%2 == 0 && empty[:h] == empty[h:])
			P, S := empty[:h], empty[h:]
			sent, _ := r.Render("<!-- -->\n")
			t.Logf("%s/%d sentinel-alone=%q", st, w, sent)
			for _, d := range spikeDocs {
				whole, err := r.Render(d.md)
				if err != nil {
					t.Fatal(err)
				}
				for _, m := range modes {
					segs, refs := cut([]byte(d.md))
					if d.kind == "linkref-def" && st == "dark" && w == 40 && m.fix {
						_ = refs
					}
					prev := ""
					var sb strings.Builder
					sb.WriteString(P)
					ok := true
					for i, s := range segs {
						if m.trim {
							s = strings.TrimRight(s, "\n") + "\n"
						}
						in := s
						if m.fix {
							in = s + "\n" + refs
						}
						if i > 0 {
							in = m.sentinel + in
						}
						out, err := r.Render(in)
						if err != nil {
							t.Fatal(err)
						}
						if !strings.HasPrefix(out, P) || !strings.HasSuffix(out, S) {
							t.Logf("NOPS %s/%d %s %s seg%d", st, w, d.kind, m.name, i)
							ok = false
							break
						}
						piece := out[len(P) : len(out)-len(S)]
						if m.fix && i > 0 && piece != "" && !strings.HasSuffix(prev, "\n") {
							lastLine := prev[strings.LastIndex(prev, "\n")+1:]
							if nl := strings.Index(piece, "\n"); nl >= 0 {
								piece = join.DropCells(piece[:nl], ansi.StringWidth(lastLine)) + piece[nl:]
							}
						}
						if piece == "" { // e.g. comment-only segment emits nothing
							continue
						}
						prev = piece
						sb.WriteString(piece)
					}
					sb.WriteString(S)
					key := m.name
					total[key]++
					if ok && sb.String() == whole {
						tally[key]++
						continue
					}
					t.Logf("MISMATCH %s/%d %-18s %-14s nseg=%d %s", st, w, d.kind, m.name, len(segs), firstDiff(whole, sb.String()))
				}
			}
		}
	}
	for k, v := range total {
		t.Logf("SUMMARY %s: %d/%d match", k, tally[k], v)
	}
}
