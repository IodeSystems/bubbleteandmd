package spike_test

// Layer 3 spike: can the line of each interactive form header (DETAILS
// title, TABS label line, each TABS panel header, tree rows) in the output
// of Stream.Frame/Final be computed from per-piece renders, without parsing
// the output?
//
// lineMap is the proposed rule: piece k starts at line
// count("\n", P) + Σ count("\n", piece j<k). DropCells on an open line
// (a piece that does not end in "\n") changes cells of one line, never the
// number of "\n", so the rule does not need a case for it.
//
// mirror re-does Stream.render and DirectiveRenderer.render with marks: each
// form renderer's []string lines give the local line of each header, Frame
// adds 1 for a not-first piece, nested bodies are shifted where the
// renderer puts the Inner lines (leading "\n" trimmed by tabsInner). Its
// output is checked against Stream.Frame (text), and every mark against the
// output line that holds a marker planted in the source (ansi.Strip).

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/iodesystems/bubbleteandmd"
	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
	"github.com/iodesystems/bubbleteandmd/internal/heal"
	"github.com/iodesystems/bubbleteandmd/internal/join"
)

// mark is one interactive header: kind tabs|panel|details|row, name its
// label/title/row name, line its line (local to the piece or string it is
// carried with).
type mark struct {
	kind, name string
	folder     bool
	line       int
}

func shift(ms []mark, by int) []mark {
	out := make([]mark, len(ms))
	for i, m := range ms {
		m.line += by
		out[i] = m
	}
	return out
}

// span is a piece's line range [start, end) in the joined output. A piece
// that does not end in "\n" shares its last line with the next piece.
type span struct{ start, end int }

// lineMap is the proposed API core: the line range of each piece of
// join.Join(p, s, pieces).
func lineMap(p string, pieces []string) []span {
	line := strings.Count(p, "\n")
	out := make([]span, len(pieces))
	for i, x := range pieces {
		if x == "" {
			out[i] = span{line, line}
			continue
		}
		nl := strings.Count(x, "\n")
		end := line + nl
		if !strings.HasSuffix(x, "\n") {
			end++
		}
		out[i] = span{line, end}
		line += nl
	}
	return out
}

// mapped is a render with its marks (lines in out).
type mapped struct {
	out   string
	marks []mark
}

type mtarget struct {
	r     *bubbleteandmd.DirectiveRenderer
	p, s  string
	width int
	frame directive.Style
}

type mirror struct {
	style    ansi.StyleConfig
	theme    directive.Theme
	outer    *mtarget
	inners   map[int]*mtarget
	memo     map[string]mapped
	unmapped int // nested marks inside a form kind the mirror does not place
}

func newTarget(t *testing.T, st ansi.StyleConfig, w, width int, frame directive.Style) *mtarget {
	r, err := bubbleteandmd.NewRenderer(st, w)
	if err != nil {
		t.Fatal(err)
	}
	e, err := r.Render("")
	if err != nil {
		t.Fatal(err)
	}
	p, s, err := join.Affixes(e)
	if err != nil {
		t.Fatal(err)
	}
	return &mtarget{r: r, p: p, s: s, width: width, frame: frame}
}

func newMirror(t *testing.T, st ansi.StyleConfig, w int) *mirror {
	th := directive.ThemeFrom(st, directive.PlainStyle(st))
	margin := 0
	if st.Document.Margin != nil {
		margin = int(*st.Document.Margin)
	}
	m := &mirror{style: st, theme: th, inners: map[int]*mtarget{}, memo: map[string]mapped{}}
	m.outer = newTarget(t, st, w, max(w-2*margin, 1), th.Frame)
	return m
}

var padRe = regexp.MustCompile(`(?: |\x1b\[[0-9;]*m +\x1b\[m)+$`)

func (m *mirror) inner(t *testing.T, md string, w int) mapped {
	w = max(w, 1)
	tg, ok := m.inners[w]
	if !ok {
		st := m.style
		zero := uint(0)
		st.Document.Margin, st.Document.Indent = &zero, &zero
		tg = newTarget(t, st, w, w, directive.Style{})
		m.inners[w] = tg
	}
	mp := m.render(t, tg, md)
	lines := strings.Split(mp.out, "\n")
	for i, l := range lines {
		lines[i] = padRe.ReplaceAllString(l, "")
	}
	mp.out = strings.Join(lines, "\n")
	return mp
}

type mpiece struct {
	text  string
	marks []mark
}

