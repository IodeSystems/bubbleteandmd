package spike_test

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
)

var frameKinds = []struct{ name, md string }{
	{"para", "Alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau.\n"},
	{"quote", "> Alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho.\n"},
	{"code", "```go\nx := 1\n```\n"},
}

func frameRender(t *testing.T, r *glamour.TermRenderer, md string, first bool) string {
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
		t.Fatalf("no affixes P=%q S=%q out=%q", P, S, out)
	}
	return out[len(P) : len(out)-len(S)]
}

var emptySGR = regexp.MustCompile("\x1b\\[[0-9;]*m\x1b\\[m")

// spec derived from glamour: margin + content + pad cells to lineW columns.
type frameSpec struct {
	margin string // left margin bytes (plain spaces)
	cell   string // one pad cell (styled space)
	lineW  int    // margin + content columns
}

func (f frameSpec) contentW() int { return f.lineW - len(f.margin) }

func (f frameSpec) blank() string { return f.margin + strings.Repeat(f.cell, f.contentW()) + "\n" }

// frame wraps content lines (each <= contentW columns, no margin) in the paragraph frame.
func (f frameSpec) frame(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(f.margin + l)
		if w := xansi.StringWidth(l); w < f.contentW() {
			b.WriteString(strings.Repeat(f.cell, f.contentW()-w))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// unframe strips margin and trailing pad cells from each line.
func (f frameSpec) unframe(t *testing.T, piece string) []string {
	if !strings.HasSuffix(piece, "\n") {
		t.Fatalf("no trailing newline %q", piece)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSuffix(piece, "\n"), "\n") {
		if !strings.HasPrefix(l, f.margin) {
			t.Fatalf("no margin %q", l)
		}
		l = strings.TrimPrefix(l, f.margin)
		for strings.HasSuffix(l, f.cell) {
			l = strings.TrimSuffix(l, f.cell)
		}
		out = append(out, l)
	}
	return out
}

func deriveSpec(t *testing.T, r *glamour.TermRenderer, st string) frameSpec {
	// margin = Document.Margin plain spaces; cell = one SGR-wrapped space (or " " in notty).
	m := int(*styles.DefaultStyles[st].Document.Margin)
	p := frameRender(t, r, frameKinds[0].md, false)
	blank := p[:strings.Index(p, "\n")]
	rest := blank[m:]
	cell := " "
	if strings.HasPrefix(rest, "\x1b") {
		cell = rest[:strings.Index(rest, "m\x1b[m")+len("m\x1b[m")+0]
		// cell = SGR + " " + reset
		cell = rest[:strings.Index(rest[1:], "\x1b")+1+len("\x1b[m")]
	}
	return frameSpec{margin: blank[:m], cell: cell, lineW: xansi.StringWidth(blank)}
}

func TestFrameTable(t *testing.T) {
	for _, st := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 80} {
			r, _ := glamour.NewTermRenderer(glamour.WithStandardStyle(st), glamour.WithWordWrap(w))
			sp := deriveSpec(t, r, st)
			t.Logf("%s/%d margin=%q cell=%q lineW=%d contentW=%d", st, w, sp.margin, sp.cell, sp.lineW, sp.contentW())
			for _, k := range frameKinds {
				for _, first := range []bool{true, false} {
					p := frameRender(t, r, k.md, first)
					lines := strings.Split(strings.TrimSuffix(p, "\n"), "\n")
					lead := 0
					for _, l := range lines {
						if l+"\n" == sp.blank() {
							lead++
						} else {
							break
						}
					}
					widths := map[int]bool{}
					for _, l := range lines {
						widths[xansi.StringWidth(l)] = true
					}
					t.Logf("  %-5s first=%-5v nlines=%d leadBlank=%d widths=%v endsNL=%v endsBlank=%v emptySGR=%d startsMargin=%v",
						k.name, first, len(lines), lead, widths, strings.HasSuffix(p, "\n"), lines[len(lines)-1]+"\n" == sp.blank(),
						len(emptySGR.FindAllString(p, -1)), strings.HasPrefix(p, sp.margin))
				}
			}
		}
	}
}

func TestFrameRoundTrip(t *testing.T) {
	for _, st := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 80} {
			r, _ := glamour.NewTermRenderer(glamour.WithStandardStyle(st), glamour.WithWordWrap(w))
			sp := deriveSpec(t, r, st)
			mds := []string{
				frameKinds[0].md,
				"One *em* **strong** `code` [l](http://x.y) text that wraps around for sure at forty columns and again at eighty columns, yes indeed.\n",
				"short\n",
				"a\n\nb\n", // two paras: piece has an inner blank line
			}
			for _, md := range mds {
				for _, first := range []bool{true, false} {
					p := frameRender(t, r, md, first)
					lines := sp.unframe(t, p)
					if !first { // drop spacing blank (unframes to "")
						if lines[0] != "" {
							t.Fatalf("%s/%d: first line not blank %q", st, w, lines[0])
						}
						lines = lines[1:]
					}
					got := sp.frame(lines)
					if !first {
						got = sp.blank() + got
					}
					if got != p {
						t.Errorf("%s/%d first=%v md=%q\n got %q\nwant %q", st, w, first, md, got, p)
					}
				}
			}
			t.Logf("%s/%d roundtrip ok (if no error above)", st, w)
		}
	}
}

func up(n uint) *uint { return &n }

func TestFrameInner(t *testing.T) {
	for _, st := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 80} {
			outer, _ := glamour.NewTermRenderer(glamour.WithStandardStyle(st), glamour.WithWordWrap(w))
			sp := deriveSpec(t, outer, st)
			cfg := styles.DefaultStyles[st]
			c := *cfg
			c.Document.Margin = up(0)
			c.Document.Indent = up(0)
			for _, iw := range []int{sp.contentW(), w - 2, w} {
				inner, _ := glamour.NewTermRenderer(glamour.WithStyles(c), glamour.WithWordWrap(iw))
				e, _ := inner.Render("")
				h := len(e) / 2
				P, S := e[:h], e[h:]
				for _, md := range []string{frameKinds[0].md, "short\n"} {
					out, _ := inner.Render(md)
					body := strings.TrimSuffix(strings.TrimPrefix(out, P), S)
					lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
					want := frameRender(t, outer, md, true)
					got := sp.frame(trimPad(sp, lines))
					t.Logf("%s/%d innerWrap=%d empty=%q equal=%v innerLineW=%d", st, w, iw, e, got == want, xansi.StringWidth(lines[0]))
					if got != want {
						t.Logf("   got  %q\n   want %q", got, want)
					}
				}
			}
		}
	}
}

func trimPad(sp frameSpec, lines []string) []string {
	var out []string
	for _, l := range lines {
		for strings.HasSuffix(l, sp.cell) {
			l = strings.TrimSuffix(l, sp.cell)
		}
		out = append(out, l)
	}
	return out
}

func TestFrameCodeTail(t *testing.T) {
	r, _ := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(40))
	p := frameRender(t, r, frameKinds[2].md, false)
	t.Logf("code dark/40 %q", p[len(p)-120:])
	p = frameRender(t, r, frameKinds[1].md, false)
	t.Logf("quote dark/40 head %q", p[:260])
}
