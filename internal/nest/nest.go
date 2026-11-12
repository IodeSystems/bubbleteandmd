// Package nest caps how deep Markdown nests, as flowdown (llx-markdown
// v0.5.0, engine/blocks.ts and engine/inlines.ts MAX_NESTING) does: a line
// that would open a block deeper than Blocks is read as text of the block
// it is in; an emphasis, strikethrough, link or image deeper than Inlines
// is kept as its text. Link destination parentheses need no cap here:
// goldmark already reads at most 32 levels (flowdown's MAX_PARENS).
//
// Why (docs/decisions.md, "Nesting caps"): glamour draws a deep blockquote
// or list in time that grows past exponential once the indents pass the
// wrap width (80 `>` at width 80: over 60 s), and it draws nested emphasis
// with Go calls per level (40 000 levels: fatal stack overflow).
package nest

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Blocks is how many blockquotes and list items may hold one another.
// flowdown allows 100; glamour cannot draw that many in time (see the
// package doc).
const Blocks = 16

// Inlines is how many emphases, strikethroughs, links and images may hold
// one another (flowdown's 100).
const Inlines = 100

// New returns goldmark as glamour's TermRenderer builds it (GFM, definition
// lists, auto heading IDs), with the caps. The caller sets its renderer.
func New() goldmark.Markdown {
	// parser.DefaultBlockParsers, with the list and blockquote parsers capped.
	blocks := []util.PrioritizedValue{
		util.Prioritized(parser.NewSetextHeadingParser(), 100),
		util.Prioritized(parser.NewThematicBreakParser(), 200),
		util.Prioritized(capped{parser.NewListParser()}, 300),
		util.Prioritized(parser.NewListItemParser(), 400),
		util.Prioritized(parser.NewCodeBlockParser(), 500),
		util.Prioritized(parser.NewATXHeadingParser(), 600),
		util.Prioritized(parser.NewFencedCodeBlockParser(), 700),
		util.Prioritized(capped{parser.NewBlockquoteParser()}, 800),
		util.Prioritized(parser.NewHTMLBlockParser(), 900),
		util.Prioritized(parser.NewParagraphParser(), 1000),
	}
	p := parser.NewParser(parser.WithBlockParsers(blocks...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		parser.WithASTTransformers(util.Prioritized(inlines{}, 100)),
	)
	return goldmark.New(goldmark.WithParser(p),
		goldmark.WithExtensions(extension.GFM, extension.DefinitionList),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()))
}

// capped opens no block where Blocks blocks already hold one another: the
// line goes on to the next parser, in the end a paragraph's text. The list
// item parser needs no cap: it opens items only in a list opened here.
type capped struct{ parser.BlockParser }

func (c capped) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	d := 0
	for n := parent; n != nil; n = n.Parent() {
		if k := n.Kind(); k == ast.KindBlockquote || k == ast.KindListItem {
			d++
		}
	}
	if d >= Blocks {
		return nil, parser.NoChildren
	}
	return c.BlockParser.Open(parent, reader, pc)
}

// inlines replaces each emphasis, strikethrough, link or image that Inlines
// others hold with the leaves under it (its text), in order. It walks with
// its own stack: the tree may be deeper than calls should go.
type inlines struct{}

func spans(n ast.Node) bool {
	switch n.Kind() {
	case ast.KindEmphasis, ast.KindLink, ast.KindImage, extast.KindStrikethrough:
		return true
	}
	return false
}

type at struct {
	n ast.Node
	d int // spans holding n
}

func (inlines) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	stack := []at{{doc, 0}}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		d := x.d
		if spans(x.n) {
			if d >= Inlines {
				flatten(x.n)
				continue
			}
			d++
		}
		for c := x.n.FirstChild(); c != nil; c = c.NextSibling() {
			stack = append(stack, at{c, d})
		}
	}
}

// flatten puts the leaves under n in n's place.
func flatten(n ast.Node) {
	parent := n.Parent()
	var leaves []ast.Node
	stack := []ast.Node{n}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if x.ChildCount() == 0 {
			if x != n {
				leaves = append(leaves, x)
			}
			continue
		}
		// children last first, so the first pops first
		for c := x.LastChild(); c != nil; c = c.PreviousSibling() {
			stack = append(stack, c)
		}
	}
	for _, l := range leaves {
		l.Parent().RemoveChild(l.Parent(), l)
		parent.InsertBefore(parent, n, l)
	}
	parent.RemoveChild(parent, n)
}
