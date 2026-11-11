package blocks

import (
	"bytes"
	"strings"
	"testing"

	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// The parser config glamour v2.0.1 uses (GFM + DefinitionList). Auto heading
// ids are left out: they number repeated headings across the whole document,
// which says nothing about where blocks end.
var gm = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList))

func gmHTML(t testing.TB, src string) string {
	var b bytes.Buffer
	if err := gm.Convert([]byte(src), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// checkCuts fails when the HTML of the blocks, joined, differs from the HTML
// of the whole document: a cut inside a top-level node changes that node.
func checkCuts(t *testing.T, name, src string) bool {
	t.Helper()
	blocks := Split(src)
	var joined strings.Builder
	var raw strings.Builder
	for _, b := range blocks {
		joined.WriteString(gmHTML(t, b.Source))
		raw.WriteString(b.Source)
	}
	if raw.String() != src {
		t.Errorf("%s: blocks do not rebuild the source", name)
		return false
	}
	if want := gmHTML(t, src); joined.String() != want {
		t.Errorf("%s: cut inside a goldmark node (%d blocks)\nsrc:\n%q", name, len(blocks), src)
		return false
	}
	return true
}

func TestGoldmarkDifferentialDocs(t *testing.T) {
	for _, d := range corpus.Docs() {
		checkCuts(t, d.Name, d.Text)
	}
}

func TestGoldmarkDifferentialFuzz(t *testing.T) {
	for i, doc := range corpus.Fuzz(1, 300) {
		checkCuts(t, "fuzz#"+string(rune('0'+i%10)), doc)
	}
}

func TestGoldmarkDifferentialPrefixes(t *testing.T) {
	if testing.Short() {
		t.Skip("prefix loop is slow")
	}
	for _, d := range corpus.Docs() {
		seen := map[string]bool{}
		for _, set := range [][]string{corpus.LinePrefixes(d.Text), corpus.FramePrefixes(d.Text)} {
			for _, p := range set {
				if seen[p] {
					continue
				}
				seen[p] = true
				if !checkCuts(t, d.Name+" prefix", p) {
					return
				}
			}
		}
	}
}

// A definition list followed by many paragraphs: goldmark keeps the
// paragraphs out of the list only after a blank line. Without one, the next
// line is a new term or continuation.
func TestDefinitionListThenParagraphs(t *testing.T) {
	var b strings.Builder
	b.WriteString("Term\n: Definition one\n\n")
	for i := 0; i < 40; i++ {
		b.WriteString("Paragraph number ")
		b.WriteString(strings.Repeat("x", i%5+1))
		b.WriteString(".\n\n")
	}
	src := b.String()
	checkCuts(t, "deflist+paras", src)
	n := len(Split(src))
	t.Logf("deflist + 40 paragraphs: %d blocks", n)
	if n < 3 {
		t.Errorf("splitter does not cut after a definition list: %d blocks", n)
	}
	for _, s := range []string{
		"Term\n: Def\nlazy line\n\nPara\n\nPara two\n",
		"Term\n: Def\n\n: Def two\n\nPara\n\nPara two\n",
		"Term\n: Def\n\nTerm2\n: Def2\n\nPara\n\nPara two\n",
	} {
		checkCuts(t, "deflist variant", s)
	}
}

func BenchmarkSplitter(b *testing.B) {
	var doc strings.Builder
	for _, d := range corpus.Docs() {
		if strings.Contains(d.Text, "]:") {
			continue
		}
		doc.WriteString(d.Text)
		doc.WriteString("\n\n")
		if doc.Len() >= 25000 {
			break
		}
	}
	frames := corpus.FramePrefixes(doc.String())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var s Splitter
		for _, f := range frames {
			s.Split(f)
		}
	}
}
