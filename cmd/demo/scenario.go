package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

// scenario is one reply to stream. src[:start] is on the page before the
// stream begins (thread: finished replies above the one streaming).
type scenario struct {
	name, about string
	src         string
	start       int
}

// scenarioNames is the order of --scenario values and of the c key.
var scenarioNames = []string{"all", "showcase", "heal", "unsafe", "long", "code", "forms", "thread"}

// scenarioByName builds one scenario; corpus text is generated on demand.
func scenarioByName(name string) (scenario, error) {
	gen := func(n string) corpus.Scenario {
		for _, s := range corpus.Scenarios() {
			if s.Name == n {
				return s
			}
		}
		panic("corpus has no " + n)
	}
	switch name {
	case "all":
		long := gen("long-100k")
		return scenario{name: name, about: "showcase → half-written Markdown → terminal controls → long-100k",
			src: strings.Join([]string{showcase, healText, unsafeText, long.Text}, "\n\n")}, nil
	case "showcase":
		return scenario{name: name, about: "every block kind and llx form, 207 lines", src: showcase}, nil
	case "heal":
		return scenario{name: name, about: "half-written Markdown: press p mid-stream", src: healText}, nil
	case "unsafe":
		return scenario{name: name, about: "terminal escapes, controls and bidi overrides in model output", src: unsafeText}, nil
	case "long", "code", "forms":
		c := gen(map[string]string{"long": "long-100k", "code": "code-50k", "forms": "forms-20k"}[name])
		return scenario{name: name, about: fmt.Sprintf("corpus %s: %s", c.Name, c.About), src: c.Text}, nil
	case "thread":
		c := gen("thread-30")
		hist := strings.Join(c.History, "\n\n") + "\n\n"
		return scenario{name: name, about: "corpus thread-30: 30 finished replies on the page, then one streams; naive renders the page per token",
			src: hist + c.Text, start: len(hist)}, nil
	}
	return scenario{}, fmt.Errorf("--scenario %q: want one of %s", name, strings.Join(scenarioNames, ", "))
}

// healText keeps spans open for many tokens, so a freeze (p) at most
// moments lands inside one: bold, a link, inline code, a table row, an
// llx marker, a code fence.
const healText = `## Half-written Markdown

Press **p** while this part streams: both panes stop on the same token. **This bold phrase runs on for a long time, so a freeze lands inside it: the left pane draws it bold from the first letter, glamour shows the two asterisks until the phrase closes.** Then a link with long text: [the bubbleteandmd repository, with its README and the decisions behind it](https://github.com/iodesystems/bubbleteandmd), and ` + "`" + `inline code that runs on for a while before its closing backtick` + "`" + `.

| Open span | glamour shows                         | bubbleteandmd shows              |
|-----------|---------------------------------------|----------------------------------|
| bold      | two asterisks and plain text          | bold text                        |
| link      | a bracket and the raw text            | the link text                    |
| table     | the header as a paragraph, half rows  | the table, held until it is one  |
| marker    | the marker as quoted text             | the callout                      |

> [!WARNING] A callout marker
> Until the marker closes, glamour draws it as a quote that starts with a bracket.

` + "```" + `python
def heal(prefix):
    # an open fence: the lines are code from the first one
    spans = open_spans(prefix)
    return prefix + "".join(close(s) for s in reversed(spans))
` + "```" + `

> [!STEPS]
1. **Freeze** with p
2. **Step** one token with n
3. **Go on** with p again`

// unsafeText is model output with bytes a terminal reads as commands.
// The left pane renders it through Stream, which calls Sanitize; the
// right pane would pass them to the terminal, so the demo shows them as
// control pictures there (visible).
const unsafeText = "## Terminal controls in model output\n\n" +
	"A model can be led to print bytes that a terminal runs as commands. " +
	"This sentence sets the window title\x1b]0;pwned\x07, clears the screen\x1b[2J\x1b[H, " +
	"writes the clipboard\x1b]52;c;cm0gLXJmIH4=\x07, hides a link \x1b]8;;https://example.com/x\x1b\\here\x1b]8;;\x1b\\, " +
	"and turns a name around with a bidi override: invoice‮fdp.exe‬.\n\n" +
	"| Tool | Output |\n|------|--------|\n| ls   | \x1b[5mblinking\x1b[0m |\n| tput | leave the alt screen\x1b[?1049l |\n\n" +
	"```sh\nprintf '\x1b[?25l'   # hide the cursor\n```\n\n" +
	"The left pane drops these bytes (Sanitize); the right pane shows them as ␛, ␇ and ⟨U+202E⟩ in place of running them."

// visible replaces each rune Sanitize would remove with a picture of it,
// so the naive pane shows the controls glamour passes through without
// sending them to this terminal. n counts the runes replaced.
func visible(src string) (out string, n int) {
	var b strings.Builder
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		i += size
		switch {
		case r == '\t' || r == '\n':
			b.WriteRune(r)
			continue
		case r < 0x20:
			b.WriteRune(0x2400 + r) // ␀..␟, ESC is ␛
		case r == 0x7f:
			b.WriteRune('␡')
		case r == utf8.RuneError && size == 1, r >= 0x80 && r <= 0x9f,
			r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			fmt.Fprintf(&b, "⟨U+%04X⟩", r)
		default:
			b.WriteRune(r)
			continue
		}
		n++
	}
	if n == 0 {
		return src, 0
	}
	return b.String(), n
}