func (m *mirror) render(t *testing.T, tg *mtarget, md string) mapped {
	key := fmt.Sprintf("%p\x00%s", tg, md)
	if v, ok := m.memo[key]; ok {
		return v
	}
	body, first := md, true
	if strings.HasPrefix(md, join.Sentinel) {
		body, first = md[len(join.Sentinel):], false
	}
	bs := blocks.Split(body)
	anyForm := false
	for _, b := range bs {
		if _, ok := directive.Parse(b.Source); ok {
			anyForm = true
		}
	}
	var res mapped
	if !anyForm {
		out, err := tg.r.Render(md)
		if err != nil {
			t.Fatal(err)
		}
		res = mapped{out: out}
	} else {
		var pieces []mpiece
		for i, b := range bs {
			pieces = m.block(t, tg, pieces, b.Source, first && i == 0)
		}
		res = joinMapped(tg.p, tg.s, pieces)
	}
	m.memo[key] = res
	return res
}

func joinMapped(p, s string, pieces []mpiece) mapped {
	texts := make([]string, len(pieces))
	for i, x := range pieces {
		texts[i] = x.text
	}
	var marks []mark
	sps := lineMap(p, texts)
	for i, sp := range sps {
		marks = append(marks, shift(pieces[i].marks, sp.start)...)
		if i > 0 && texts[i-1] != "" && !strings.HasSuffix(texts[i-1], "\n") {
			innerSeams++
			for _, m := range pieces[i].marks {
				if m.line == 0 {
					seamMarks++
				}
			}
		}
	}
	return mapped{out: join.Join(p, s, texts), marks: marks}
}

// innerSeams: open-line seams inside DirectiveRenderer joins; seamMarks:
// marks on the shared line of one (counted once per memoized render).
var innerSeams, seamMarks int

func (m *mirror) glamour(t *testing.T, tg *mtarget, pieces []mpiece, src string, first bool) []mpiece {
	if strings.TrimSpace(src) == "" {
		return pieces
	}
	out, err := tg.r.Render(join.Input(src, first))
	if err != nil {
		t.Fatal(err)
	}
	piece, err := join.Strip(out, tg.p, tg.s)
	if err != nil {
		t.Fatal(err)
	}
	if piece == "" {
		return pieces
	}
	return append(pieces, mpiece{text: piece})
}

func (m *mirror) block(t *testing.T, tg *mtarget, pieces []mpiece, src string, first bool) []mpiece {
	d, ok := directive.Parse(src)
	for {
		if !ok {
			return m.glamour(t, tg, pieces, src, first)
		}
		pieces = m.glamour(t, tg, pieces, src[:d.Start], first)
		if len(pieces) > 0 {
			first = false
		}
		lines, marks := m.form(t, d, tg.width)
		if len(lines) > 0 {
			off := 0
			if !first {
				off = 1 // Frame's leading blank line
			}
			pieces = append(pieces, mpiece{directive.Frame(lines, tg.width, tg.frame, first), shift(marks, off)})
			first = false
		}
		src = src[d.End:]
		d, ok = directive.Parse(src)
	}
}

// lead is the number of leading "\n" of s; trimmedLines the line count
// strings.Split(strings.Trim(s, "\n"), "\n") gives (tabsInner).
func lead(s string) int         { return len(s) - len(strings.TrimLeft(s, "\n")) }
func trimmedLines(s string) int { return strings.Count(strings.Trim(s, "\n"), "\n") + 1 }

