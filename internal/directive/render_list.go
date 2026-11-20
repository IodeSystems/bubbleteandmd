package directive

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// listLines renders Markdown through c.Inner and returns its lines without
// leading or trailing blank lines, each cut to width cells.
func listLines(c Ctx, md string, width int) ([]string, error) {
	lines, _, err := listBody(c, md, width, "")
	return lines, err
}

// listBody is listLines for a form body under slot (Ctx.Body), with its
// marks; slot "" renders through Inner.
func listBody(c Ctx, md string, width int, slot string) ([]string, []Mark, error) {
	if strings.TrimSpace(md) == "" {
		return nil, nil, nil
	}
	if width < 1 {
		width = 1
	}
	var s string
	var marks []Mark
	var err error
	if slot == "" {
		s, err = c.Inner(md, width)
	} else {
		s, marks, err = c.body(md, width, slot)
	}
	if err != nil {
		return nil, nil, err
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	drop := 0
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
		drop++
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(strings.TrimRight(l, " "), width, "")
	}
	return lines, place(marks, drop, len(lines), 0), nil
}

// listInline renders inline Markdown on one line, for a stat cell.
func listInline(c Ctx, md string, width int) (string, error) {
	lines, err := listLines(c, md, width)
	if err != nil {
		return "", err
	}
	return strings.Join(lines, " "), nil
}

// listStyle styles every line; the zero style (notty) leaves it unchanged.
func listStyle(st lipgloss.Style, lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if l == "" {
			continue
		}
		out[i] = st.Render(l)
	}
	return out
}

func listClamp(lines []string, width int) []string {
	for i, l := range lines {
		if ansi.StringWidth(l) > width {
			lines[i] = ansi.Truncate(l, width, "")
		}
	}
	return lines
}

func listDigits(n int) int { return len(strconv.Itoa(n)) }

func renderSteps(d Directive, c Ctx) ([]string, []Mark, error) {
	s := d.Steps
	if s == nil {
		return nil, nil, nil
	}
	var marks []Mark
	th := c.Theme
	var out []string
	if s.Title != "" {
		t, err := listLines(c, s.Title, c.Width)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, listStyle(th.Heading, t)...)
	}
	first := 1
	if s.Ordered && s.Start > 0 {
		first = s.Start
	}
	nw := listDigits(first + len(s.Items) - 1)
	gutter := nw + 3 // "n ● "
	cw := c.Width - gutter
	if cw < 1 {
		cw = 1
	}
	rail := strings.Repeat(" ", nw+1) + th.Muted.Render("│") + " "
	bare := strings.Repeat(" ", nw+1) + th.Muted.Render("│")
	for i, it := range s.Items {
		num := first + i
		mark := th.Heading.Render(strings.Repeat(" ", nw-listDigits(num))+strconv.Itoa(num)+" ●") + " "

		var head []string
		if it.Title != "" {
			t, err := listLines(c, it.Title, cw)
			if err != nil {
				return nil, nil, err
			}
			head = listStyle(th.Heading, t)
			if it.When != "" {
				w, err := listInline(c, it.When, cw)
				if err != nil {
					return nil, nil, err
				}
				w = th.Muted.Render(w)
				if n := len(head); n > 0 && ansi.StringWidth(head[n-1])+1+ansi.StringWidth(w) <= cw {
					head[n-1] += " " + w
				} else {
					wl, _ := listLines(c, it.When, cw)
					head = append(head, listStyle(th.Muted, wl)...)
				}
			}
		}
		body, bm, err := listBody(c, it.Body, cw, "s"+strconv.Itoa(i))
		if err != nil {
			return nil, nil, err
		}
		marks = append(marks, place(bm, 0, len(body), len(out)+len(head))...)
		lines := append(head, body...)
		if len(lines) == 0 {
			lines = []string{""}
		}
		for j, l := range lines {
			if j == 0 {
				out = append(out, strings.TrimRight(mark+l, " "))
			} else if l == "" {
				out = append(out, bare)
			} else {
				out = append(out, rail+l)
			}
		}
		if i < len(s.Items)-1 {
			out = append(out, bare)
		}
	}
	return listClamp(out, c.Width), marks, nil
}

// listWord is one unbreakable word with its style.
type listWord struct {
	text string
	st   lipgloss.Style
}

func listWords(text string, st lipgloss.Style, width int) []listWord {
	var ws []listWord
	for _, f := range strings.Fields(text) {
		for ansi.StringWidth(f) > width {
			head := ansi.Truncate(f, width, "")
			if head == "" {
				break
			}
			ws = append(ws, listWord{head, st})
			f = f[len(head):]
		}
		if f != "" {
			ws = append(ws, listWord{f, st})
		}
	}
	return ws
}

func renderSources(d Directive, c Ctx) ([]string, error) {
	s := d.Sources
	if s == nil {
		return nil, nil
	}
	th := c.Theme
	var out []string
	if s.Title != "" {
		t, err := listLines(c, s.Title, c.Width)
		if err != nil {
			return nil, err
		}
		out = append(out, listStyle(th.Heading, t)...)
	}
	nw := listDigits(len(s.Items))
	gutter := nw + 2 // "n. "
	cw := c.Width - gutter
	if cw < 1 {
		cw = 1
	}
	plain := func(md string) (string, error) {
		t, err := listInline(c, md, cw)
		return ansi.Strip(t), err
	}
	for i, it := range s.Items {
		var words []listWord
		if it.URL == "" && it.Title == "" {
			n, err := plain(it.Note)
			if err != nil {
				return nil, err
			}
			words = listWords(n, th.Text, cw)
		} else {
			title, err := plain(it.Title)
			if err != nil {
				return nil, err
			}
			if it.Host == "" && title != "" && strings.Contains(it.URL, title) {
				title = it.URL // bare URL: show the address itself
			}
			words = listWords(title, th.Link, cw)
			if it.Host != "" {
				words = append(words, listWord{"·", th.Muted})
				words = append(words, listWords(it.Host, th.Muted, cw)...)
			}
			if it.Note != "" {
				n, err := plain(it.Note)
				if err != nil {
					return nil, err
				}
				words = append(words, listWords(n, th.Muted, cw)...)
			}
		}
		num := strings.Repeat(" ", nw-listDigits(i+1)) + strconv.Itoa(i+1) + "."
		prefix := th.Muted.Render(num) + " "
		indent := strings.Repeat(" ", gutter)
		var cur strings.Builder
		curW := 0
		var lines []string
		for _, w := range words {
			ww := ansi.StringWidth(w.text)
			if curW > 0 && curW+1+ww > cw {
				lines = append(lines, cur.String())
				cur.Reset()
				curW = 0
			}
			if curW > 0 {
				cur.WriteString(" ")
				curW++
			}
			cur.WriteString(w.st.Render(w.text))
			curW += ww
		}
		if cur.Len() > 0 || len(lines) == 0 {
			lines = append(lines, cur.String())
		}
		for j, l := range lines {
			if j == 0 {
				out = append(out, prefix+l)
			} else {
				out = append(out, indent+l)
			}
		}
	}
	return listClamp(out, c.Width), nil
}
