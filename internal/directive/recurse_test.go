package directive

import (
	"strings"
	"testing"
)

func TestSplitQuotes(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"pros cons", "> [!PROS]\n> - a\n> [!CONS]\n> - b\n", "> [!PROS]\n> - a\n\n> [!CONS]\n> - b\n"},
		{"three", "> [!NOTE]\n> x\n> [!TIP] T\n> y\n> [!WARNING]\n> z", "> [!NOTE]\n> x\n\n> [!TIP] T\n> y\n\n> [!WARNING]\n> z"},
		{"tabs content", "> [!TABS]\n> #### A\n> [!PROS]\n> - a\n", ""},
		{"plain quote", "> text\n> [!PROS]\n> - a\n", ""},
		{"mid-line", "> [!PROS]\n> - a\n> see [!CONS]\n", ""},
		{"unknown kind", "> [!PROS]\n> - a\n> [!FOO]\n", ""},
		{"no space after", "> [!PROS]\n> - a\n> [!CONS]x\n", ""},
		{"fence in quote", "> [!NOTE]\n> ```\n> [!TIP]\n> ```\n", ""},
		{"fence at top", "```\n> [!PROS]\n> - a\n> [!CONS]\n```\n", ""},
		{"nested level", "> [!NOTE]\n> > [!TIP]\n> > x\n", ""},
		{"after tabs marker", "> [!PROS]\n> - a\n> [!TABS]\n> #### X\n> [!CONS]\n", "> [!PROS]\n> - a\n\n> [!TABS]\n> #### X\n> [!CONS]\n"},
		{"top-level line", "[!PROS]\n- a\n[!CONS]\n- b\n", ""},
		{"html block", "<details>\n> [!DETAILS]\n> [!DETAILS]\n", ""},
		{"lazy line stops", "> [!PROS]\n> - a\nlazy\n> [!CONS]\n> - b\n", ""},
		{"text after", "> [!PROS]\n> - a\n> [!CONS]\n> - b\n\npara\n", "> [!PROS]\n> - a\n\n> [!CONS]\n> - b\n\npara\n"},
	} {
		want := c.want
		if want == "" {
			want = c.in
		}
		got, sh := SplitQuotes(c.in)
		if got != want {
			t.Errorf("%s: got %q want %q", c.name, got, want)
		}
		if (got == c.in) != (sh == nil) {
			t.Errorf("%s: shifts %v", c.name, sh)
		}
	}
}

func TestBodyForms(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"pros cons", "[!PROS]\n- a\n- b\n[!CONS]\n- c", "> [!PROS]\n> - a\n> - b\n\n> [!CONS]\n> - c"},
		{"text above", "Intro.\n[!NOTE] T\nbody\n\nafter\n", "Intro.\n\n> [!NOTE] T\n> body\n\nafter\n"},
		{"blank above", "Intro.\n\n[!NOTE]\nbody\n", "Intro.\n\n> [!NOTE]\n> body\n"},
		{"fence in run", "[!NOTE]\n```\n[!TIP]\n\nx\n```\nend\n", "> [!NOTE]\n> ```\n> [!TIP]\n>\n> x\n> ```\n> end\n"},
		{"fence", "```\n[!PROS]\n- a\n```\n", ""},
		{"mid-line", "see [!PROS]\n- a\n", ""},
		{"indented", "  [!PROS]\n- a\n", ""},
		{"quoted", "> [!PROS]\n> - a\n", ""},
		{"lowercase", "[!pros]\n- a\n", "> [!pros]\n> - a\n"},
	} {
		want := c.want
		if want == "" {
			want = c.in
		}
		got, sh := BodyForms(c.in)
		if got != want {
			t.Errorf("%s: got %q want %q", c.name, got, want)
		}
		if (got == c.in) != (sh == nil) {
			t.Errorf("%s: shifts %v", c.name, sh)
		}
	}
}

// TestMarkerLineOffsets: each form a marker line opens maps back to the
// marker line's offset in the text as written.
func TestMarkerLineOffsets(t *testing.T) {
	body := "Intro.\n[!PROS]\n- a\n[!CONS]\n- b\n\n[!NOTE]\nx\n"
	out, sh := BodyForms(body)
	for _, mk := range []string{"[!PROS]", "[!CONS]", "[!NOTE]"} {
		at := strings.Index(out, "> "+mk)
		if got, want := RawOffset(sh, at), strings.Index(body, mk); got != want {
			t.Errorf("%s: raw %d, want %d", mk, got, want)
		}
	}
	block := "> [!PROS]\n> - a\n> [!CONS]\n> - b\n"
	src, _, s := Prepare(block)
	at := strings.Index(src, "> [!CONS]")
	if got, want := s.Raw(at), strings.Index(block, "> [!CONS]"); got != want {
		t.Errorf("quote split: raw %d, want %d", got, want)
	}
}

// TestMarkerLinesParse: the three inputs read as a pros and cons pair.
func TestMarkerLinesParse(t *testing.T) {
	top, _, _ := Prepare("> [!PROS]\n> - a\n> [!CONS]\n> - b\n")
	d, ok := Parse(top)
	if !ok || d.Verdict == nil || len(d.Verdict.Pros) != 1 || len(d.Verdict.Cons) != 1 || d.End != len(top) {
		t.Fatalf("top-level quote: %+v %v", d, ok)
	}
	tabs, ok := Parse("> [!TABS]\n> #### A\n> [!PROS]\n> - a\n> [!CONS]\n> - b\n>\n> #### B\n> > [!PROS]\n> > - c\n> > [!CONS]\n> > - d\n")
	if !ok || tabs.Tabs == nil || len(tabs.Tabs.Panels) != 2 {
		t.Fatalf("tabs: %+v %v", tabs, ok)
	}
	for i, p := range tabs.Tabs.Panels {
		body, _ := BodyForms(p.Body)
		src, _, _ := Prepare(body)
		d, ok := Parse(src)
		if !ok || d.Verdict == nil || len(d.Verdict.Pros) != 1 || len(d.Verdict.Cons) != 1 {
			t.Errorf("panel %d %q: %+v %v", i, src, d, ok)
		}
	}
}
