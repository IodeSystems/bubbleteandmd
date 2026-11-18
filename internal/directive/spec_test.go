package directive

// The shared forms specification of flowdown v0.7.1 (spec/forms/spec.txt,
// vendored in testdata/flowdown-spec with its licence): each example's
// forms, one a line, as flowdown's test/forms.ts prints them, from the
// forms Parse finds the way the renderer and IDs find them.

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/iodesystems/bubbleteandmd/internal/blocks"
)

const specFence = "````````````````````````````````"

type specExample struct {
	n         int
	md, forms string
	line      int
}

// specExamples reads spec.txt as flowdown's suites do: a 32-backtick fence
// opened with `example`, the Markdown, a line `.`, the expected lines; `→`
// stands for a tab. Numbered from 1.
func specExamples(t *testing.T, src string) []specExample {
	t.Helper()
	var out []specExample
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], specFence+" example") {
			continue
		}
		ex := specExample{n: len(out) + 1, line: i + 1}
		var md, forms []string
		in := &md
		for i++; i < len(lines) && lines[i] != specFence; i++ {
			if lines[i] == "." && in == &md {
				in = &forms
				continue
			}
			*in = append(*in, strings.ReplaceAll(lines[i], "→", "\t"))
		}
		ex.md = strings.Join(md, "\n") + "\n"
		ex.forms = strings.Join(forms, "\n") + "\n"
		out = append(out, ex)
	}
	return out
}

// specForms prints the forms of doc as test/forms.ts does, or "(none)".
func specForms(doc string) string {
	var out []string
	specBody(doc, 0, &out)
	if len(out) == 0 {
		return "(none)\n"
	}
	return strings.Join(out, "\n") + "\n"
}

func specLine(depth int, name, title string, count int) string {
	s := strings.Repeat("  ", depth) + name
	if title != "" {
		s += fmt.Sprintf(" %q", title)
	}
	if count >= 0 {
		s += fmt.Sprintf(" %d", count)
	}
	return s
}

// specBody prints the forms of md at depth: top level as written, a body
// after BodyForms, as the renderer reads each.
func specBody(md string, depth int, out *[]string) {
	if depth > 0 {
		md, _ = BodyForms(md)
	}
	for _, b := range blocks.Split(md) {
		src, _, _ := Prepare(b.Source)
		for pos := 0; ; {
			d, ok := Parse(src[pos:])
			if !ok {
				break
			}
			specForm(d, depth, out)
			pos += d.End
		}
	}
}

func specForm(d Directive, depth int, out *[]string) {
	add := func(name, title string, count int) { *out = append(*out, specLine(depth, name, title, count)) }
	in := func(md string) {
		if md != "" {
			specBody(md, depth+1, out)
		}
	}
	switch {
	case d.Callout != nil:
		add("callout "+d.Callout.Kind, d.Callout.Title, -1)
		in(d.Callout.Body)
	case d.Details != nil:
		add("details", d.Details.Title, -1)
		in(d.Details.Body)
	case d.Steps != nil:
		add("steps", d.Steps.Title, len(d.Steps.Items))
		for _, s := range d.Steps.Items {
			in(s.Body)
		}
	case d.Stats != nil:
		add("stats", d.Stats.Title, len(d.Stats.Items))
	case d.Sources != nil:
		add("sources", d.Sources.Title, len(d.Sources.Items))
	case d.Verdict != nil:
		v := d.Verdict
		side := func(name, title string, items []Item) {
			if items == nil {
				return
			}
			add(name, title, len(items))
			for _, it := range items {
				in(it.Body)
			}
		}
		if v.ConsFirst {
			side("cons", v.ConsTitle, v.Cons)
			side("pros", v.ProsTitle, v.Pros)
		} else {
			side("pros", v.ProsTitle, v.Pros)
			side("cons", v.ConsTitle, v.Cons)
		}
	case d.Tabs != nil:
		add("tabs", d.Tabs.Title, -1)
		for _, p := range d.Tabs.Panels {
			*out = append(*out, specLine(depth+1, "tab", p.Label, -1))
			if p.Body != "" {
				specBody(p.Body, depth+2, out)
			}
		}
	}
	// Attributions, trees and titled code fences are no forms of the spec.
	in(d.After)
}

// specSkips: examples our deliberate deviations read otherwise, with why.
var specSkips = map[int]string{}

func TestFlowdownFormsSpec(t *testing.T) {
	b, err := os.ReadFile("../../testdata/flowdown-spec/spec.txt")
	if err != nil {
		t.Fatal(err)
	}
	exs := specExamples(t, string(b))
	if len(exs) == 0 {
		t.Fatal("no examples")
	}
	pass, skip := 0, 0
	for _, ex := range exs {
		if why, ok := specSkips[ex.n]; ok {
			t.Logf("example %d (line %d) skipped: %s", ex.n, ex.line, why)
			skip++
			continue
		}
		if got := specForms(ex.md); got != ex.forms {
			t.Errorf("example %d (line %d):\n%s\ngot:\n%swant:\n%s", ex.n, ex.line, ex.md, got, ex.forms)
			continue
		}
		pass++
	}
	t.Logf("%d examples: %d pass, %d skipped, %d fail", len(exs), pass, skip, len(exs)-pass-skip)
}

// The markers table: our list of markers is the spec's.
func TestFlowdownFormsSpecMarkers(t *testing.T) {
	b, err := os.ReadFile("../../testdata/flowdown-spec/spec.txt")
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile("(?m)^\\| `\\[!([A-Z]+)\\]` \\|")
	var spec []string
	for _, m := range row.FindAllStringSubmatch(string(b), -1) {
		spec = append(spec, strings.ToLower(m[1]))
	}
	ours := append(slices.Clone(CalloutKinds), ContainerKinds...)
	slices.Sort(spec)
	slices.Sort(ours)
	if !slices.Equal(spec, ours) {
		t.Errorf("markers: spec %v, ours %v", spec, ours)
	}
}
