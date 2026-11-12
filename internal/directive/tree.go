// Ported from llx-markdown packages/llx-markdown/src/core/file-tree-parse.ts (MIT, see NOTICE).

package directive

import (
	"regexp"
	"strconv"
	"strings"
)

// TreeRow is one row of a ```tree fence.
//
// Depth is the number of 4-column guides (`│   ` or four spaces) before a
// branch (`├── ` / `└── `); a line with no branch is a root. A row is a folder
// when its name ends with `/` or the next row is deeper. A comment, after `#`
// or after two or more spaces, is set apart from the name. A line the grammar
// does not fit is kept as a plain row at its depth.
type TreeRow struct {
	Depth   int
	Name    string
	Comment string
	Folder  bool
	// Path is the row's place in the tree, `src/app/App.tsx#4`: a key unique
	// per row, since a drawing may repeat a name.
	Path string
	// Parent is the index of the nearest shallower row above, or -1.
	Parent int
}

// jsSpace is the class JavaScript's \s matches.
const jsSpace = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

var (
	treeGuide   = regexp.MustCompile(`^(?:│ {3}|\| {3}| {4})`)
	treeBranch  = regexp.MustCompile("^(?:├──|└──|\\|--|`--|\\+--)[" + jsSpace + "]?")
	treeComment = regexp.MustCompile(`[` + jsSpace + `]+#|[` + jsSpace + `]{2,}`)
	commentLead = regexp.MustCompile(`^[` + jsSpace + `]*#?[` + jsSpace + `]*`)
)

func trimJS(s string) string { return strings.Trim(s, jsSpaceChars) }

const jsSpaceChars = "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

func readTreeLine(line string) (TreeRow, bool) {
	rest := strings.TrimRight(line, jsSpaceChars)
	if trimJS(rest) == "" {
		return TreeRow{}, false
	}
	depth := 0
	for {
		m := treeGuide.FindString(rest)
		if m == "" {
			break
		}
		rest = rest[len(m):]
		depth++
	}
	if m := treeBranch.FindString(rest); m != "" {
		rest = rest[len(m):]
		depth++
	} else {
		rest = strings.TrimLeft(rest, jsSpaceChars)
	}
	name, comment := rest, ""
	if at := treeComment.FindStringIndex(rest); at != nil {
		name = rest[:at[0]]
		comment = trimJS(commentLead.ReplaceAllString(rest[at[0]:], ""))
	}
	return TreeRow{Depth: depth, Name: trimJS(name), Comment: comment}, true
}

// CursorRow is the index in drawn (the TreeRow.Path of each drawn row of a
// tree, in row order) of the row that shows the cursor (TreeState.Cursor):
// the row cursor names; for a row hidden under a closed folder, that
// folder (the last drawn row above it whose path is an ancestor of
// cursor's); else ("", or no such row) the first row. The renderer and
// the Doc both read the cursor with it. -1 when drawn is empty.
func CursorRow(drawn []string, cursor string) int {
	if len(drawn) == 0 {
		return -1
	}
	if cursor == "" {
		return 0
	}
	plain, at := splitPath(cursor)
	best := 0
	for i, p := range drawn {
		if p == cursor {
			return i
		}
		if q, j := splitPath(p); j < at && strings.HasPrefix(plain, strings.TrimSuffix(q, "/")+"/") {
			best = i
		}
	}
	return best
}

// splitPath splits a TreeRow.Path into its plain path and row index (-1
// when it has none).
func splitPath(p string) (string, int) {
	k := strings.LastIndexByte(p, '#')
	if k < 0 {
		return p, -1
	}
	i, err := strconv.Atoi(p[k+1:])
	if err != nil {
		return p, -1
	}
	return p[:k], i
}

// ParseTree reads the body of a ```tree fence into rows.
func ParseTree(source string) []TreeRow {
	var rows []TreeRow
	for _, line := range strings.Split(source, "\n") {
		if row, ok := readTreeLine(line); ok {
			rows = append(rows, row)
		}
	}
	plain := make([]string, len(rows))
	for i := range rows {
		rows[i].Parent = -1
		for above := i - 1; above >= 0; above-- {
			if rows[above].Depth < rows[i].Depth {
				rows[i].Parent = above
				break
			}
		}
		if p := rows[i].Parent; p >= 0 {
			plain[i] = strings.TrimSuffix(plain[p], "/") + "/" + rows[i].Name
		} else {
			plain[i] = rows[i].Name
		}
		deeper := i+1 < len(rows) && rows[i+1].Depth > rows[i].Depth
		rows[i].Folder = strings.HasSuffix(rows[i].Name, "/") || deeper
		rows[i].Path = plain[i] + "#" + strconv.Itoa(i)
	}
	return rows
}
