// Ported from llx-markdown packages/llx-markdown/src/react/forms.test.tsx and
// src/dialect.test.ts (MIT, see NOTICE). The TS cases assert rendered HTML;
// these assert what Parse reads, which is what that HTML is drawn from.

package directive

import (
	"reflect"
	"strings"
	"testing"

	"github.com/iodesystems/bubbleteandmd/internal/blocks"
)

func mustParse(t *testing.T, src string, kind Kind) Directive {
	t.Helper()
	d, ok := Parse(src)
	if !ok {
		t.Fatalf("Parse(%q): no form, want %v", src, kind)
	}
	if d.Kind != kind {
		t.Fatalf("Parse(%q).Kind = %v, want %v", src, d.Kind, kind)
	}
	return d
}

func noForm(t *testing.T, src string) {
	t.Helper()
	if d, ok := Parse(src); ok {
		t.Fatalf("Parse(%q) = %v form, want none", src, d.Kind)
	}
}

// kinds splits doc into blocks and returns the kind each parses to.
func kinds(doc string) []Kind {
	var out []Kind
	for _, b := range blocks.Split(doc) {
		if d, ok := Parse(b.Source); ok {
			out = append(out, d.Kind)
		} else {
			out = append(out, KindNone)
		}
	}
	return out
}

func eq[T any](t *testing.T, what string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", what, got, want)
	}
}

func labels(tabs *Tabs) []string {
	var out []string
	for _, p := range tabs.Panels {
		out = append(out, p.Label)
	}
	return out
}

// forms.test.tsx: tabs (`> [!TABS]` with a heading per variant)

func TestTabsEachHeadingIsATab(t *testing.T) {
	src := strings.Join([]string{"> [!TABS]", "> #### macOS", "> brew install llx", "> #### Linux", "> curl -fsSL get.llx.sh | sh"}, "\n")
	d := mustParse(t, src, KindTabs)
	eq(t, "labels", labels(d.Tabs), []string{"macOS", "Linux"})
	eq(t, "bodies", []string{d.Tabs.Panels[0].Body, d.Tabs.Panels[1].Body}, []string{"brew install llx", "curl -fsSL get.llx.sh | sh"})
	eq(t, "range", [2]int{d.Start, d.End}, [2]int{0, len(src)})
}

func TestTabsWithoutHeadingStaysQuote(t *testing.T) {
	noForm(t, "> [!TABS]\n> just text")
}

func TestTabsFormattedHeadingLabel(t *testing.T) {
	d := mustParse(t, "> [!TABS]\n#### **macOS** `arm64`\nOne.\n#### [Linux](https://example.com)\nTwo.", KindTabs)
	eq(t, "labels", labels(d.Tabs), []string{"macOS arm64", "Linux"})
	eq(t, "body", d.Tabs.Panels[1].Body, "Two.")
}

// forms.test.tsx: headline figures (`> [!STATS]`)

func TestStatsValueOverLabel(t *testing.T) {
	d := mustParse(t, "> [!STATS]\n> - **3×** faster builds\n> - **−40%** memory", KindStats)
	eq(t, "items", d.Stats.Items, []Stat{{"3×", "faster builds"}, {"−40%", "memory"}})
}

// forms.test.tsx: the case for and against (`> [!PROS]` / `> [!CONS]`)

func TestVerdictPairSideBySide(t *testing.T) {
	src := "> [!PROS]\n> - fast\n\n> [!CONS]\n> - costly"
	d := mustParse(t, src, KindVerdict)
	eq(t, "verdict", *d.Verdict, Verdict{Pros: []Item{{"fast"}}, Cons: []Item{{"costly"}}})
	eq(t, "end", d.End, len(src))
}

func TestVerdictAloneWithTitle(t *testing.T) {
	d := mustParse(t, "> [!CONS] Risks\n> - lock-in", KindVerdict)
	eq(t, "verdict", *d.Verdict, Verdict{ConsTitle: "Risks", Cons: []Item{{"lock-in"}}})
	eq(t, "title", d.Title, "Risks")
}

// forms.test.tsx: file tree (a ```tree fence)

var treeSrc = strings.Join([]string{"src/", "├── app.ts        # entry point", "└── features/", "    └── chat/"}, "\n")

