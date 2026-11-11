// Ported from llx-markdown packages/llx-markdown/src/core/heal.test.ts and dollars.test.tsx
// (MIT, see NOTICE). Each TS test is one subtest; the TS names are kept.

package heal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

type pair struct{ in, want string }

// run checks every pair of a TS test.
func run(t *testing.T, name string, pairs ...pair) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		for _, p := range pairs {
			if got := Close(p.in); got != p.want {
				t.Errorf("Close(%q) = %q, want %q", p.in, got, p.want)
			}
		}
	})
}

// same checks inputs that must come back unchanged.
func same(t *testing.T, name string, texts ...string) {
	t.Helper()
	pairs := make([]pair, len(texts))
	for i, s := range texts {
		pairs[i] = pair{s, s}
	}
	run(t, name, pairs...)
}

func TestMath(t *testing.T) {
	run(t, "withholds a formula still being typed, from its opening dollar",
		pair{"Energy is $E = mc", "Energy is"},
		pair{"Then:\n$$\n\\frac{a}{", "Then:"},
		pair{"Done $a$ and $b", "Done $a$ and"})
	same(t, "keeps a closed formula, currency and shell variables as they are",
		"Energy is $E = mc^2$ here", "It costs $5 today", "Set $HOME/bin first",
		"Export $PATH before", "Escaped \\$x stays", "In code `echo $x` only")
	same(t, "a dollar that can never open or close a formula hides nothing",
		`for i in $(seq 1 12); do printf "line %s" $i; done`,
		`for i in $(seq 1 12); do printf "line %s" $i`,
		"It costs 5$ and", "Spaced $ x", "Then $a$5 more", "Energy is $E$")
	run(t, "a dollar at the very end may open a formula with the next token",
		pair{"Energy is $", "Energy is"})
	same(t, "a * or _ inside a closed formula is not an open emphasis",
		"Product $a*b$ and $x_i$ done")
}

func TestCalloutMarkers(t *testing.T) {
	run(t, "hides a callout marker still being typed, keeps a complete one",
		pair{"Intro\n\n> [!WARN", "Intro\n"},
		pair{"Intro\n\n> [!", "Intro\n"},
		pair{"> [", ""},
		pair{"Intro\n\n> [!WARNING]", "Intro\n\n> [!WARNING]"})
	same(t, "leaves an ordinary quote or a bracket in prose alone", "> quoted [text", "See [")
}

func TestVeryString(t *testing.T) {
	t.Run("comes back as the very string, not a copy of it", func(t *testing.T) {
		text := strings.Repeat("A paragraph of plain prose.\n\n", 50) + "And the last one, finished."
		got := Close(text)
		if got != text || unsafe.StringData(got) != unsafe.StringData(text) {
			t.Error("Close copied a text with nothing to complete")
		}
		if got, want := Close(text+" **bold"), text+" **bold**"; got != want {
			t.Errorf("got %q", got[len(got)-20:])
		}
	})
}

