package directive

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

// renderTree draws a ```tree fence: guides redrawn, all folders open. A
// tree with state (State.Trees) marks each folder "▾ " open or "▸ "
// closed and hides the rows under a closed one. Focused, the cursor row's
// name is drawn with Theme.Focus.
func renderTree(d Directive, c Ctx) ([]string, []Mark, error) {
	if d.Tree == nil {
		return nil, nil, nil
	}
	rows := d.Tree.Rows
	th := c.Theme
	ts, folds := c.State.tree(c.Key)
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
	// shown[i]: no folder above row i is closed. Rows under a hidden row are
	// hidden, so only the parent needs a look.
	shown := make([]bool, len(rows))
	for i, r := range rows {
		p := r.Parent
		shown[i] = p < 0 || (shown[p] && !ts.Closed[rows[p].Path])
	}
	cursor := -1
	if c.State.focused(c.Key) && len(rows) > 0 {
		cursor = 0
		for i, r := range rows {
			if ts.Cursor != "" && r.Path == ts.Cursor {
				cursor = i
				break
			}
		}
		for cursor >= 0 && !shown[cursor] {
			cursor = rows[cursor].Parent
		}
	}
	lastAt := map[int]bool{}
	var out []string
	var marks []Mark
	for i, r := range rows {
		lastAt[r.Depth] = last[i]
		if !shown[i] {
			continue
		}
		var b strings.Builder
		for k := 1; k < r.Depth; k++ {
			if lastAt[k] {
				b.WriteString("    ")
			} else {
				b.WriteString(th.Text.Render("│   "))
			}
		}
		// notty draws focus as a glyph swap of the same width: the fold
		// mark, or for a row with none the guide's rule.
		glyph := th.Plain && i == cursor && r.Folder && folds
		rule := "── "
		if th.Plain && i == cursor && !glyph {
			rule = "━━ "
		}
		if r.Depth >= 1 {
			if last[i] {
				b.WriteString(th.Text.Render("└" + rule))
			} else {
				b.WriteString(th.Text.Render("├" + rule))
			}
		}
		name := r.Name
		if r.Folder {
			if !strings.HasSuffix(name, "/") {
				name += "/"
			}
			if folds {
				if ts.Closed[r.Path] {
					name = th.foldGlyph("▸", "▶", glyph) + " " + name
				} else {
					name = th.foldGlyph("▾", "▼", glyph) + " " + name
				}
			}
			b.WriteString(th.focus(th.Heading, i == cursor).Render(name))
		} else {
			b.WriteString(th.focus(th.Text, i == cursor).Render(name))
		}
		if r.Comment != "" {
			b.WriteString("  " + th.Muted.Render("# "+r.Comment))
		}
		line := b.String()
		if xansi.StringWidth(line) > c.Width {
			line = xansi.Truncate(line, c.Width, "…")
		}
		marks = append(marks, Mark{Key: c.Key, Kind: MarkRow, Line: len(out), Height: 1, Index: i, Path: r.Path})
		out = append(out, line)
	}
	return out, marks, nil
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
	for i, l := range lines {
		if xansi.StringWidth(l) > c.Width {
			lines[i] = xansi.Truncate(l, c.Width, "…")
		}
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
