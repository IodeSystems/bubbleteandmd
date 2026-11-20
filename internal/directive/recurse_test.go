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

// TestQuoteHoldsForm: a plain quote is KindQuote when a quote in it, at any
// depth through quotes, opens with a known marker (flowdown shapes every
// quote); else it stays the quote it was written as.
func TestQuoteHoldsForm(t *testing.T) {
	for _, c := range []struct {
		name, in string
		want     Kind
	}{
		{"pros cons after", "> #### Option B\n> > [!PROS]\n> - a\n>\n> > [!CONS]\n> - b\n", KindQuote},
		{"two levels", "> text\n>\n> > more\n> >\n> > > [!NOTE]\n> > > x\n", KindQuote},
		{"marker only, no list", "> text\n>\n> > [!PROS]\n", KindQuote},
		{"plain", "> text\n>\n> > more\n", KindNone},
		{"marker line in plain quote", "> text\n> [!PROS]\n> - a\n", KindNone},
		{"unknown marker", "> text\n>\n> > [!FOO]\n> > x\n", KindNone},
		{"quote in a list item", "> - item\n>\n>   > [!NOTE]\n>   > x\n", KindNone},
		{"form quote", "> [!NOTE]\n> > [!TIP]\n> > x\n", KindCallout},
	} {
		if d, _ := Parse(c.in); d.Kind != c.want {
			t.Errorf("%s: kind %v, want %v", c.name, d.Kind, c.want)
		}
	}
	d, _ := Parse("> #### Option B\n> > [!PROS]\n> - a\n")
	if d.Quote == nil || d.Quote.Body != "#### Option B\n> [!PROS]\n- a" {
		t.Errorf("body %+v", d.Quote)
	}
}

// TestJoinTabs: quotes after quoted tabs, each opening with a heading of
// the first tab heading's depth, are more tabs (split tabs joined, Carl
// 2026-11-20); any other quote, or a block between, ends them.
func TestJoinTabs(t *testing.T) {
	src := "> [!TABS]\n> #### A\n> one\n\n> #### B\n> two\n>\n> #### C\n> three\n\n> ### D\n> four\n"
	d, ok := Parse(src)
	if !ok || d.Tabs == nil {
		t.Fatalf("%+v", d)
	}
	var labels []string
	for _, p := range d.Tabs.Panels {
		labels = append(labels, p.Label+"="+p.Body)
	}
	if got := strings.Join(labels, " "); got != "A=one B=two C=three" {
		t.Errorf("panels %s", got)
	}
	if rest := src[d.End:]; !strings.HasPrefix(strings.TrimLeft(rest, "\n"), "> ### D") {
		t.Errorf("end %q", rest)
	}
	for _, in := range []string{
		"> [!TABS]\n#### A\none\n\n> #### B\n> two\n",
		"> [!TABS]\n> #### A\n> one\n\ntext\n\n> #### B\n> two\n",
		"> [!TABS]\n> #### A\n> one\n\n> two\n",
	} {
		if d, _ := Parse(in); d.Tabs == nil || len(d.Tabs.Panels) != 1 {
			t.Errorf("%q: %+v", in, d.Tabs)
		}
	}
}
