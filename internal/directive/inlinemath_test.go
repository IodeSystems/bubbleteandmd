package directive

import "testing"

func TestInlineMath(t *testing.T) {
	for in, want := range map[string]string{
		"Sum: $2 + 2 = 4$.":         "Sum: `2 + 2 = 4`.",
		`A step: $3 \cdot 4$ here.`: "A step: `3 \\cdot 4` here.",
		"$E = mc^2$ and $x_i$":      "`E = mc^2` and `x_i`",
		"From $5 for $n^2$ rows":    "From $5 for `n^2` rows",
		// Prices and variables stay text.
		"$5 and $10 = cheap":      "$5 and $10 = cheap",
		"pay $5 now and 7$ later": "pay $5 now and 7$ later",
		"$HOME/bin and $PATH":     "$HOME/bin and $PATH",
		// Not across a paragraph, a code span, a fence or HTML.
		"$a\n\nb$":                       "$a\n\nb$",
		"`$x$` and $y$":                  "`$x$` and `y`",
		"```\n$x$\n```\n":                "```\n$x$\n```\n",
		"    $x$ indented\n":             "    $x$ indented\n",
		"<div>\n$x$\n</div>\n":           "<div>\n$x$\n</div>\n",
		"a <span title=\"$x$\">b</span>": "a <span title=\"$x$\">b</span>",
		// A backtick in or beside the formula: left as written.
		"$a`b$":    "$a`b$",
		"`c`$x$ d": "`c`$x$ d",
		// Headings, quotes, list items and table cells.
		"#### $x^2$ tab":                     "#### `x^2` tab",
		"> see $a+b$\n> and $c$":             "> see `a+b`\n> and `c`",
		"- item $k$\n":                       "- item `k`\n",
		"| a | b |\n|---|---|\n| $x$ | $5 |": "| a | b |\n|---|---|\n| `x` | $5 |",
		// `$$` is display math, MathFences' to read.
		"$$x$$": "$$x$$",
	} {
		if got := InlineMath(in); got != want {
			t.Errorf("InlineMath(%q) = %q, want %q", in, got, want)
		}
		if got := InlineMath(in); len(got) != len(in) {
			t.Errorf("InlineMath(%q): length %d, want %d", in, len(got), len(in))
		}
	}
}

// TestInlineMathDollars: notty keeps a formula's `$` in the code span, and
// the shifts map each byte of the rewritten text that src holds back to it.
func TestInlineMathDollars(t *testing.T) {
	for in, want := range map[string]string{
		"Sum: $2 + 2 = 4$.":           "Sum: `$2 + 2 = 4$`.",
		"$a_1 + b_1$ and $x_i$":       "`$a_1 + b_1$` and `$x_i$`",
		`set $\{x\}$ here`:            "set `$\\{x\\}$` here",
		"From $5 for $n^2$ rows":      "From $5 for `$n^2$` rows",
		"$5 and $10 = cheap":          "$5 and $10 = cheap",
		"`$x$` and $y$":               "`$x$` and `$y$`",
		"> see $a+b$\n>\n> and $c$\n": "> see `$a+b$`\n>\n> and `$c$`\n",
		"$a`b$":                       "$a`b$",
	} {
		got, shifts := InlineMathDollars(in)
		if got != want {
			t.Errorf("InlineMathDollars(%q) = %q, want %q", in, got, want)
			continue
		}
		if len(shifts) != len(got)-len(in) {
			t.Errorf("InlineMathDollars(%q): %d shifts, want %d", in, len(shifts), len(got)-len(in))
		}
		// Bytes of got that are not inserted backticks, in order, are in.
		ins := map[int]bool{}
		for _, s := range shifts {
			ins[s.At-1] = true
		}
		for i := range len(got) {
			if ins[i] {
				continue
			}
			if r := RawOffset(shifts, i); r < 0 || r >= len(in) || in[r] != got[i] {
				t.Errorf("InlineMathDollars(%q): byte %d maps to %d", in, i, r)
				break
			}
		}
	}
}
