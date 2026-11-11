package directive

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
)

func codeCtx(t *testing.T, name string, w int) Ctx {
	t.Helper()
	cfg := *styles.DefaultStyles[name]
	th := ThemeFrom(cfg, name == "notty")
	inner := func(md string, width int) (string, error) {
		r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(width))
		if err != nil {
			return "", err
		}
		return r.Render(md)
	}
	return Ctx{Width: w, Theme: th, Inner: inner}
}

func firstDirective(t *testing.T, md string) Directive {
	t.Helper()
	d, ok := Parse(md)
	if !ok {
		t.Fatalf("no directive in %q", md)
	}
	return d
}

const codeTreeSrc = "```tree\nsrc/\n├── app/\n│   ├── main.go  # entry\n│   └── util.go\n├── docs/\n│   └── guide/\n│       └── intro.md\n└── README.md\n```\n"
const codeTitleSrc = "```go title=\"cmd/server/main.go\"\nfunc main() {\n\tfmt.Println(\"a fairly long line of code that needs wrapping at narrow width\")\n}\n```\n"

func TestTreeGolden(t *testing.T) {
	for _, name := range []string{"dark", "light", "notty"} {
		for _, w := range []int{36, 76} {
			c := codeCtx(t, name, w)
			lines, err := renderTree(firstDirective(t, codeTreeSrc), c)
			if err != nil {
				t.Fatal(err)
			}
			checkWidth(t, lines, w)
			if name == "notty" && strings.Contains(strings.Join(lines, ""), "\x1b") {
				t.Errorf("notty has SGR")
			}
			golden(t, fmt.Sprintf("tree_%s_%d", name, w), strings.Join(lines, "\n")+"\n")
		}
	}
}

func TestCodeTitleGolden(t *testing.T) {
	for _, name := range []string{"dark", "light", "notty"} {
		for _, w := range []int{36, 76} {
			c := codeCtx(t, name, w)
			lines, err := renderCodeTitle(firstDirective(t, codeTitleSrc), c)
			if err != nil {
				t.Fatal(err)
			}
			checkWidth(t, lines, w)
			if name == "notty" && strings.Contains(strings.Join(lines, ""), "\x1b") {
				t.Errorf("notty has SGR")
			}
			if !strings.Contains(xansi.Strip(lines[0]), "▤ cmd/server/main.go") || strings.TrimSpace(xansi.Strip(lines[1])) == "" {
				t.Errorf("title not directly above code: %q", lines[:2])
			}
			golden(t, fmt.Sprintf("codetitle_%s_%d", name, w), strings.Join(lines, "\n")+"\n")
		}
	}
}

func TestTreeNarrowNoPanic(t *testing.T) {
	for _, name := range []string{"dark", "notty"} {
		c := codeCtx(t, name, 10)
		for _, src := range []string{codeTreeSrc, codeTitleSrc} {
			d := firstDirective(t, src)
			lines, err := Render(d, c)
			if err != nil {
				t.Fatal(err)
			}
			checkWidth(t, lines, 10)
		}
	}
}

func TestTreeDeepCommentsUnicode(t *testing.T) {
	src := "```tree\nroot/\n└── a/\n    └── b/\n        └── c/\n            ├── 日本語ファイル.txt  # 説明 text\n            └── café.go\n```\n"
	for _, w := range []int{36, 76} {
		c := codeCtx(t, "dark", w)
		lines, err := renderTree(firstDirective(t, src), c)
		if err != nil {
			t.Fatal(err)
		}
		checkWidth(t, lines, w)
		golden(t, fmt.Sprintf("tree_deep_%d", w), strings.Join(lines, "\n")+"\n")
	}
}

func TestTreePlainRowAndFence(t *testing.T) {
	c := codeCtx(t, "notty", 40)
	lines, err := renderTree(Directive{Kind: KindTree, Tree: &Tree{Rows: ParseTree("??? weird\nfile.txt")}}, c)
	if err != nil || len(lines) != 2 {
		t.Fatalf("%v %q", err, lines)
	}
	if f := codeTitleFence("a\n````\nb\n"); f != "`````" {
		t.Errorf("fence %q", f)
	}
}

// TestCodeTitleCutsWidth: a code line glamour does not wrap (one long token
// at a narrow width) is cut to the width, as the other forms' lines are.
func TestCodeTitleCutsWidth(t *testing.T) {
	src := "```go title=\"a.go\"\nx := \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"\n```\n"
	for _, name := range []string{"dark", "notty"} {
		for w := 1; w <= 8; w++ {
			lines, err := renderCodeTitle(firstDirective(t, src), codeCtx(t, name, w))
			if err != nil {
				t.Fatal(err)
			}
			checkWidth(t, lines, w)
		}
	}
}
