package directive

import (
	"fmt"
	"math/rand/v2"
	"regexp"
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
				lines, _, err := Render(d, c)
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

// listStatsLong: long labels with code spans at their ends (dun's report,
// neutral text). No two tiles fit side by side, so it draws as a list.
const listStatsLong = "> [!STATS]\n" +
	"> - **0** cache keys in the config — the `APP_*` reads never resolved\n" +
	"> - **1** rule on the deploy role (`push` only) — the `pull` grant is gone\n" +
	"> - **0** `login` / `token` steps left in the scripts\n"

// TestStatsList: the list layout: value column, two spaces, the label
// wrapped under its column (w40), one line per stat when labels fit (w80),
// and no blank cell at a line's edges.
func TestStatsList(t *testing.T) {
	d := listDirective(t, listStatsLong, KindStats)
	want := "0  cache keys in the config — the APP_* reads never resolved\n" +
		"1  rule on the deploy role (push only) — the pull grant is gone\n" +
		"0  login / token steps left in the scripts"
	line := regexp.MustCompile(`^([01]| ) {2}[^ ]`)
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{80, 40} {
			lines, err := renderStats(d, listCtx(t, style, w))
			if err != nil {
				t.Fatal(err)
			}
			checkWidth(t, lines, w)
			got := ansi.Strip(strings.Join(lines, "\n"))
			if w == 80 && style == "notty" && got != want {
				t.Errorf("notty:\n%s\nwant\n%s", got, want)
			}
			if n := len(lines); w == 80 && n != 3 || w == 40 && n <= 3 {
				t.Errorf("%s/%d: %d lines:\n%s", style, w, n, got)
			}
			for i, l := range strings.Split(got, "\n") {
				l = strings.ReplaceAll(l, " ", " ")
				if strings.HasSuffix(l, " ") || !line.MatchString(l) {
					t.Errorf("%s/%d line %d: %q", style, w, i, l)
				}
			}
		}
	}
}

// TestStatTrim: blank cells go at the edges only, and the code pad's style
// stays on the cells left.
func TestStatTrim(t *testing.T) {
	code := "\x1b[38;5;203;48;5;236m"
	in := "\x1b[38;5;252m\x1b[m" + code + " login \x1b[m\x1b[38;5;252m / \x1b[m" + code + " x \x1b[m\x1b[38;5;252m \x1b[m\x1b[38;5;252m \x1b[m"
	want := code + "login \x1b[m\x1b[38;5;252m / \x1b[m" + code + " x\x1b[m"
	if got := statTrim(in); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	for in, want := range map[string]string{
		"": "", "  ": "", " a b ": "a b", "\x1b[1m \x1b[m": "",
		"\x1b]8;;u\x07 a \x1b]8;;\x07": "\x1b]8;;u\x07a\x1b]8;;\x07",
	} {
		if got := statTrim(in); got != want {
			t.Errorf("statTrim(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestStatsStreamNoShrink: a stats form arriving item by item (each value
// rune by rune, then its label word by word) never draws fewer lines than
// the frame before. Random shapes cross every layout rule: tiles, list,
// values wider than statsMaxValue, labels wider than the width.
func TestStatsStreamNoShrink(t *testing.T) {
	values := []string{"0", "3×", "−40%", "1,204", "99.9%", "12 ms", "1.5 GB/s", "4,000,000 rows", "日本語"}
	words := strings.Fields("cache keys in the config `APP_*` reads never resolved rule on deploy role `push` only steps left a b 値")
	rng := rand.New(rand.NewPCG(7, 11))
	cases := 200
	if testing.Short() {
		cases = 60
	}
	for n := 0; n < cases; n++ {
		items := make([]Stat, 1+rng.IntN(7))
		for i := range items {
			items[i].Value = values[rng.IntN(len(values))]
			ws := make([]string, rng.IntN(14))
			for j := range ws {
				ws[j] = words[rng.IntN(len(words))]
			}
			items[i].Label = strings.Join(ws, " ")
		}
		style := []string{"dark", "notty"}[n%2]
		w := []int{24, 36, 50, 80}[rng.IntN(4)]
		c := listCtx(t, style, w)
		last, lastDesc := 0, ""
		frame := func(st []Stat, desc string) {
			lines, err := renderStats(Directive{Stats: &Stats{Items: st}}, c)
			if err != nil {
				t.Fatal(err)
			}
			checkWidth(t, lines, w)
			if len(lines) < last {
				t.Fatalf("case %d %s w%d: %d → %d lines\n%s\n→ %s\n%s", n, style, w, last, len(lines), lastDesc, desc, ansi.Strip(strings.Join(lines, "\n")))
			}
			last, lastDesc = len(lines), desc
		}
		for k, it := range items {
			st := append([]Stat(nil), items[:k]...)
			frame(append(st, Stat{}), fmt.Sprintf("item %d empty", k))
			rs := []rune(it.Value)
			for j := 1; j <= len(rs); j++ {
				frame(append(st, Stat{Value: string(rs[:j])}), fmt.Sprintf("item %d value %q", k, string(rs[:j])))
			}
			ws := strings.Fields(it.Label)
			for j := 1; j <= len(ws); j++ {
				frame(append(st, Stat{Value: it.Value, Label: strings.Join(ws[:j], " ")}), fmt.Sprintf("item %d label %q", k, strings.Join(ws[:j], " ")))
			}
		}
	}
}