func TestOpenFenceBody(t *testing.T) {
	t.Run("is the body of the one unterminated fence, and null once every fence is closed", func(t *testing.T) {
		for _, c := range []struct {
			in, want string
			ok       bool
		}{
			{"Intro\n```mermaid\nflowchart LR\n  A --> B", "flowchart LR\n  A --> B", true},
			{"```mermaid\n", "", true},
			{"```mermaid\nA --> B\n```\nThen prose.", "", false},
			{"Plain prose", "", false},
		} {
			if got, ok := OpenFenceBody(c.in); got != c.want || ok != c.ok {
				t.Errorf("OpenFenceBody(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
			}
		}
	})
	t.Run("only the tail fence counts as growing: a closed diagram above it is drawn", func(t *testing.T) {
		text := "```mermaid\nA --> B\n```\n\nMore:\n```mermaid\nC --> D"
		growing, ok := OpenFenceBody(text)
		if !IsGrowingFence(growing, ok, "C --> D\n") {
			t.Error("tail fence not growing")
		}
		if IsGrowingFence(growing, ok, "A --> B") {
			t.Error("closed fence growing")
		}
		if IsGrowingFence("", false, "A --> B") {
			t.Error("no open fence is growing")
		}
	})
}

func TestClose(t *testing.T) {
	same(t, "plain complete text is returned verbatim",
		"Hello **world**, see `code` and [a link](https://x.dev).")
	same(t, "empty string is returned as-is", "")
	run(t, "closes an unterminated code fence",
		pair{"intro\n```ts\nconst a = 1", "intro\n```ts\nconst a = 1\n```"})
	run(t, "closes an unterminated tilde fence with the same marker",
		pair{"~~~~\ncode", "~~~~\ncode\n~~~~"})
	same(t, "leaves a terminated fence untouched", "```ts\nconst a = 1\n```\nafter")
	run(t, "skips inline fixes while inside a fence", pair{"```\na ** b", "```\na ** b\n```"})
	run(t, "a shorter fence-like line does not close a longer opener",
		pair{"````\n```\ncode", "````\n```\ncode\n````"})
	run(t, "closes a dangling bold span", pair{"this is **import", "this is **import**"})
	run(t, "strips a bare trailing bold opener", pair{"this is **", "this is "})
	run(t, "closes a dangling strikethrough span", pair{"was ~~remov", "was ~~remov~~"})
	run(t, "closes a dangling inline code span", pair{"call `fetchCata", "call `fetchCata`"})
	run(t, "strips a bare trailing backtick opener", pair{"call `", "call "})
	same(t, "ignores ** inside inline code when balancing", "the glob `**/*.ts` matches")
	run(t, "closes a half-typed link", pair{"see [docs](https://exa", "see [docs](https://exa)"})
	run(t, "hides a half-typed image",
		pair{"look ![diagram](https://exa", "look "}, pair{"look ![diag", "look "})
	run(t, "only the last paragraph is touched",
		pair{"a ** b\n\nnow **bol", "a ** b\n\nnow **bol**"})
	same(t, "balanced earlier paragraphs stay byte-identical", "**done** and `ok`\n\ntail ")
	same(t, "escaped backticks are not counted", "a literal \\` backtick")

	// single-char emphasis
	run(t, "closes a dangling single-asterisk italic", pair{"this is *important", "this is *important*"})
	run(t, "closes a dangling single-underscore italic", pair{"this is _important", "this is _important_"})
	run(t, "strips a bare trailing single emphasis opener", pair{"open intraword a*", "open intraword a"})
	same(t, "leaves a whitespace-flanked lone asterisk alone", "a stray *")
	same(t, "leaves complete single emphasis untouched", "an *italic* word", "an _italic_ word")
	same(t, "does not touch a multiplication asterisk", "compute 2 * 3 now")
	same(t, "does not touch a glob asterisk", "match *.ts files")
	same(t, "does not touch a trailing bullet marker", "list:\n\n* ")
	same(t, "does not touch intraword underscores",
		"the file_name var", "python __init__ dunder", "a snake_case_var here")
	same(t, "ignores single emphasis chars inside inline code", "see `a * b` and `x_y`")
	run(t, "does not double-count ** when balancing single asterisks",
		pair{"**bold** then *it", "**bold** then *it*"})

	// partial table rows
	run(t, "hides a half-typed trailing table row",
		pair{"| A | B |\n| --- | --- |\n| 1 |", "| A | B |\n| --- | --- |"})
	same(t, "does not hide pipe-prefixed prose or a table header before its separator",
		"| literal", "intro\n\n| quoted prose", "| A | B |",
		"| A | B |\n| --- | --- |\n| 1 | 2 |\n\n| prose")
	same(t, "keeps a completed table row (ends in newline)", "| A | B |\n| --- | --- |\n| 1 | 2 |\n")
}

func TestOpenFenceBodyIncremental(t *testing.T) {
	// The TS test proves its cache equals a fresh scan; this port has no cache, so only the
	// per-prefix answers are checked, against a reference that rescans line by line.
	t.Run("matches a fresh scan while text grows token by token", func(t *testing.T) {
		reply := "Intro\n\n```ts\nconst a = 1\n```\n\nMore\n~~~~\ncode\n~~~ x\n~~~~\nend\n```\nlast"
		for i := 0; i <= len(reply); i++ {
			text := reply[:i]
			got, ok := OpenFenceBody(text)
			if want, wantOK := refOpenBody(text); got != want || ok != wantOK {
				t.Fatalf("prefix %d: got %q,%v want %q,%v", i, got, ok, want, wantOK)
			}
		}
		if got, _ := OpenFenceBody(reply); got != "last" {
			t.Errorf("got %q", got)
		}
		if got, ok := OpenFenceBody("```"); got != "" || !ok {
			t.Error("bare opener")
		}
		if _, ok := OpenFenceBody("```\n```"); ok {
			t.Error("closed pair")
		}
	})
	// "stays linear on a long streamed reply" counted lines of the TS cache; not ported.
}

// refOpenBody is an independent, simple fence scan for the prefix test.
func refOpenBody(text string) (string, bool) {
	var marker string
	body, pos := 0, 0
	for _, line := range strings.SplitAfter(text, "\n") {
		trim := strings.TrimRight(strings.TrimLeft(line, " "), "\n")
		ch := ""
		if strings.HasPrefix(trim, "```") {
			ch = "`"
		} else if strings.HasPrefix(trim, "~~~") {
			ch = "~"
		}
		if ch != "" {
			run := trim[:len(trim)-len(strings.TrimLeft(trim, ch))]
			rest := trim[len(run):]
			switch {
			case marker == "" && !(ch == "`" && strings.Contains(rest, "`")):
				marker, body = run, min(pos+len(line), len(text))
			case marker != "" && ch == marker[:1] && len(run) >= len(marker) && strings.TrimSpace(rest) == "":
				marker = ""
			}
		}
		pos += len(line)
	}
	if marker == "" {
		return "", false
	}
	return text[body:], true
}

func TestIsLiteralDollar(t *testing.T) {
	t.Run("currency amounts and shell variables are dollar signs", func(t *testing.T) {
		for _, s := range []string{"$5 and", "$10,", "$1.5M today", "$HOME/bin", "$PATH,", "$PATH before"} {
			if !isLiteralDollar(s, 0) {
				t.Errorf("%q: want literal", s)
			}
		}
	})
	t.Run("formulas, display math and escaped dollars are not", func(t *testing.T) {
		for _, s := range []string{"$E = mc^2$", "$10^6$", "$2x$", "$AB$", "$AB = CD$", "$$", "$x_i$"} {
			if isLiteralDollar(s, 0) {
				t.Errorf("%q: want not literal", s)
			}
		}
		if isLiteralDollar("\\$5", 1) {
			t.Error(`\$5`)
		}
	})
	// Beyond the TS: backtracking and non-ASCII lookahead of the currency pattern.
	t.Run("currency lookahead", func(t *testing.T) {
		for s, want := range map[string]bool{"$5.5x": true, "$5—": true, "$5k": true, "$5kx": false, "$5x": false, "$": false} {
			if got := isLiteralDollar(s, 0); got != want {
				t.Errorf("%q = %v", s, got)
			}
		}
	})
}

func corpusDocs(t *testing.T) map[string]string {
	files, _ := filepath.Glob("../corpus/testdata/*.md")
	if len(files) == 0 {
		t.Skip("no corpus under ../corpus/testdata")
	}
	docs := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		docs[filepath.Base(f)] = string(b)
	}
	return docs
}

