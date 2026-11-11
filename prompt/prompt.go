// Ported from llx-markdown packages/llx-markdown/src/prompt/index.ts and src/dialect.ts (MIT, see NOTICE).

// Package prompt holds the instructions that teach a model to write the
// directive forms the renderer draws. A model writes the forms it was told
// about and no others. The text is plain, the same on every call with the same
// options, so it fits a cached prompt prefix.
package prompt

import (
	"fmt"
	"strings"
)

// DefaultHeader is the section's default heading line.
const DefaultHeader = "# Answer formatting"

// Options select what the prompt says. The zero value is the default prompt.
type Options struct {
	// Header is the section's heading line. Empty: DefaultHeader.
	Header string
	// Math mentions $…$ math. Off by default; turn it on only if the
	// renderer shows math.
	Math bool
	// Diagrams teaches ```mermaid diagrams. Off by default; turn it on only
	// if the renderer draws them (this module shows mermaid as code).
	Diagrams bool
}

// calloutKinds are GitHub's alert kinds.
var calloutKinds = []string{"note", "tip", "important", "warning", "caution"}

// diagramKinds are the Mermaid types that read well at chat width.
var diagramKinds = []string{
	"flowchart", "sequenceDiagram", "stateDiagram-v2", "erDiagram", "gantt", "pie",
	"xychart-beta", "classDiagram", "mindmap", "timeline", "quadrantChart",
	"sankey-beta", "radar-beta", "treemap-beta", "gitGraph",
}

// Examples are the forms the prompt shows, one string of lines each. They are
// shown, not described: a model told only about a form keeps writing the
// table or numbered list it writes by habit.
var Examples = map[string]string{
	"comparison": strings.Join([]string{
		"> [!TABS]",
		"#### PostgreSQL",
		"> [!PROS]",
		"- concurrent writes",
		"",
		"> [!CONS]",
		"- needs a server",
		"",
		"---",
	}, "\n"),
	"verdict": strings.Join([]string{
		"> [!PROS]",
		"- typed APIs",
		"",
		"> [!CONS]",
		"- a build step",
	}, "\n"),
	"steps": strings.Join([]string{
		"> [!STEPS]",
		"1. **First 24 hours**",
		"   *Day 1*",
		"   Rest and keep warm.",
	}, "\n"),
	"stats": strings.Join([]string{
		"> [!STATS]",
		"- **3×** faster installs",
		"- **−40%** memory",
	}, "\n"),
	"details": strings.Join([]string{
		"> [!DETAILS] Full log",
		"> The long part, quoted.",
	}, "\n"),
	"sources": strings.Join([]string{
		"> [!SOURCES]",
		"- [Page title](https://example.com/page)",
	}, "\n"),
}

// shown puts an example under its rule line, two spaces in, blank lines blank.
func shown(example string) string {
	lines := strings.Split(example, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "\n")
}

func headerOf(h string) string {
	if h == "" {
		return DefaultHeader
	}
	return h
}

// FormsPrinciple is one line for the rules at the top of a system prompt.
// Described only in a section further down, the forms read as an optional
// reference. header is the section's heading line ("" for DefaultHeader).
func FormsPrinciple(header string) string {
	name := strings.TrimLeft(headerOf(header), "# \t")
	return fmt.Sprintf(`- Shape every reply with the forms in "%s": the page renders them, and a plain list or table where a form fits is a formatting defect.`, name)
}

// Format returns the section that teaches the directive forms, ready to place
// in a system prompt.
func Format(o Options) string {
	header := headerOf(o.Header)
	var markers []string
	for _, k := range calloutKinds {
		markers = append(markers, "[!"+strings.ToUpper(k)+"]")
	}
	math := ""
	if o.Math {
		math = " with $…$ math"
	}
	lines := []string{
		header,
		FormsPrinciple(header),
		"Replies render as GitHub Markdown" + math + " plus the forms below; the user's style preferences override them.",
		"Match the content to the first form that fits; a table or list is the fallback. Write a marker as its own quote line, the content unquoted after it.",
		"- Lead with the answer. Short replies use prose; long ones use `##` per thread, short paragraphs and selective bold.",
		"- Options compared (X vs Y, which to use) or variants (per OS, language): a tab per option, `#### Label`; a compared option holds its sides. After them a `---`, then one line on when to pick which:\n" + shown(Examples["comparison"]),
		"- One option weighed (should I, is it worth it): its sides, then the verdict:\n" + shown(Examples["verdict"]),
		"- Stages in order (a plan, recovery, schedule, by day, a procedure): steps, never a numbered list:\n" + shown(Examples["steps"]),
		"- Headline figures (speedups, sizes, key numbers): stats:\n" + shown(Examples["stats"]),
		"- Exact values across options (specs, prices, limits): a table.",
		"- A risk or key aside: one or two callouts, quotes starting `> [!KIND]` (" + strings.Join(markers, " ") + "); text after the marker titles it.",
		"- Long secondary material (a full log, raw output, a derivation): folded, its content quoted:\n" + shown(Examples["details"]),
		"- Pages the reply draws on: sources, last:\n" + shown(Examples["sources"]),
		"- A directory layout: a ```tree fence of `├──`/`└──` lines. Things to do: `- [ ] item`.",
		"- Code for a file names it: ```ts title=\"src/app.ts\". A change to code: a ```diff fence.",
	}
	if o.Diagrams {
		lines = append(lines,
			"- Draw what a picture explains better in a ```mermaid fence ("+strings.Join(diagramKinds, ", ")+"); quote labels with punctuation.",
			"- Draw proactively for branching flows, structure and data; prefer one useful picture over ASCII art. Use supplied data; label illustrative data. Never claim an unsupported chart type.",
		)
	}
	lines = append(lines, "- Quote attribution: `> — Author`. Image caption: `![alt](url \"Caption\")`.")
	return strings.Join(lines, "\n")
}
