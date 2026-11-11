package directive

import (
	"regexp"
	"strings"
	"testing"

	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// TestCorpusBlocksParse reads every block of the static corpus and of the
// generated fuzz documents: each parses to a form or to none, without panic,
// and a form's range lies in its block.
func TestCorpusBlocksParse(t *testing.T) {
	var docs []string
	for _, d := range corpus.Docs() {
		docs = append(docs, d.Text)
	}
	docs = append(docs, corpus.Fuzz(1, 200)...)
	n, forms := 0, 0
	for _, doc := range docs {
		for _, b := range blocks.Split(doc) {
			n++
			d, ok := Parse(b.Source)
			if !ok {
				continue
			}
			forms++
			if d.Start < 0 || d.Start > d.End || d.End > len(b.Source) {
				t.Errorf("block %q: range [%d,%d) outside [0,%d)", b.Source, d.Start, d.End, len(b.Source))
			}
		}
	}
	t.Logf("%d blocks, %d forms", n, forms)
}

// TestCorpusExamples: the corpus copies of the prompt's examples parse to their form.
func TestCorpusExamples(t *testing.T) {
	want := map[string]Kind{
		"example-comparison": KindTabs, "example-details": KindDetails, "example-sources": KindSources,
		"example-stats": KindStats, "example-steps": KindSteps, "example-verdict": KindVerdict,
	}
	for _, doc := range corpus.Docs() {
		kind, ok := want[doc.Name]
		if !ok {
			continue
		}
		found := false
		for _, b := range blocks.Split(doc.Text) {
			if d, ok := Parse(b.Source); ok && d.Kind == kind {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no %v block", doc.Name, kind)
		}
	}
}

// formLead is the first line of a block the forms generators of
// internal/corpus/replies.go write, and the kind it is.
var formLead = []struct {
	re   *regexp.Regexp
	kind Kind
}{
	{regexp.MustCompile(`^> \[!(?:NOTE|TIP|WARNING|IMPORTANT|CAUTION)\]$`), KindCallout},
	{regexp.MustCompile(`^> \[!STEPS\]$`), KindSteps},
	{regexp.MustCompile(`^> \[!TABS\]$`), KindTabs},
	{regexp.MustCompile(`^> \[!STATS\]$`), KindStats},
	{regexp.MustCompile(`^> \[!PROS\]$`), KindVerdict},
	{regexp.MustCompile("^```tree$"), KindTree},
}

// TestGeneratedForms: every block the callout, steps, tabs, stats, verdicts
// and tree generators wrote parses to its kind, covering the whole block.
func TestGeneratedForms(t *testing.T) {
	seen := map[Kind]int{}
	for _, sc := range corpus.Scenarios() {
		for _, doc := range append(append([]string{}, sc.History...), sc.Text) {
			for _, b := range blocks.Split(doc) {
				first, _, _ := strings.Cut(b.Source, "\n")
				first = strings.TrimSuffix(first, "\r")
				for _, f := range formLead {
					if !f.re.MatchString(first) {
						continue
					}
					d, ok := Parse(b.Source)
					if !ok || d.Kind != f.kind {
						t.Errorf("%s: block %q: got %v %v, want %v", sc.Name, b.Source, ok, d.Kind, f.kind)
						continue
					}
					seen[f.kind]++
					if strings.TrimSpace(b.Source[d.End:]) != "" {
						t.Errorf("%s: %v leaves %q", sc.Name, d.Kind, b.Source[d.End:])
					}
					if d.Kind == KindVerdict && (d.Verdict.Pros == nil || d.Verdict.Cons == nil) {
						t.Errorf("%s: verdict not paired: %q", sc.Name, b.Source)
					}
					if d.Kind == KindSteps {
						for _, s := range d.Steps.Items {
							// head() may end a title in a space: `**x **` is no bold in CommonMark.
							if s.Title == "" && !strings.Contains(s.Body, " **\n") {
								t.Errorf("%s: step %+v", sc.Name, s)
							}
							if s.Title != "" && (!strings.HasPrefix(s.When, "Day ") || s.Body == "") {
								t.Errorf("%s: step %+v", sc.Name, s)
							}
						}
					}
					// Tabs right after tabs, with no `---` between, are one run (as in llx).
					if d.Kind == KindTabs && len(d.Tabs.Panels)%3 != 0 {
						t.Errorf("%s: tabs %v", sc.Name, labels(d.Tabs))
					}
				}
			}
		}
	}
	for _, f := range formLead {
		if seen[f.kind] == 0 {
			t.Errorf("no generated %v block", f.kind)
		}
	}
	t.Logf("%v", seen)
}

// TestStreamingPrefixes parses the growing last block of a forms reply at
// every line, as a stream would: no panic, ranges in bounds.
func TestStreamingPrefixes(t *testing.T) {
	var text string
	for _, sc := range corpus.Scenarios() {
		if sc.Name == "forms-20k" {
			text = sc.Text
		}
	}
	for _, p := range corpus.LinePrefixes(text) {
		bs := blocks.Split(p)
		if len(bs) == 0 {
			continue
		}
		src := bs[len(bs)-1].Source
		if d, ok := Parse(src); ok && (d.Start > d.End || d.End > len(src)) {
			t.Fatalf("prefix block %q: range [%d,%d)", src, d.Start, d.End)
		}
	}
}
