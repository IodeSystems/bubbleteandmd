package diffview

import (
	"reflect"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	for line, want := range map[string]Kind{
		"":                            Context,
		" x := 1":                     Context,
		"plain":                       Context,
		"+x":                          Added,
		"+":                           Added,
		"!x":                          Added,
		"-x":                          Removed,
		"-":                           Removed,
		"--":                          Removed,
		"++":                          Added,
		"---":                         Header,
		"--- a/f.go":                  Header,
		"+++ b/f.go":                  Header,
		"@@ -1,3 +1,3 @@ func":        Hunk,
		"@":                           Context,
		"diff --git a/f b/f":          Header,
		"index 1234..5678 100644":     Header,
		"Index: f.go":                 Header,
		"===================":         Header,
		`\ No newline at end of file`: Header,
		"*** 1,2 ****":                Header,
	} {
		if got := Classify(line); got != want {
			t.Errorf("Classify(%q) = %d, want %d", line, got, want)
		}
	}
}

func kinds(s string) []Kind {
	var out []Kind
	for _, c := range s {
		switch c {
		case '-':
			out = append(out, Removed)
		case '+':
			out = append(out, Added)
		case '@':
			out = append(out, Hunk)
		default:
			out = append(out, Context)
		}
	}
	return out
}

func TestPairs(t *testing.T) {
	for in, want := range map[string][]int{
		"-+":     {1, 0},
		"--++":   {2, 3, 0, 1},
		"--+":    {2, -1, 0},
		"-++":    {1, 0, -1},
		"+-":     {-1, -1},
		"- +":    {-1, -1, -1},
		"-@+":    {-1, -1, -1},
		" -+ -+": {-1, 2, 1, -1, 5, 4},
		"---":    {-1, -1, -1},
		"":       {},
	} {
		if got := Pairs(kinds(in)); !reflect.DeepEqual(got, want) {
			t.Errorf("Pairs(%q) = %v, want %v", in, got, want)
		}
	}
	long := strings.Repeat("-", MaxRun+1) + "+"
	for i, p := range Pairs(kinds(long)) {
		if p != -1 {
			t.Errorf("run over MaxRun: line %d pairs with %d", i, p)
		}
	}
	ok := strings.Repeat("-", MaxRun) + strings.Repeat("+", MaxRun)
	if p := Pairs(kinds(ok)); p[0] != MaxRun || p[MaxRun-1] != 2*MaxRun-1 {
		t.Errorf("run of MaxRun does not pair: %v", p[:3])
	}
}

func spans(s string, rs []Range) []string {
	var out []string
	for _, r := range rs {
		out = append(out, s[r.From:r.To])
	}
	return out
}

func TestWords(t *testing.T) {
	for _, c := range []struct {
		a, b   string
		da, db []string
	}{
		{"  typecheck: true,", "  typecheck: false,", []string{"true"}, []string{"false"}},
		{"x := f(a, b)", "x := f(a, b, c)", nil, []string{", c"}},
		{"return nil", "return err", []string{"nil"}, []string{"err"}},
		{"same", "same", nil, nil},
		{"héllo wörld", "héllo welt", []string{"wörld"}, []string{"welt"}},
		// Nothing shared but spaces: no marks.
		{"alpha beta", "gamma delta", nil, nil},
		{"", "x", nil, nil},
	} {
		ra, rb := Words(c.a, c.b)
		if got := spans(c.a, ra); !reflect.DeepEqual(got, c.da) {
			t.Errorf("Words(%q, %q) a = %q, want %q", c.a, c.b, got, c.da)
		}
		if got := spans(c.b, rb); !reflect.DeepEqual(got, c.db) {
			t.Errorf("Words(%q, %q) b = %q, want %q", c.a, c.b, got, c.db)
		}
	}
	long := strings.Repeat("a ", MaxRunes/2+1)
	if ra, rb := Words(long, long+"b"); ra != nil || rb != nil {
		t.Errorf("line over MaxRunes got marks")
	}
}

func BenchmarkWords(b *testing.B) {
	x := strings.Repeat("value := compute(alpha, beta) + 1; ", 13)
	y := strings.Repeat("value := compute(alpha, gamma) + 2; ", 13)
	for i := 0; i < b.N; i++ {
		Words(x, y)
	}
}