func TestTreeDepthFoldersComments(t *testing.T) {
	rows := ParseTree(treeSrc)
	eq(t, "rows", rows, []TreeRow{
		{Depth: 0, Name: "src/", Folder: true, Path: "src/#0", Parent: -1},
		{Depth: 1, Name: "app.ts", Comment: "entry point", Path: "src/app.ts#1", Parent: 0},
		{Depth: 1, Name: "features/", Folder: true, Path: "src/features/#2", Parent: 0},
		{Depth: 2, Name: "chat/", Folder: true, Path: "src/features/chat/#3", Parent: 2},
	})
}

func TestTreeFoldersAreFoldable(t *testing.T) {
	d := mustParse(t, "```tree\n"+treeSrc+"\n```", KindTree)
	folders := 0
	for _, r := range d.Tree.Rows {
		if r.Folder {
			folders++
		}
	}
	eq(t, "rows", len(d.Tree.Rows), 4)
	eq(t, "folders", folders, 3)
}

func TestTreeRowsNotBoxCharacters(t *testing.T) {
	d := mustParse(t, "```tree\n"+treeSrc+"\n```", KindTree)
	eq(t, "comment", d.Tree.Rows[1].Comment, "entry point")
	for _, r := range d.Tree.Rows {
		if strings.Contains(r.Name, "├──") {
			t.Errorf("row %q keeps its branch", r.Name)
		}
	}
}

// forms.test.tsx: checklists — GFM task lists, not a form of this package.

func TestChecklistIsNoForm(t *testing.T) {
	noForm(t, "- [ ] backup the database\n- [x] tag the release")
}

// forms.test.tsx: a marker-only quote governs the blocks right after it

func TestMarkerOnlyTabsTakeHeadings(t *testing.T) {
	d := mustParse(t, "> [!TABS]\n#### macOS\nUse fnm.\n\n```sh\nfnm install --lts\n```\n\n#### Linux\nSame on Linux.", KindTabs)
	eq(t, "labels", labels(d.Tabs), []string{"macOS", "Linux"})
	eq(t, "body", d.Tabs.Panels[0].Body, "Use fnm.\n\n```sh\nfnm install --lts\n```")
}

func TestMarkerOnlyVerdictsInsideTabs(t *testing.T) {
	src := strings.Join([]string{
		"> [!TABS]", "#### PostgreSQL", "> [!PROS]", "- concurrent writes", "", "> [!CONS]", "- needs a server", "",
		"#### SQLite", "> [!PROS]", "- one file",
	}, "\n")
	d := mustParse(t, src, KindTabs)
	eq(t, "labels", labels(d.Tabs), []string{"PostgreSQL", "SQLite"})
	pg := mustParse(t, d.Tabs.Panels[0].Body, KindVerdict)
	eq(t, "postgres", *pg.Verdict, Verdict{Pros: []Item{{"concurrent writes"}}, Cons: []Item{{"needs a server"}}})
	lite := mustParse(t, d.Tabs.Panels[1].Body, KindVerdict)
	eq(t, "sqlite", *lite.Verdict, Verdict{Pros: []Item{{"one file"}}})
}

func TestMarkerOnlyTakesOneList(t *testing.T) {
	src := "> [!STEPS]\n1. **Day 1**\n   Rest.\n\nAfterwards, walk."
	d := mustParse(t, src, KindSteps)
	eq(t, "rest", src[d.End:], "Afterwards, walk.")
	eq(t, "step", d.Steps.Items, []Step{{Title: "Day 1", Body: "Rest."}})
	tabs := "> [!TABS]\n#### A\nOne.\n\n---\n\nAfter."
	d = mustParse(t, tabs, KindTabs)
	eq(t, "rest", tabs[d.End:], "---\n\nAfter.")
	eq(t, "body", d.Tabs.Panels[0].Body, "One.")
}

// answerFormExamples are dialect.ts ANSWER_FORM_EXAMPLES, the text the prompt shows.
var answerFormExamples = map[string][]string{
	"comparison": {"> [!TABS]", "#### PostgreSQL", "> [!PROS]", "- concurrent writes", "", "> [!CONS]", "- needs a server", "", "---"},
	"verdict":    {"> [!PROS]", "- typed APIs", "", "> [!CONS]", "- a build step"},
	"steps":      {"> [!STEPS]", "1. **First 24 hours**", "   *Day 1*", "   Rest and keep warm."},
	"stats":      {"> [!STATS]", "- **3×** faster installs", "- **−40%** memory"},
	"details":    {"> [!DETAILS] Full log", "> The long part, quoted."},
	"sources":    {"> [!SOURCES]", "- [Page title](https://example.com/page)"},
}

