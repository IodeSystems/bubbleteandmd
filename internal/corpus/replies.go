// Ported from llx-markdown packages/llx-markdown/test/replies.ts (MIT, see NOTICE).

package corpus

import (
	"fmt"
	"strings"
)

var words = strings.Fields("the a migration table index query cache request worker session token reply stream block parser renderer layout value config build release test agent model tool file branch commit review error retry timeout queue lock write read")

func sentence(r *Rand) string {
	n := r.Int(6, 18)
	ws := make([]string, n)
	for i := range ws {
		ws[i] = Pick(r, words)
	}
	at := r.Int(0, len(ws)-1)
	switch r.Int(0, 9) {
	case 0:
		ws[at] = "**" + ws[at] + "**"
	case 1:
		ws[at] = "`" + ws[at] + "()`"
	case 2:
		ws[at] = "[" + ws[at] + "](https://example.com/" + ws[at] + ")"
	case 3:
		ws[at] = "_" + ws[at] + "_"
	case 4:
		ws[at] = fmt.Sprintf("$%d", r.Int(2, 99))
	}
	text := strings.Join(ws, " ")
	return strings.ToUpper(text[:1]) + text[1:] + "."
}

// head is sentence(r).slice(0, n); all generated text is ASCII.
func head(r *Rand, n int) string {
	s := sentence(r)
	if len(s) > n {
		s = s[:n]
	}
	return s
}

func paragraph(r *Rand) string {
	n := r.Int(2, 5)
	ss := make([]string, n)
	for i := range ss {
		ss[i] = sentence(r)
	}
	return strings.Join(ss, " ")
}

func list(r *Rand, ordered bool) string {
	n := r.Int(3, 7)
	ss := make([]string, n)
	for i := range ss {
		mark := "-"
		if ordered {
			mark = fmt.Sprintf("%d.", i+1)
		}
		ss[i] = mark + " " + sentence(r)
	}
	return strings.Join(ss, "\n")
}

// code: lines < 0 draws the default r.Int(6, 30).
func code(r *Rand, lines int) string {
	if lines < 0 {
		lines = r.Int(6, 30)
	}
	lang := Pick(r, []string{"ts", "py", "sh", "json", "rust"})
	body := make([]string, lines)
	for i := range body {
		name := Pick(r, words)
		indent := strings.Repeat("  ", r.Int(0, 3))
		fn := Pick(r, words)
		num := r.Int(0, 999)
		cm := Pick(r, words)
		body[i] = fmt.Sprintf("%sconst %s%d = await %s(%s, %d) // %s", indent, name, i, fn, name, num, cm)
	}
	return "```" + lang + "\n" + strings.Join(body, "\n") + "\n```"
}

// table: rows < 0 draws the default r.Int(3, 8).
func table(r *Rand, rows int) string {
	if rows < 0 {
		rows = r.Int(3, 8)
	}
	out := []string{"| Name | Status | Time | Note |\n| --- | --- | --- | --- |"}
	for i := 0; i < rows; i++ {
		name := Pick(r, words)
		status := Pick(r, []string{"ok", "failed", "skipped"})
		ms := r.Int(1, 900)
		out = append(out, fmt.Sprintf("| %s | %s | %d ms | %s |", name, status, ms, sentence(r)))
	}
	return strings.Join(out, "\n")
}

func callout(r *Rand) string {
	kind := Pick(r, []string{"NOTE", "TIP", "WARNING", "IMPORTANT", "CAUTION"})
	a := sentence(r)
	b := sentence(r)
	return "> [!" + kind + "]\n> " + a + "\n> " + b
}

func steps(r *Rand) string {
	n := r.Int(3, 5)
	items := make([]string, n)
	for i := range items {
		title := head(r, 30)
		items[i] = fmt.Sprintf("%d. **%s**\n   *Day %d*\n   %s", i+1, title, i+1, sentence(r))
	}
	return "> [!STEPS]\n" + strings.Join(items, "\n")
}

func tabs(r *Rand) string {
	var parts []string
	for _, label := range []string{"macOS", "Linux", "Windows"} {
		parts = append(parts, "#### "+label+"\n"+code(r, r.Int(2, 5)))
	}
	return "> [!TABS]\n" + strings.Join(parts, "\n")
}

func stats(r *Rand) string {
	items := make([]string, 3)
	for i := range items {
		pct := r.Int(2, 99)
		items[i] = fmt.Sprintf("- **%d%%** %s", pct, head(r, 40))
	}
	return "> [!STATS]\n" + strings.Join(items, "\n")
}