func (m *mirror) form(t *testing.T, d directive.Directive, w int) ([]string, []mark) {
	var calls []mapped
	c := directive.Ctx{Width: w, Theme: m.theme, Inner: func(md string, w int) (string, error) {
		mp := m.inner(t, md, w)
		calls = append(calls, mp)
		return mp.out, nil
	}}
	lines, _, err := directive.Render(d, c)
	if err != nil {
		t.Fatal(err)
	}
	var marks []mark
	// calloutBody: calloutInner keeps leading lines, drops trailing blanks.
	calloutBody := func(body string) (int, []mark) {
		if strings.TrimSpace(body) == "" {
			return 0, nil
		}
		mp := calls[0]
		out := strings.TrimRight(mp.out, "\n")
		if out == "" {
			return 0, nil
		}
		return strings.Count(out, "\n") + 1, mp.marks
	}
	switch d.Kind {
	case directive.KindTree:
		for i, r := range d.Tree.Rows {
			marks = append(marks, mark{kind: "row", name: r.Name, folder: r.Folder, line: i})
		}
	case directive.KindDetails:
		nb, inner := calloutBody(d.Details.Body)
		nt := len(lines) - nb
		marks = append(marks, mark{kind: "details", name: d.Details.Title, line: 0})
		marks = append(marks, shift(inner, nt)...)
	case directive.KindCallout:
		nb, inner := calloutBody(d.Callout.Body)
		marks = append(marks, shift(inner, len(lines)-nb)...)
	case directive.KindTabs:
		if len(d.Tabs.Panels) == 0 {
			break
		}
		marks = append(marks, mark{kind: "tabs", name: d.Tabs.Panels[0].Label, line: 0})
		line, ci := 1, 0
		for _, p := range d.Tabs.Panels {
			line++ // blank
			marks = append(marks, mark{kind: "panel", name: p.Label, line: line})
			line++
			if strings.TrimSpace(p.Body) != "" {
				mp := calls[ci]
				ci++
				marks = append(marks, shift(mp.marks, line-lead(mp.out))...)
				line += trimmedLines(mp.out)
			}
		}
		if line != len(lines) {
			t.Fatalf("tabs: mirror counts %d lines, renderer drew %d", line, len(lines))
		}
	case directive.KindVerdict:
		v := d.Verdict
		ci := 0
		col := func(items []directive.Item) (int, []mark) {
			n, ms := 1, []mark(nil)
			for _, it := range items {
				if strings.TrimSpace(it.Body) == "" {
					continue
				}
				mp := calls[ci]
				ci++
				ms = append(ms, shift(mp.marks, n-lead(mp.out))...)
				n += trimmedLines(mp.out)
			}
			return n, ms
		}
		hasP, hasC := v.Pros != nil, v.Cons != nil
		if hasP && hasC && w >= 64 {
			_, pm := col(v.Pros)
			_, km := col(v.Cons)
			marks = append(pm, km...) // side by side, top aligned: same rows
			break
		}
		var pn, kn int
		var pm, km []mark
		if hasP {
			pn, pm = col(v.Pros)
		}
		if hasC {
			kn, km = col(v.Cons)
		}
		fn, fm, sm := pn, pm, km
		if v.ConsFirst {
			fn, fm, sm = kn, km, pm
		}
		off := fn
		if pn > 0 && kn > 0 {
			off++
		}
		marks = append(fm, shift(sm, off)...)
	default:
		for _, c := range calls {
			m.unmapped += len(c.marks)
		}
	}
	// directive.Render draws After itself (L3), as the last Inner call.
	if strings.TrimSpace(d.After) != "" {
		after := calls[len(calls)-1]
		if out := strings.Trim(after.out, "\n"); out != "" {
			base := len(lines) - trimmedLines(after.out)
			marks = append(marks, shift(after.marks, base-lead(after.out))...)
		}
	}
	return lines, marks
}

// piece is a top-level Stream piece: rendered as Stream does (Input, Strip).
func (m *mirror) piece(t *testing.T, src string, first bool) mpiece {
	mp := m.render(t, m.outer, join.Input(src, first))
	x, err := join.Strip(mp.out, m.outer.p, m.outer.s)
	if err != nil {
		t.Fatal(err)
	}
	// marks were counted from the start of mp.out, which has P before x.
	return mpiece{x, shift(mp.marks, -strings.Count(m.outer.p, "\n"))}
}

// frame mirrors Stream.render: the pieces and their marks.
func (m *mirror) frame(t *testing.T, src string, healed bool) []mpiece {
	src = bubbleteandmd.Sanitize(src)
	bs := blocks.Split(src)
	stable := max(0, len(bs)-2)
	var pieces []mpiece
	for _, b := range bs[:stable] {
		pieces = append(pieces, m.piece(t, b.Source, b.Offset == 0))
	}
	tailAt, tail := 0, src
	if n := len(bs); n > 0 {
		tailAt, tail = bs[n-1].Offset, src[bs[n-1].Offset:]
		if healed {
			tail = heal.Close(tail)
		}
		if n > 1 {
			near := bs[n-2]
			if healed {
				head := src[near.Offset:tailAt]
				if two := heal.Close(src[near.Offset:]); !strings.HasPrefix(two, head) || two[len(head):] != tail {
					tailAt, tail = near.Offset, two
				}
			}
			if tailAt != near.Offset {
				pieces = append(pieces, m.piece(t, near.Source, near.Offset == 0))
			}
		}
	}
	if tail != "" {
		pieces = append(pieces, m.piece(t, tail, tailAt == 0))
	}
	return pieces
}