func TestEveryPromptExampleIsItsForm(t *testing.T) {
	for name, lines := range answerFormExamples {
		src := strings.Join(lines, "\n")
		d, ok := Parse(src)
		if !ok {
			t.Errorf("%s: no form", name)
			continue
		}
		switch name {
		case "comparison":
			eq(t, name, d.Kind, KindTabs)
			v := mustParse(t, d.Tabs.Panels[0].Body, KindVerdict)
			if v.Verdict.Pros == nil || v.Verdict.Cons == nil {
				t.Errorf("%s: verdicts not paired: %+v", name, v.Verdict)
			}
			eq(t, name+" rest", src[d.End:], "---")
		case "verdict":
			eq(t, name, d.Kind, KindVerdict)
			eq(t, name, *d.Verdict, Verdict{Pros: []Item{{"typed APIs"}}, Cons: []Item{{"a build step"}}})
		case "steps":
			eq(t, name, d.Kind, KindSteps)
			eq(t, name, d.Steps.Items, []Step{{"First 24 hours", "Day 1", "Rest and keep warm."}})
		case "stats":
			eq(t, name, d.Kind, KindStats)
			eq(t, name, d.Stats.Items[0], Stat{"3×", "faster installs"})
		case "details":
			eq(t, name, d.Kind, KindDetails)
			eq(t, name, *d.Details, Details{"Full log", "The long part, quoted."})
		case "sources":
			eq(t, name, d.Kind, KindSources)
			eq(t, name, d.Sources.Items[0], Source{"Page title", "https://example.com/page", "example.com", ""})
		}
		if d.End != len(src) && name != "comparison" {
			t.Errorf("%s: form ends at %d of %d", name, d.End, len(src))
		}
	}
}

func TestMarkerWithNothingToGovernStaysQuote(t *testing.T) {
	noForm(t, "> [!STATS]\n\nJust prose.")
}

// forms.test.tsx: a reply that also carries raw HTML

func TestRawHTMLKeepsDialect(t *testing.T) {
	got := kinds("<b>note</b>\n\n> [!STEPS]\n> 1. **Day 1**\n>    Rest.\n\n> [!TIP]\n> Cache it.")
	eq(t, "kinds", got, []Kind{KindNone, KindSteps, KindCallout})
}

// forms.test.tsx: steps (`> [!STEPS]` over a list)

var recovery = strings.Join([]string{
	"> [!STEPS] Recovery after surgery",
	"> 1. **First 24 hours**",
	">    *Day 1*",
	">    Coming out of anaesthesia; keep her warm and still.",
	"> 2. **Activity returns**",
	">    *Days 2–3*",
	">    Appetite comes back; keep walks short.",
}, "\n")

func TestStepsTimelineWithTitle(t *testing.T) {
	d := mustParse(t, recovery, KindSteps)
	eq(t, "title", d.Steps.Title, "Recovery after surgery")
	eq(t, "ordered", [2]any{d.Steps.Ordered, d.Steps.Start}, [2]any{true, 1})
	eq(t, "range", [2]int{d.Start, d.End}, [2]int{0, len(recovery)})
}

func TestStepsHeadAndText(t *testing.T) {
	d := mustParse(t, recovery, KindSteps)
	eq(t, "items", d.Steps.Items, []Step{
		{"First 24 hours", "Day 1", "Coming out of anaesthesia; keep her warm and still."},
		{"Activity returns", "Days 2–3", "Appetite comes back; keep walks short."},
	})
}

func TestStepsWhenAfterDashOrAbsent(t *testing.T) {
	d := mustParse(t, strings.Join([]string{"> [!STEPS]", "> - **Install** — *5 min*", ">   Run the installer.", "> - **Verify**"}, "\n"), KindSteps)
	eq(t, "ordered", d.Steps.Ordered, false)
	eq(t, "title", d.Steps.Title, "")
	eq(t, "items", d.Steps.Items, []Step{{"Install", "5 min", "Run the installer."}, {Title: "Verify"}})
}

func TestBoldLedListIsNotSteps(t *testing.T) {
	noForm(t, "1. **Pros:** fast\n2. **Cons:** costly")
}

func TestStepsMarkerWithoutListStaysQuote(t *testing.T) {
	noForm(t, "> [!STEPS]\n> Just a sentence.")
}

