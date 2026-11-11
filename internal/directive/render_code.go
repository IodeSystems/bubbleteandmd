package directive

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

// renderTree draws a ```tree fence: all folders open, guides redrawn.
func renderTree(d Directive, c Ctx) ([]string, error) {
	if d.Tree == nil {
		return nil, nil
	}
	rows := d.Tree.Rows
	th := c.Theme
	last := make([]bool, len(rows))
	for i, r := range rows {
		last[i] = true
		for j := i + 1; j < len(rows) && rows[j].Depth >= r.Depth; j++ {
			if rows[j].Depth == r.Depth {
				last[i] = false
				break
			}
		}
	}
	lastAt := map[int]bool{}
	var out []string
	for i, r := range rows {
		var b strings.Builder
		for k := 1; k < r.Depth; k++ {
			if lastAt[k] {
				b.WriteString("    ")
			} else {
				b.WriteString(th.Text.Render("│   "))
			}
		}
		if r.Depth >= 1 {
			if last[i] {
				b.WriteString(th.Text.Render("└── "))
			} else {
				b.WriteString(th.Text.Render("├── "))
			}
		}
		lastAt[r.Depth] = last[i]
		name := r.Name
		if r.Folder {
			if !strings.HasSuffix(name, "/") {
				name += "/"
			}
			b.WriteString(th.Heading.Render(name))
		} else {
			b.WriteString(th.Text.Render(name))
		}
		if r.Comment != "" {
			b.WriteString("  " + th.Muted.Render("# "+r.Comment))
		}
		line := b.String()
		if xansi.StringWidth(line) > c.Width {
			line = xansi.Truncate(line, c.Width, "…")
		}
		out = append(out, line)
	}
	return out, nil
}

// renderCodeTitle draws a title line over the code block Inner renders.
func renderCodeTitle(d Directive, c Ctx) ([]string, error) {
	if d.CodeTitle == nil {
		return nil, nil
	}
	ct := d.CodeTitle
	title := c.Theme.Code.Render("▤ " + ct.Title)
	if xansi.StringWidth(title) > c.Width {
		title = xansi.Truncate(title, c.Width, "…")
	}
	body := ct.Body
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	fence := codeTitleFence(body)
	md := fence + ct.Lang + "\n" + body + fence + "\n"
	s, err := c.Inner(md, c.Width)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(xansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(xansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return append([]string{title}, lines...), nil
}

// codeTitleFence is a backtick fence longer than any backtick run at a line start of body.
func codeTitleFence(body string) string {
	n := 3
	for _, l := range strings.Split(body, "\n") {
		t := strings.TrimLeft(l, " ")
		k := 0
		for k < len(t) && t[k] == '`' {
			k++
		}
		if k >= n {
			n = k + 1
		}
	}
	return strings.Repeat("`", n)
}