// Markers planted in the source: QP<n>Q in every heading (tab panel
// labels), QD<n>Q in every DETAILS title, QT<n>Q before every tree row.
var (
	headingRe = regexp.MustCompile(`^((?:> ?)*#{1,6} )(.*)$`)
	treeOpen  = regexp.MustCompile("^((?:> ?)*)```tree\\s*$")
	treeLead  = regexp.MustCompile("^([ \t│|├└─+`\\-]*)(.*)$")
	markerRe  = regexp.MustCompile(`Q[PDT]\d+Q`)
)

func plant(doc string) string {
	n := 0
	next := func(k string) string { n++; return "Q" + k + strconv.Itoa(n) + "Q" }
	lines := strings.Split(doc, "\n")
	treeQ, inTree := "", false
	for i, l := range lines {
		if inTree {
			body := strings.TrimPrefix(l, treeQ)
			if strings.HasPrefix(strings.TrimSpace(body), "```") || !strings.HasPrefix(l, strings.TrimRight(treeQ, " ")) {
				inTree = false
				continue
			}
			if strings.TrimSpace(body) == "" {
				continue
			}
			sm := treeLead.FindStringSubmatch(body)
			lines[i] = treeQ + sm[1] + next("T") + sm[2]
			continue
		}
		if sm := treeOpen.FindStringSubmatch(l); sm != nil {
			treeQ, inTree = sm[1], true
			continue
		}
		if strings.Contains(l, "[!DETAILS]") {
			lines[i] = strings.Replace(l, "[!DETAILS]", "[!DETAILS] "+next("D"), 1)
			continue
		}
		if sm := headingRe.FindStringSubmatch(l); sm != nil {
			lines[i] = sm[1] + next("P") + " " + sm[2]
		}
	}
	return strings.Join(lines, "\n")
}

type lmStats struct {
	frames, textEq, byteEq, spanLines, spanBad               int
	marks, checked, ok, bad, partial, truncated, rev, revBad int
	unmapped, seams                                          int
	mapNs, frameNs                                           time.Duration
	byKind                                                   map[string]int
	fails                                                    []string
}

// check compares the mirror's pieces and marks with out, Stream's output.
func (st *lmStats) check(name, p string, pieces []mpiece, mp mapped, out string, final bool) {
	st.frames++
	plain := strings.Split(xansi.Strip(out), "\n")
	if xansi.Strip(mp.out) == xansi.Strip(out) {
		st.textEq++
	} else if len(st.fails) < 10 {
		st.fails = append(st.fails, name+": mirror text != Stream text")
	}
	if mp.out == out {
		st.byteEq++
	}
	// Each piece's complete lines are where lineMap says.
	texts := make([]string, len(pieces))
	for i, x := range pieces {
		texts[i] = x.text
	}
	sps := lineMap(p, texts)
	openBefore := false
	for i, sp := range sps {
		pl := strings.Split(xansi.Strip(texts[i]), "\n")
		for j := 0; j < sp.end-sp.start; j++ {
			if (j == 0 && openBefore) || (j == sp.end-sp.start-1 && !strings.HasSuffix(texts[i], "\n")) {
				continue // shared line
			}
			st.spanLines++
			if sp.start+j >= len(plain) || plain[sp.start+j] != pl[j] {
				st.spanBad++
				if len(st.fails) < 10 {
					st.fails = append(st.fails, fmt.Sprintf("%s: piece %d line %d differs", name, i, j))
				}
			}
		}
		if openBefore && texts[i] != "" {
			st.seams++
		}
		if texts[i] != "" {
			openBefore = !strings.HasSuffix(texts[i], "\n")
		}
	}
	at := map[int]map[string]bool{}
	for _, m := range mp.marks {
		st.marks++
		st.byKind[m.kind]++
		mk := markerRe.FindString(m.name)
		if mk == "" {
			st.partial++ // marker not yet streamed whole
			continue
		}
		if at[m.line] == nil {
			at[m.line] = map[string]bool{}
		}
		at[m.line][mk] = true
		needle := mk
		switch m.kind {
		case "panel":
			needle = "▸ " + mk
		case "details":
			needle = "▾ " + mk
		}
		got := -1
		for i, l := range plain {
			if strings.Contains(l, needle) && !(m.kind == "tabs" && strings.Contains(l, "▸ "+mk)) {
				got = i
				break
			}
		}
		st.checked++
		switch {
		case got == m.line:
			st.ok++
		case got < 0 && m.line < len(plain) && strings.Contains(plain[m.line], "…"):
			st.truncated++
		default:
			st.bad++
			if len(st.fails) < 10 {
				st.fails = append(st.fails, fmt.Sprintf("%s: %s %q at line %d, marker at %d", name, m.kind, mk, m.line, got))
			}
		}
	}
	// Reverse: every drawn header in the output is a mark at its line.
	for i, l := range plain {
		for _, mk := range markerRe.FindAllString(l, -1) {
			drawn := (mk[1] == 'P' && strings.Contains(l, "▸ "+mk)) || (mk[1] == 'D' && strings.Contains(l, "▾ "+mk)) || (mk[1] == 'T' && strings.ContainsAny(l, "├└") || mk[1] == 'T' && final)
			if !drawn {
				continue
			}
			st.rev++
			if !at[i][mk] {
				st.revBad++
				if len(st.fails) < 10 {
					st.fails = append(st.fails, fmt.Sprintf("%s: drawn %s at line %d has no mark (final=%v)", name, mk, i, final))
				}
			}
		}
	}
}