// forms.test.tsx: a folded section (`> [!DETAILS]`)

func TestDetailsTitleAndWholeContent(t *testing.T) {
	d := mustParse(t, "> [!DETAILS] Build log\n> First line.\n>\n> ```sh\n> bun test\n> ```", KindDetails)
	eq(t, "details", *d.Details, Details{"Build log", "First line.\n\n```sh\nbun test\n```"})
}

func TestDetailsWithoutTitle(t *testing.T) {
	d := mustParse(t, "> [!DETAILS]\n> Hidden.", KindDetails)
	eq(t, "details", *d.Details, Details{"", "Hidden."})
}

func TestDetailsFormattedTitleIsContent(t *testing.T) {
	d := mustParse(t, "> [!DETAILS] **Bold** title\n> Body.", KindDetails)
	eq(t, "details", *d.Details, Details{"", "**Bold** title\nBody."})
	d = mustParse(t, "> [!DETAILS] Steps taken\n> - one\n> - two", KindDetails)
	eq(t, "details", *d.Details, Details{"Steps taken", "- one\n- two"})
}

func TestDetailsFoldsOnlyQuoted(t *testing.T) {
	src := "> [!DETAILS] More\n> Inside.\n\n- outside"
	d := mustParse(t, src, KindDetails)
	eq(t, "body", d.Details.Body, "Inside.")
	eq(t, "rest", src[d.End:], "- outside")
}

func TestDetailsHoldsCallout(t *testing.T) {
	got := kinds("<b>x</b>\n\n> [!DETAILS] Why\n> > [!TIP]\n> > Cache it.")
	eq(t, "kinds", got, []Kind{KindNone, KindDetails})
	d := mustParse(t, "> [!DETAILS] Why\n> > [!TIP]\n> > Cache it.", KindDetails)
	c := mustParse(t, d.Details.Body, KindCallout)
	eq(t, "callout", *c.Callout, Callout{"tip", "", "Cache it."})
}

// forms.test.tsx: references (`> [!SOURCES]` over a list of links)

func TestSourcesTitleSiteNote(t *testing.T) {
	d := mustParse(t, "> [!SOURCES] Further reading\n- [CommonMark spec](https://www.commonmark.org/spec) — the grammar\n- [Local notes](./notes.md)", KindSources)
	eq(t, "title", d.Sources.Title, "Further reading")
	eq(t, "items", d.Sources.Items, []Source{
		{"CommonMark spec", "https://www.commonmark.org/spec", "commonmark.org", "the grammar"},
		{"Local notes", "./notes.md", "", ""},
	})
}

func TestSourcesBareURLNamedBySite(t *testing.T) {
	d := mustParse(t, "> [!SOURCES]\n- https://example.com/a/b", KindSources)
	eq(t, "items", d.Sources.Items, []Source{{"example.com", "https://example.com/a/b", "", ""}})
	d = mustParse(t, "> [!SOURCES]\n- [https://example.com/a](https://example.com/a)", KindSources)
	eq(t, "items", d.Sources.Items, []Source{{"example.com", "https://example.com/a", "", ""}})
}

func TestSourcesPlainItemStaysLine(t *testing.T) {
	got := kinds("<b>x</b>\n\n> [!SOURCES]\n- plain\n- [A](https://a.dev)")
	eq(t, "kinds", got, []Kind{KindNone, KindSources})
	d := mustParse(t, "> [!SOURCES]\n- plain\n- [A](https://a.dev)", KindSources)
	eq(t, "items", d.Sources.Items, []Source{{Note: "plain"}, {"A", "https://a.dev", "a.dev", ""}})
}

// forms.test.tsx: a code fence that names its file

func TestCodeFenceTitle(t *testing.T) {
	for _, info := range []string{`ts title="src/app.ts"`, `ts filename='src/app.ts'`, `ts src/app.ts`} {
		d := mustParse(t, "```"+info+"\nconst a = 1\n```", KindCodeTitle)
		eq(t, info, *d.CodeTitle, CodeTitle{"src/app.ts", "ts", "const a = 1\n"})
	}
	noForm(t, "```ts {1,3} showLineNumbers\nconst a = 1\n```")
	noForm(t, "```ts\nconst a = 1\n```")
}

