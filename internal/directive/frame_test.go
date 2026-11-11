package directive

import (
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func glamourPiece(t *testing.T, r *glamour.TermRenderer, md string, first bool) string {
	t.Helper()
	e, _ := r.Render("")
	h := len(e) / 2
	P, S := e[:h], e[h:]
	in := md
	if !first {
		in = "<!-- -->\n\n" + md
	}
	out, err := r.Render(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, P) || !strings.HasSuffix(out, S) {
		t.Fatalf("no affixes in %q", out)
	}
	return out[len(P) : len(out)-len(S)]
}

func TestFrameMatchesGlamour(t *testing.T) {
	for _, name := range []string{"dark", "light", "notty"} {
		cfg := *styles.DefaultStyles[name]
		fs := StyleOf(cfg, name == "notty")
		if name == "notty" != PlainStyle(cfg) {
			t.Errorf("%s: PlainStyle = %v", name, PlainStyle(cfg))
		}
		for _, w := range []int{40, 80} {
			r, _ := glamour.NewTermRenderer(glamour.WithStandardStyle(name), glamour.WithWordWrap(w))
			cw := w - 2*len(fs.Margin)
			for _, md := range []string{
				"Alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau.\n",
				"short\n",
				"One *em* **strong** `code` [l](http://x.y) text that wraps around for sure at forty columns and again at eighty columns.\n",
			} {
				for _, first := range []bool{true, false} {
					want := glamourPiece(t, r, md, first)
					body := want
					if !first {
						body = body[strings.Index(body, "\n")+1:]
					}
					var lines []string
					for _, l := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
						l = strings.TrimPrefix(l, fs.Margin)
						for strings.HasSuffix(l, fs.Cell) {
							l = strings.TrimSuffix(l, fs.Cell)
						}
						lines = append(lines, l)
					}
					if got := Frame(lines, cw, fs, first); got != want {
						t.Errorf("%s/%d first=%v\n got %q\nwant %q", name, w, first, got, want)
					}
				}
			}
		}
	}
}

func TestFrameStyledLinesKeepWidth(t *testing.T) {
	for _, name := range []string{"dark", "light", "notty"} {
		cfg := *styles.DefaultStyles[name]
		th := ThemeFrom(cfg, name == "notty")
		line := th.Callout["warning"].Render("WARNING") + " " + th.Muted.Render("dim")
		if lipgloss.Width(line) != xansi.StringWidth(line) {
			t.Fatal("width mismatch")
		}
		got := Frame([]string{line}, 30, th.Frame, true)
		if w := xansi.StringWidth(strings.TrimSuffix(got, "\n")); w != len(th.Frame.Margin)+30 {
			t.Errorf("%s: framed width %d", name, w)
		}
		if name == "notty" && strings.Contains(got, "\x1b") {
			t.Errorf("notty has SGR: %q", got)
		}
	}
}

func TestRenderStubs(t *testing.T) {
	var _ ansi.StyleConfig
	if _, err := Render(Directive{Kind: KindTree}, Ctx{Width: 40}); err == nil {
		t.Fatal("want error")
	}
	if _, err := Render(Directive{}, Ctx{Width: 40}); err == nil {
		t.Fatal("want error")
	}
}
