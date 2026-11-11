package directive

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
)

var tabsPad = regexp.MustCompile("(\x1b\\[[0-9;]*m)* (\x1b\\[[0-9;]*m)*$")

var tabsSGR = regexp.MustCompile("\x1b\\[[0-9;]*m")

// tabsCtx builds a Ctx whose Inner is a real glamour renderer, margin 0.
func tabsCtx(t *testing.T, style string, width int) Ctx {
	t.Helper()
	cfg := *styles.DefaultStyles[style]
	zero := uint(0)
	cfg.Document.Margin = &zero
	th := ThemeFrom(cfg, style == "notty")
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
			for {
				n := tabsPad.ReplaceAllString(l, "")
				if n == l {
					break
				}
				l = n
			}
			if strings.Contains(l, "\x1b[") && !strings.HasSuffix(l, "\x1b[m") {
				l += "\x1b[m"
			}
			ls = append(ls, l)
		}
		return strings.Join(ls, "\n"), nil
	}}
}

func tabsDir() Directive {
	return Directive{Kind: KindTabs, Tabs: &Tabs{Panels: []Panel{
		{Label: "macOS", Body: "Install with **brew** and then run the setup command to finish.\n\n```sh\nbrew install llx\n```\n"},
		{Label: "Linux", Body: "Use the package manager of your distribution, for example `apt`."},
		{Label: "Windows", Body: "- scoop\n- winget\n"},
	}}}
}

func verdictDir(consFirst bool) Directive {
	return Directive{Kind: KindVerdict, Verdict: &Verdict{ConsFirst: consFirst,
		Pros: []Item{{Body: "Fast startup and a small memory footprint on every platform."}, {Body: "Simple."}},
		Cons: []Item{{Body: "Young ecosystem."}, {Body: "Few maintainers, so fixes can take a long while to land."}, {Body: "No plugin API yet."}},
	}}
}

func TestTabsGolden(t *testing.T) {
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{36, 64, 76} {
			c := tabsCtx(t, style, w)
			lines, _, err := renderTabs(tabsDir(), c)
			if err != nil {
				t.Fatal(err)
			}
			checkWidth(t, lines, w)
			if style == "notty" && tabsSGR.MatchString(strings.Join(lines, "\n")) {
				t.Errorf("notty has SGR")
			}
			golden(t, fmt.Sprintf("tabs_%s_%d", style, w), strings.Join(lines, "\n")+"\n")
		}
	}
}

func TestVerdictGolden(t *testing.T) {
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{36, 64, 76} {
			c := tabsCtx(t, style, w)
			for name, d := range map[string]Directive{"pc": verdictDir(false), "cp": verdictDir(true)} {
				lines, _, err := renderVerdict(d, c)
				if err != nil {
					t.Fatal(err)
				}
				checkWidth(t, lines, w)
				if style == "notty" && tabsSGR.MatchString(strings.Join(lines, "\n")) {
					t.Errorf("notty has SGR")
				}
				golden(t, fmt.Sprintf("verdict_%s_%s_%d", name, style, w), strings.Join(lines, "\n")+"\n")
			}
			only := Directive{Kind: KindVerdict, Verdict: &Verdict{ProsTitle: "Why", Pros: []Item{{Body: "One reason."}}}}
			lines, _, err := renderVerdict(only, c)
			if err != nil {
				t.Fatal(err)
			}
			checkWidth(t, lines, w)
			golden(t, fmt.Sprintf("verdict_pros_%s_%d", style, w), strings.Join(lines, "\n")+"\n")
		}
	}
}

func TestVerdictColumnsAlign(t *testing.T) {
	c := tabsCtx(t, "notty", 64)
	lines, _, err := renderVerdict(verdictDir(false), c)
	if err != nil {
		t.Fatal(err)
	}
	// the right column starts at cell 30 + 4 on every row that has one
	for _, l := range lines {
		if r := []rune(l); len(r) > 30 && strings.TrimSpace(string(r[30:34])) != "" {
			t.Errorf("gap not blank: %q", l)
		}
	}
	if !strings.Contains(lines[0], "+ Pros") || !strings.Contains(lines[0], "− Cons") {
		t.Errorf("headers: %q", lines[0])
	}
	if len(lines) < 5 {
		t.Errorf("unequal columns: %d rows", len(lines))
	}
}

func TestTabsVerdictNarrow(t *testing.T) {
	for _, style := range []string{"dark", "notty"} {
		for _, w := range []int{1, 2, 3, 5, 10} {
			c := tabsCtx(t, style, w)
			for _, d := range []Directive{tabsDir(), verdictDir(false), verdictDir(true)} {
				lines, _, err := Render(d, c)
				if err != nil {
					t.Fatal(err)
				}
				for i, l := range lines {
					if xansi.StringWidth(l) > w {
						t.Errorf("w=%d line %d too wide: %q", w, i, l)
					}
				}
			}
		}
	}
}