func TestCodeFenceTitleWhileOpen(t *testing.T) {
	got := kinds("<b>x</b>\n\n```ts title=\"a.ts\"\nconst a = 1\n```")
	eq(t, "kinds", got, []Kind{KindNone, KindCodeTitle})
	d := mustParse(t, "```ts title=\"a.ts\"\nconst a = 1", KindCodeTitle)
	eq(t, "title", d.Title, "a.ts")
}

// forms.test.tsx: a callout title on the marker line

func TestCalloutTitleWhenBodyOpensWithFormatting(t *testing.T) {
	d := mustParse(t, "> [!CAUTION] Data loss\n> `--force` drops the column.", KindCallout)
	eq(t, "callout", *d.Callout, Callout{"caution", "Data loss", "`--force` drops the column."})
}

func TestCalloutFormattedMarkerLineIsBody(t *testing.T) {
	d := mustParse(t, "> [!NOTE] **Do not** run it twice", KindCallout)
	eq(t, "callout", *d.Callout, Callout{"note", "", "**Do not** run it twice"})
}

// dialect.test.ts: parseCalloutMarker

func TestCalloutEveryKindAnyCase(t *testing.T) {
	for _, kind := range CalloutKinds {
		d := mustParse(t, "> [!"+strings.ToUpper(kind)+"]\n> Body", KindCallout)
		eq(t, kind, *d.Callout, Callout{kind, "", "Body"})
	}
	d := mustParse(t, "> [!Tip] body", KindCallout)
	eq(t, "kind", d.Callout.Kind, "tip")
}

func TestCalloutTitleIsMarkerLine(t *testing.T) {
	d := mustParse(t, "> [!WARNING] Key rotation\n> Every client logs out.", KindCallout)
	eq(t, "callout", *d.Callout, Callout{"warning", "Key rotation", "Every client logs out."})
}

func TestCalloutUnknownOrMidLine(t *testing.T) {
	noForm(t, "> [!FOO]\n> Body")
	noForm(t, "> See [!NOTE] later")
	noForm(t, "> [NOTE]\n> Body")
}

// dialect.test.ts: ATTRIBUTION_PREFIX

func TestAttributionPrefix(t *testing.T) {
	for _, src := range []string{"> Quote.\n> — Ada Lovelace", "> Quote.\n> -- Ada Lovelace"} {
		d := mustParse(t, src, KindAttribution)
		eq(t, src, *d.Attribution, Attribution{"Quote.", "Ada Lovelace"})
	}
	for _, src := range []string{"> Quote.\n> —Ada", "> text — more", "> Quote.\n> - a list item"} {
		noForm(t, src)
	}
}

// remark-answer-blocks.ts applyAttribution: further cases of its comments.

func TestAttributionOwnParagraphAndSourceOnly(t *testing.T) {
	d := mustParse(t, "> Simplicity is prerequisite.\n>\n> — Edsger W. Dijkstra", KindAttribution)
	eq(t, "attribution", *d.Attribution, Attribution{"Simplicity is prerequisite.", "Edsger W. Dijkstra"})
	d = mustParse(t, "> Quote,  \n> -- *Ada*", KindAttribution)
	eq(t, "attribution", *d.Attribution, Attribution{"Quote,", "*Ada*"})
	// A quote that is nothing but its source line has no quotation to attribute.
	noForm(t, "> — Ada")
	// A callout keeps its dash line as body.
	d = mustParse(t, "> [!NOTE]\n> Quote.\n> — Ada", KindCallout)
	eq(t, "body", d.Callout.Body, "Quote.\n— Ada")
}

func TestSectionNumber(t *testing.T) {
	for _, c := range []struct{ in, num, title string }{
		{"1.2 Water temperature", "1.2", "Water temperature"},
		{"3. Results", "3.", "Results"},
	} {
		num, title, ok := SectionNumber(c.in)
		eq(t, c.in, [3]any{num, title, ok}, [3]any{c.num, c.title, true})
	}
	for _, in := range []string{"2024", "1.5x faster builds", "Intro"} {
		if _, _, ok := SectionNumber(in); ok {
			t.Errorf("SectionNumber(%q) ok", in)
		}
	}
}

func TestHeadingIDs(t *testing.T) {
	got := Headings("## Normal\n\ntext\n\n## Normal\n\n# **Bold** `x`\n\n## ???")
	eq(t, "headings", got, []Heading{
		{2, "Normal", "normal", 0},
		{2, "Normal", "normal-1", 17},
		{1, "Bold x", "bold-x", 28},
		{2, "???", "section", 44},
	})
}
