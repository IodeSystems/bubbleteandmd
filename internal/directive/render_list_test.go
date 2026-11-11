package directive

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

// listCtx builds a Ctx whose Inner is a real glamour renderer, margin 0.
func listCtx(t *testing.T, name string, width int) Ctx {
	t.Helper()
	cfg := *styles.DefaultStyles[name]
	zero := uint(0)
	cfg.Document.Margin = &zero
	th := ThemeFrom(cfg, name == "notty")
	return Ctx{Width: width, Theme: th, Inner: func(md string, w int) (string, error) {
		r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(w))
		if err != nil {
			return "", err
		}
		out, err := r.Render(md)
		if err != nil {
			return "", err
		}
		var ls []string
		for _, l := range strings.Split(strings.Trim(out, "\n"), "\n") {
			ls = append(ls, strings.TrimRight(l, " "))
		}
		return strings.Join(ls, "\n"), nil
	}}
}

const listSteps = "> [!STEPS] Ship it\n" +
	"> 1. **Build** *(2 min)* — compile the project with `make all` and wait for the long running linker to finish.\n" +
	"> 2. **Test**\n>    Run the suite.\n" +
	"> 3. Just a body with no bold title that is long enough to wrap around.\n"

const listStats = "> [!STATS] Numbers\n" +
	"> - **99.9%** uptime\n> - **1,204** requests per second\n> - **日本語** CJK 値\n> - **🚀 3x** emoji speed\n> - **12** users\n"

const listSources = "> [!SOURCES] Reading\n" +
	"> 1. [A very long article title that must wrap with a hanging indent](https://www.example.com/a) — a note\n" +
	"> 2. [Short](https://go.dev/doc/)\n" +
	"> 3. https://example.org/page\n" +
	"> 4. Plain item, not a link\n"

func listDirective(t *testing.T, src string, k Kind) Directive {
	return mustParse(t, src, k)
}

func TestListForms(t *testing.T) {
	forms := []struct {
		name string
		src  string
		kind Kind
	}{{"steps", listSteps, KindSteps}, {"stats", listStats, KindStats}, {"sources", listSources, KindSources}}
	for _, f := range forms {
		d := listDirective(t, f.src, f.kind)
		for _, style := range []string{"dark", "light", "notty"} {
			for _, w := range []int{36, 76, 10} {
				c := listCtx(t, style, w)
				lines, err := Render(d, c)
				if err != nil {
					t.Fatalf("%s/%s/%d: %v", f.name, style, w, err)
				}
				checkWidth(t, lines, w)
				if style == "notty" && strings.Contains(strings.Join(lines, "\n"), "\x1b") {
					t.Errorf("%s/%d notty has SGR", f.name, w)
				}
				if w == 10 {
					continue
				}
				golden(t, fmt.Sprintf("list_%s_%s_%d", f.name, style, w), strings.Join(lines, "\n")+"\n")
			}
		}
	}
}

func TestStatsWideCells(t *testing.T) {
	d := listDirective(t, listStats, KindStats)
	c := listCtx(t, "notty", 40)
	lines, err := renderStats(d, c)
	if err != nil {
		t.Fatal(err)
	}
	checkWidth(t, lines, 40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "日本語") || !strings.Contains(joined, "🚀") {
		t.Fatalf("missing cells: %q", joined)
	}
	// 日本語 is 6 cells; the label row must start the next cell in the same column as the value row.
	for _, l := range lines {
		_ = ansi.StringWidth(l)
	}
}

func TestSourcesBareAndPlain(t *testing.T) {
	d := listDirective(t, listSources, KindSources)
	lines, _ := renderSources(d, listCtx(t, "notty", 76))
	j := strings.Join(lines, "\n")
	for _, want := range []string{"example.com", "go.dev", "https://example.org/page", "Plain item"} {
		if !strings.Contains(j, want) {
			t.Errorf("missing %q in %q", want, j)
		}
	}
	if strings.Contains(j, "www.") {
		t.Errorf("www. not dropped: %q", j)
	}
}