func TestLineMapSpike(t *testing.T) {
	type doc struct{ name, text string }
	var docs []doc
	es, err := os.ReadDir("../../testdata/directives")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		b, err := os.ReadFile("../../testdata/directives/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, doc{"dir-" + e.Name(), plant(string(b))})
	}
	for _, d := range corpus.Docs() {
		if strings.Contains(d.Text, "[!") || strings.Contains(d.Text, "```tree") {
			docs = append(docs, doc{"doc-" + d.Name, plant(d.Text)})
		}
	}
	for _, sc := range corpus.Scenarios() {
		if sc.Name == "forms-20k" {
			docs = append(docs, doc{"scenario-" + sc.Name, plant(sc.Text)})
		}
	}
	defer func() { t.Logf("inner open seams %d, marks on a shared line %d", innerSeams, seamMarks) }()
	for _, style := range []string{"dark", "light", "notty"} {
		for _, w := range []int{40, 78, 98} {
			st := &lmStats{byKind: map[string]int{}}
			sc := *styles.DefaultStyles[style]
			for _, d := range docs {
				r, err := bubbleteandmd.NewRenderer(sc, w)
				if err != nil {
					t.Fatal(err)
				}
				s := bubbleteandmd.NewStream(r, "k", bubbleteandmd.NewCache(256))
				m := newMirror(t, sc, w)
				prefixes := corpus.FramePrefixes(d.text)
				for i, src := range prefixes {
					final := i == len(prefixes)-1
					t0 := time.Now()
					var out string
					if final {
						out, err = s.Final(src)
					} else {
						out, err = s.Frame(src)
					}
					if err != nil {
						t.Fatal(err)
					}
					st.frameNs += time.Since(t0)
					pieces := m.frame(t, src, !final)
					texts := make([]string, len(pieces))
					for j, x := range pieces {
						texts[j] = x.text
					}
					t1 := time.Now()
					var marks []mark
					for j, sp := range lineMap(m.outer.p, texts) {
						marks = append(marks, shift(pieces[j].marks, sp.start)...)
					}
					st.mapNs += time.Since(t1)
					mp := mapped{out: join.Join(m.outer.p, m.outer.s, texts), marks: marks}
					st.check(fmt.Sprintf("%s/%s/w%d/%d", d.name, style, w, i), m.outer.p, pieces, mp, out, final)
				}
				st.unmapped += m.unmapped
			}
			t.Logf("%s w%d: frames %d text= %d byte= %d | span lines %d bad %d | marks %d %v checked %d ok %d bad %d trunc %d partial %d | reverse %d bad %d | unmapped nested %d | open seams top %d | map %.1fµs/frame vs Frame %.0fµs/frame",
				style, w, st.frames, st.textEq, st.byteEq, st.spanLines, st.spanBad, st.marks, st.byKind, st.checked, st.ok, st.bad, st.truncated, st.partial, st.rev, st.revBad, st.unmapped, st.seams,
				float64(st.mapNs.Microseconds())/float64(st.frames), float64(st.frameNs.Microseconds())/float64(st.frames))
			for _, f := range st.fails {
				t.Error(f)
			}
		}
	}
}