func TestCompleteDocsUnchanged(t *testing.T) {
	for name, doc := range corpusDocs(t) {
		doc = strings.TrimRight(doc, "\n") + "\n"
		if got := Close(doc); got != doc {
			t.Errorf("%s: Close changed a complete document: %q", name, firstDiff(doc, got))
		}
	}
}

// Idempotence is not claimed by llx for every prefix: a hidden tail can expose a new one. It holds
// when the healed text is itself complete, which is what a renderer feeds back.
func TestPrefixesNeverPanicAndKeepHead(t *testing.T) {
	for name, doc := range corpusDocs(t) {
		for i := 0; i <= len(doc); i++ {
			p := doc[:i]
			got := Close(p)
			// Inline fixes touch only the last paragraph: everything before it is untouched.
			if at := strings.LastIndex(p, "\n\n"); at >= 0 && openFence(p) == nil && !strings.HasPrefix(got, p[:at+2]) &&
				!strings.HasPrefix(p[:at+2], got) {
				t.Fatalf("%s@%d: head changed: %q -> %q", name, i, p, got)
			}
		}
	}
}

func TestIdempotentOnInlineFixes(t *testing.T) {
	for _, s := range []string{
		"this is **import", "was ~~remov", "call `fetchCata", "see [docs](https://exa",
		"this is *important", "this is _important", "intro\n```ts\nconst a = 1", "~~~~\ncode",
		"look ![diag", "Energy is $E = mc", "Done $a$ and $b", "a ** b\n\nnow **bol",
	} {
		once := Close(s)
		if twice := Close(once); twice != once {
			t.Errorf("Close not idempotent on %q: %q then %q", s, once, twice)
		}
	}
}

func firstDiff(a, b string) string {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[max(0, i-20):min(len(a), i+20)]
		}
	}
	return "length differs"
}

// TestTabBeforeFence: CommonMark (and blocks.Split) take a fence's indent
// as up to three spaces. A tab is four columns, so "\t```" starts indented
// code: closing it would add a fence the final text does not have. Not in
// the TS: its \s{0,3} takes the tab as one space.
func TestTabBeforeFence(t *testing.T) {
	for _, s := range []string{"para\n\n\t```\ncode", "\t~~~\ncode", " \t```\ncode", " ```\ncode", "    ```\ncode"} {
		if _, ok := OpenFenceBody(s); ok {
			t.Errorf("OpenFenceBody(%q) is a fence", s)
		}
		if got := Close(s); strings.HasSuffix(got, "\n```") || strings.HasSuffix(got, "\n~~~") {
			t.Errorf("Close(%q) = %q closes a fence", s, got)
		}
	}
	run(t, "up to three spaces before ``` is a fence",
		pair{"   ```\ncode", "   ```\ncode\n```"})
}