func verdicts(r *Rand) string {
	pros := list(r, false)
	cons := list(r, false)
	return "> [!PROS]\n" + pros + "\n\n> [!CONS]\n" + cons
}

func tree(r *Rand) string {
	n := r.Int(4, 10)
	ls := make([]string, n)
	for i := range ls {
		indent := strings.Repeat("  ", r.Int(0, 3))
		name := Pick(r, words)
		ext := Pick(r, []string{"ts", "md", "json"})
		ls[i] = indent + name + "." + ext
	}
	return "```tree\n" + strings.Join(ls, "\n") + "\n```"
}

func mathBlock(r *Rand) string {
	a, b, c := r.Int(2, 9), r.Int(1, 9), r.Int(2, 9)
	return fmt.Sprintf("$$\n\\sum_{i=1}^{%d} x_i^2 = \\frac{%d}{%d}\n$$", a, b, c)
}

type part func(*Rand) string

var (
	h2    part = func(r *Rand) string { return "## " + head(r, 40) }
	h3    part = func(r *Rand) string { return "### " + head(r, 30) }
	ul    part = func(r *Rand) string { return list(r, false) }
	ol    part = func(r *Rand) string { return list(r, true) }
	cd    part = func(r *Rand) string { return code(r, -1) }
	tb    part = func(r *Rand) string { return table(r, -1) }
	mixed      = []part{paragraph, paragraph, paragraph, ul, ol, cd, tb, callout, h2, h3}
	forms      = []part{paragraph, steps, tabs, stats, verdicts, callout, tree, h2}
	prose      = []part{paragraph, paragraph, ul, h2}
)

// reply draws blocks from parts until the reply reaches size characters.
func reply(seed uint32, size int, parts []part) string {
	r := Seeded(seed)
	blocks := []string{"# " + head(r, 40)}
	length := len(blocks[0])
	for length < size {
		block := Pick(r, parts)(r)
		blocks = append(blocks, block)
		length += len(block) + 2
	}
	return strings.Join(blocks, "\n\n")
}

// Scenario is one generated reply, a pure function of its seed.
type Scenario struct {
	Name  string
	About string
	// History: replies already finished above the one streaming.
	History []string
	Text    string
}

// Scenarios returns the ten TS scenarios in order.
func Scenarios() []Scenario {
	history := make([]string, 30)
	for i := range history {
		history[i] = reply(uint32(100+i), 3_000, mixed)
	}
	return []Scenario{
		{Name: "chat-5k", About: "A short chat answer: prose and lists.", Text: reply(1, 5_000, prose)},
		{Name: "answer-20k", About: "A typical agent answer: prose, lists, code, tables, callouts.", Text: reply(2, 20_000, mixed)},
		{Name: "forms-20k", About: "Dense dialect: steps, tabs, stats, pros and cons, trees.", Text: reply(3, 20_000, forms)},
		{Name: "math-20k", About: "Prose with formulas and literal dollars.", Text: reply(4, 20_000, append(append([]part{}, mixed...), mathBlock, mathBlock))},
		{Name: "code-50k", About: "An agent writing a file: one code fence that keeps growing.", Text: "Here is the file:\n\n" + code(Seeded(5), 900) + "\n\nDone."},
		{Name: "table-20k", About: "One table that keeps growing: a single block reparsed on every token.", Text: "Results:\n\n" + table(Seeded(6), 160)},
		{Name: "long-100k", About: "A long report.", Text: reply(7, 100_000, mixed)},
		{Name: "crlf-20k", About: "Windows line endings, which some models and proxies send.", Text: strings.ReplaceAll(reply(8, 20_000, mixed), "\n", "\r\n")},
		{Name: "footnotes-20k", About: "A reply with footnotes: definitions resolve across blocks.", Text: reply(9, 20_000, mixed) + "\n\nSee the source.[^1]\n\n[^1]: The reference."},
		{Name: "thread-30", About: "Thirty finished replies above the one streaming, all on the page.", History: history, Text: reply(10, 10_000, mixed)},
	}
}

// Scaling returns the same kind of reply at 10k and 40k characters.
func Scaling() [2]Scenario {
	return [2]Scenario{
		{Name: "answer-10k", About: "Scaling, small.", Text: reply(2, 10_000, mixed)},
		{Name: "answer-40k", About: "Scaling, four times larger.", Text: reply(2, 40_000, mixed)},
	}
}
