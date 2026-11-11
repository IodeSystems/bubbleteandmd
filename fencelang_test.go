package bubbleteandmd

import (
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

func TestPlainUnknownLangs(t *testing.T) {
	n := 0
	c := newLangCache()
	inner := c.lookup
	c.lookup = func(name string) bool { n++; return inner(name) }
	cases := []struct{ in, want string }{
		{"```\nx\n```\n", "```text\nx\n```\n"},
		{"```go\nx\n```\n", "```go\nx\n```\n"},
		{"```nosuchlang\nx\n```\n", "```text\nx\n```\n"},
		{"```mermaid title=\"t\"\nx\n```\n", "```text title=\"t\"\nx\n```\n"},
		{"```go title=\"t\"\nx\n```\n", "```go title=\"t\"\nx\n```\n"},
		{"~~~\nx\n~~~\n", "~~~text\nx\n~~~\n"},
		{"  ```\r\nx\r\n  ```\r\n", "  ```text\r\nx\r\n  ```\r\n"},
		// inside another fence, and indented code: untouched
		{"````md\n```\nx\n```\n````\n", "````md\n```\nx\n```\n````\n"},
		{"    ```\n    x\n    ```\n", "    ```\n    x\n    ```\n"},
		{"```\nunclosed", "```text\nunclosed"},
	}
	for _, tc := range cases {
		if got := c.plainUnknownLangs(tc.in); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
	// lookups: "" never asked; go, md, nosuchlang, mermaid asked once each.
	if n != 4 {
		t.Errorf("lookups = %d, want 4 (once per name)", n)
	}
}

// Unknown and empty languages render with the same lines and text as the
// raw glamour render; only colours may differ.
func TestPlainUnknownLangsRender(t *testing.T) {
	for _, style := range []string{"dark", "light"} {
		r := newTestRenderer(t, style, 60)
		g, err := glamour.NewTermRenderer(glamour.WithStyles(*styles.DefaultStyles[style]), glamour.WithWordWrap(60))
		if err != nil {
			t.Fatal(err)
		}
		for _, md := range []string{
			"a\n\n```\nfunc main() { return 1 }\n```\n\nb\n",
			"```nosuchlang\n#!/bin/sh\necho hi\n```\n",
			"```math\nx^2\n```\n",
			"```mermaid\ngraph TD\n```\n",
		} {
			got, err := r.Render(md)
			if err != nil {
				t.Fatal(err)
			}
			want, err := g.Render(md)
			if err != nil {
				t.Fatal(err)
			}
			a, b := strings.Split(ansi.Strip(got), "\n"), strings.Split(ansi.Strip(want), "\n")
			if len(a) != len(b) {
				t.Fatalf("%s %q: %d lines, want %d", style, md, len(a), len(b))
			}
			for i := range a {
				if a[i] != b[i] {
					t.Errorf("%s %q line %d: %q want %q", style, md, i, a[i], b[i])
				}
			}
		}
	}
}

func BenchmarkUnlabeledFences(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		sb.WriteString("text\n\n```\nfunc main() { return 1 }\n```\n\n")
	}
	md := sb.String()
	r, err := NewRenderer(*styles.DefaultStyles["dark"], 80)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("rewrite", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := r.Render(md); err != nil {
				b.Fatal(err)
			}
		}
	})
	g, _ := glamour.NewTermRenderer(glamour.WithStyles(*styles.DefaultStyles["dark"]), glamour.WithWordWrap(80))
	b.Run("raw-glamour", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := g.Render(md); err != nil {
				b.Fatal(err)
			}
		}
	})
}
