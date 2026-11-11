package tablestream

import (
	"math"
	"sort"
)

// modelWidths returns the column widths lipgloss v2.1.0 gives a table with
// header and rows of these cell display widths, at table width width, with
// the cell style glamour sets (ansi/table.go setStyles: margin 1 each side,
// no padding in glamour's default styles: frame 2) and its borders (column
// separators only).
//
// Ported from lipgloss table/resizing.go (resize, newResizer,
// optimizedWidths, expandTableWidth, shrinkTableWidth, minWidth; MIT;
// v2.1.0 added the minWidth floor pass). It is not read
// from a rendered table: lipgloss cuts each line at the table width, so a
// table drawn wider shows the separator of one that fits.
func modelWidths(head []int, rows [][]int, width int) []int {
	const frame = 2
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	// String pads the header to the data's columns; DataToMatrix pads rows.
	all := make([][]int, 0, len(rows)+1)
	h := append([]int(nil), head...)
	for len(h) < cols {
		h = append(h, 0)
	}
	all = append(all, h)
	for _, r := range rows {
		p := append([]int(nil), r...)
		for len(p) < cols {
			p = append(p, 0)
		}
		all = append(all, p)
	}
	type column struct {
		min, max int
		widths   []int // rows after the one that added the column
	}
	var cs []column
	for _, r := range all {
		for i, w := range r {
			if len(cs) <= i {
				cs = append(cs, column{min: w, max: w})
				continue
			}
			cs[i].widths = append(cs[i].widths, w)
			cs[i].min = min(cs[i].min, w)
			cs[i].max = max(cs[i].max, w)
		}
	}
	border := len(cs) - 1
	ws := make([]int, len(cs))
	total := 0
	for i, c := range cs {
		ws[i] = c.max + frame
		total += ws[i]
	}
	if total <= width {
		// expandTableWidth: widen the narrowest (first) column until it fits.
		for sum(ws)+border < width {
			at, least := 0, math.MaxInt32
			for j, w := range ws {
				if w < least {
					at, least = j, w
				}
			}
			ws[at]++
		}
		return ws
	}
	medians := make([]int, len(cs))
	for j, c := range cs {
		medians[j] = median(c.widths)
	}
	// floor: lipgloss minWidth, the frame plus the narrowest cell (at least 1).
	floor := func(j int) int { return frame + max(cs[j].min, 1) }
	// A width equal to the fixed width (0: none set) is skipped, as lipgloss does;
	// with useFloor, so is a width at or below the floor.
	biggest := func(veryBigOnly, useFloor bool) {
		for sum(ws)+border > width {
			at, big := -math.MaxInt32, -math.MaxInt32
			for j, w := range ws {
				if w == 0 || useFloor && w <= floor(j) {
					continue
				}
				if veryBigOnly {
					if w >= width/2 && w > big {
						at, big = j, w
					}
				} else if w > big {
					at, big = j, w
				}
			}
			if at < 0 || ws[at] == 0 {
				break
			}
			ws[at]--
		}
	}
	toMedian := func(useFloor bool) {
		for sum(ws)+border > width {
			at, diff := -math.MaxInt32, -math.MaxInt32
			for j, w := range ws {
				if w == 0 || useFloor && w <= floor(j) {
					continue
				}
				d := w - medians[j]
				if d > 0 && d > diff {
					at, diff = j, d
				}
			}
			// lipgloss stops at index 0 too (`<= 0`).
			if at <= 0 || ws[at] == 0 {
				break
			}
			ws[at]--
		}
	}
	biggest(true, true)
	toMedian(true)
	biggest(false, true)
	// Still too wide (a width budget below the floors): again with no floor.
	if sum(ws)+border > width {
		biggest(true, false)
		toMedian(false)
		biggest(false, false)
	}
	return ws
}

// median is lipgloss table/util.go median (it sorts its argument).
func median(n []int) int {
	s := append([]int(nil), n...)
	sort.Ints(s)
	if len(s) == 0 {
		return 0
	}
	if len(s)%2 == 0 {
		h := len(s) / 2
		return (s[h-1] + s[h]) / 2
	}
	return s[len(s)/2]
}
