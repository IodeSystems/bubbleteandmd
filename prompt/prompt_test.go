package prompt

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/iodesystems/bubbleteandmd/internal/directive"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestDefaultGolden(t *testing.T) {
	const path = "testdata/default.golden"
	got := Format(Options{})
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("default prompt differs from %s (run with -update)", path)
	}
}

// examplesIn returns each indented run under a rule line, as a model reads it.
func examplesIn(p string) []string {
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.TrimSpace(strings.Join(cur, "\n")))
			cur = nil
		}
	}
	for _, l := range strings.Split(p, "\n") {
		switch {
		case strings.HasPrefix(l, "  "):
			cur = append(cur, l[2:])
		case l == "" && len(cur) > 0:
			cur = append(cur, "")
		default:
			flush()
		}
	}
	flush()
	return out
}

func TestExamplesParse(t *testing.T) {
	ex := examplesIn(Format(Options{}))
	if len(ex) != 6 {
		t.Fatalf("got %d examples, want 6", len(ex))
	}
	want := []directive.Kind{directive.KindTabs, directive.KindVerdict, directive.KindSteps,
		directive.KindStats, directive.KindDetails, directive.KindSources}
	for i, e := range ex {
		d, ok := directive.Parse(e)
		if !ok {
			t.Errorf("example %d does not parse:\n%s", i, e)
			continue
		}
		if d.Kind != want[i] {
			t.Errorf("example %d: kind %v, want %v", i, d.Kind, want[i])
		}
	}
}

func TestInlineForms(t *testing.T) {
	cases := map[string]directive.Kind{
		"> [!WARNING]\n> careful":            directive.KindCallout,
		"```tree\n├── a\n└── b\n```":         directive.KindTree,
		"```ts title=\"src/app.ts\"\nx\n```": directive.KindCodeTitle,
		"> quote\n> — Author":                directive.KindAttribution,
	}
	for in, k := range cases {
		d, ok := directive.Parse(in)
		if !ok || d.Kind != k {
			t.Errorf("%q: got %v ok=%v, want %v", in, d.Kind, ok, k)
		}
	}
	for _, kind := range calloutKinds {
		if !strings.Contains(Format(Options{}), "[!"+strings.ToUpper(kind)+"]") {
			t.Errorf("callout %s missing", kind)
		}
	}
}

func TestOptions(t *testing.T) {
	plain := Format(Options{})
	if strings.Contains(plain, "mermaid") || strings.Contains(plain, "$…$") {
		t.Error("default prompt mentions mermaid or math")
	}
	full := Format(Options{Math: true, Diagrams: true, Header: "## Formatting"})
	for _, s := range []string{"```mermaid", "$…$", "## Formatting\n", `forms in "Formatting"`} {
		if !strings.Contains(full, s) {
			t.Errorf("full prompt lacks %q", s)
		}
	}
}
