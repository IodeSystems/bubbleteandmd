package directive

import "testing"

// Cases not in llx-markdown's tests: the byte ranges goldmark does not keep,
// and rules of the TS code its tests do not reach.

func TestEdgeConsBeforePros(t *testing.T) {
	d := mustParse(t, "> [!CONS]\n- a\n\n> [!PROS] Good\n- b\n- c", KindVerdict)
	eq(t, "verdict", *d.Verdict, Verdict{ProsTitle: "Good", Pros: []Item{{"b"}, {"c"}}, Cons: []Item{{"a"}}, ConsFirst: true})
}

func TestEdgeQuotedListThenProse(t *testing.T) {
	d := mustParse(t, "> [!STEPS]\n> 1. **A**\n>    x\n>\n> After para.", KindSteps)
	eq(t, "after", d.After, "After para.")
	eq(t, "items", d.Steps.Items, []Step{{Title: "A", Body: "x"}})
	// A side with content after its list pairs with nothing (TS: it is no bare aside).
	d = mustParse(t, "> [!PROS]\n> - a\n>\n> note\n\n> [!CONS]\n> - b", KindVerdict)
	eq(t, "cons", d.Verdict.Cons, []Item(nil))
}

func TestEdgeCRLF(t *testing.T) {
	d := mustParse(t, "> [!STEPS]\r\n> 1. **A** *w*\r\n>    body\r\n", KindSteps)
	eq(t, "items", d.Steps.Items, []Step{{"A", "w", "body"}})
}

func TestEdgeEmptyFenceThenBreakInTabs(t *testing.T) {
	src := "> [!TABS]\n## A\n```\n```\n***\nx"
	d := mustParse(t, src, KindTabs)
	eq(t, "body", d.Tabs.Panels[0].Body, "```\n```")
	eq(t, "rest", src[d.End:], "***\nx")
}

func TestEdgeLazyCalloutAndLeadingText(t *testing.T) {
	d := mustParse(t, "> [!NOTE]\n> line one\nlazy two", KindCallout)
	eq(t, "body", d.Callout.Body, "line one\nlazy two")
	src := "intro\n> [!TIP]\n> x"
	d = mustParse(t, src, KindCallout)
	eq(t, "start", src[d.Start:], "> [!TIP]\n> x")
}

func TestEdgeSourceLinkForms(t *testing.T) {
	d := mustParse(t, "> [!SOURCES]\n- www.x.com — note\n- <https://y.org/p> hi\n- [**B** `c`](http://b.io/x \"t\") · n", KindSources)
	eq(t, "items", d.Sources.Items, []Source{
		{"www.x.com", "http://www.x.com", "x.com", "note"},
		{"y.org", "https://y.org/p", "", "hi"},
		{"**B** `c`", "http://b.io/x", "b.io", "n"},
	})
}

func TestEdgeStepBodyKeepsBlocks(t *testing.T) {
	d := mustParse(t, "> [!STEPS]\n1. **A**\n   - nested\n   - more\n\n   ```\n   code\n   ```\n2. **B**", KindSteps)
	eq(t, "items", d.Steps.Items, []Step{{Title: "A", Body: "- nested\n- more\n\n```\ncode\n```"}, {Title: "B"}})
}

func TestEdgeTitledCodeBody(t *testing.T) {
	d := mustParse(t, "  ```go main.go\n  x := 1\n    y\n  ```", KindCodeTitle)
	eq(t, "code", *d.CodeTitle, CodeTitle{"main.go", "go", "x := 1\n  y\n"})
}

// TestEdgeTabContinuation: a tab in an item's continuation indent counts to
// the next 4-column stop (CommonMark), so a tab-indented paragraph stays a
// paragraph, not indented code.
func TestEdgeTabContinuation(t *testing.T) {
	d := mustParse(t, "> [!STEPS]\n\n- **One** step\n\n\tcontinued para\n- **Two** x\n", KindSteps)
	eq(t, "items", d.Steps.Items, []Step{{"One", "", "step\n\n  continued para"}, {"Two", "", "x"}})
	d = mustParse(t, "> [!STEPS]\n\n1.\t**One** step\n\n\tmore\n", KindSteps)
	eq(t, "items", d.Steps.Items, []Step{{"One", "", "step\n\nmore"}})
	for _, c := range []struct{ in, want string }{
		{"- a\n\n\tb\n", "a\n\n  b\n"},
		{"- a\n\n  \tb\n", "a\n\n  b\n"},
		{"1.\ta\n\n\tb\n", "a\n\nb\n"},
		{"- a\n\n\t\tcode\n", "a\n\n      code\n"},
		{"- a\n\n  b\n", "a\n\nb\n"},
	} {
		eq(t, "dedent "+c.in, dedentItem(c.in), c.want)
	}
}
