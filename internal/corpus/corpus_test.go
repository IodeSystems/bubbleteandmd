// Ported from llx-markdown packages/llx-markdown/test (MIT, see NOTICE).

package corpus

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// tsCorpusEntries is the number of keys in the TS CORPUS: 6 ANSWER_FORM_EXAMPLES + 8 literals.
const tsCorpusEntries = 14

// goOnlyDocs are fixtures with no TS counterpart: emoji.md.
const goOnlyDocs = 1

// tsScenarios is the number of scenarios() entries in the TS replies.ts.
const tsScenarios = 10

func TestMulberry32KnownVectors(t *testing.T) {
	// Computed by tracing the TS algorithm with 32-bit wraparound (seed 1 matches the published
	// mulberry32 first output 0.6270739405881613).
	want := map[uint32][]float64{
		0:  {0.26642920868471265, 0.0003297457005828619, 0.2232720274478197},
		1:  {0.6270739405881613, 0.002735721180215478, 0.5274470399599522},
		42: {0.6011037519201636, 0.44829055899754167, 0.8524657934904099},
	}
	for seed, vals := range want {
		r := Seeded(seed)
		for i, v := range vals {
			if got := r.Next(); got != v {
				t.Errorf("seed %d value %d: got %v want %v", seed, i, got, v)
			}
		}
	}
}

func TestHashVector(t *testing.T) {
	if got := Hash("hello"); got != "4f9f2cab" {
		t.Errorf("Hash(hello) = %s", got)
	}
}

func TestDocsCount(t *testing.T) {
	docs := Docs()
	t.Logf("fixtures=%d ts=%d", len(docs), tsCorpusEntries)
	if len(docs) != tsCorpusEntries+goOnlyDocs {
		t.Fatalf("got %d docs, want TS CORPUS %d + %d Go-only", len(docs), tsCorpusEntries, goOnlyDocs)
	}
	for _, d := range docs {
		if d.Text == "" || strings.HasSuffix(d.Text, "\n") {
			t.Errorf("%s: empty or trailing newline (TS entries have none)", d.Name)
		}
	}
}

func TestScenariosDeterministic(t *testing.T) {
	a, b := Scenarios(), Scenarios()
	if len(a) != tsScenarios {
		t.Fatalf("got %d scenarios, TS has %d", len(a), tsScenarios)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Scenarios not deterministic")
	}
	for _, s := range a {
		if len(s.Text) < 1000 {
			t.Errorf("%s: only %d chars", s.Name, len(s.Text))
		}
	}
	if n := len(a[9].History); n != 30 {
		t.Errorf("thread-30 history = %d", n)
	}
	if !strings.Contains(a[7].Text, "\r\n") || strings.Contains(strings.ReplaceAll(a[7].Text, "\r\n", ""), "\n") {
		t.Error("crlf-20k line endings wrong")
	}
	sc := Scaling()
	if len(sc[1].Text) < 3*len(sc[0].Text) {
		t.Error("scaling pair not ~4x")
	}
}

func TestFuzzDeterministic(t *testing.T) {
	a, b := Fuzz(1, 200), Fuzz(1, 200)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Fuzz not deterministic")
	}
	if a[4] != FuzzDoc(5) {
		t.Error("Fuzz(1,n)[i] != FuzzDoc(i+1)")
	}
	distinct := map[string]bool{}
	for _, d := range a {
		distinct[d] = true
		if n := strings.Count(d, "\n") + 1; n < 2 || n > 14 {
			t.Errorf("doc has %d lines", n)
		}
	}
	if len(distinct) < 100 {
		t.Errorf("only %d distinct docs", len(distinct))
	}
}

func TestPrefixes(t *testing.T) {
	for _, d := range Docs() {
		for _, p := range []Pattern{Token, Frame, Line} {
			got := Prefixes(d.Text, p, 42)
			if got[len(got)-1] != d.Text {
				t.Fatalf("%s/%s: last prefix is not the text", d.Name, p)
			}
			for i := 1; i < len(got); i++ {
				if !strings.HasPrefix(got[i], got[i-1]) || len(got[i]) <= len(got[i-1]) {
					t.Fatalf("%s/%s: prefix %d does not grow", d.Name, p, i)
				}
			}
		}
	}
	if got := Prefixes("", Token, 42); len(got) != 1 || got[0] != "" {
		t.Errorf("empty text: %q", got)
	}
	fr := FramePrefixes(strings.Repeat("x", 45))
	if len(fr) != 3 || len(fr[0]) != 20 || len(fr[1]) != 40 {
		t.Errorf("frame sizes: %d", len(fr))
	}
	if got := LinePrefixes("a\nb\nc"); !reflect.DeepEqual(got, []string{"a\n", "a\nb\n", "a\nb\nc"}) {
		t.Errorf("line: %q", got)
	}
	if !reflect.DeepEqual(TokenPrefixes(Docs()[0].Text), TokenPrefixes(Docs()[0].Text)) {
		t.Error("token prefixes not deterministic")
	}
}

func TestPrefixesNeverSplitCharacters(t *testing.T) {
	text := "Hi \U0001F600 \u4f60\u597d\U0001F680\U0001F680 end \U0001F468\u200d\U0001F469 \u00e9"
	texts := []string{text, strings.Repeat(text, 20)}
	for _, d := range Docs() {
		texts = append(texts, d.Text)
	}
	for _, s := range texts {
		for _, p := range []Pattern{Token, Frame, Line} {
			for seed := uint32(0); seed < 20; seed++ {
				got := Prefixes(s, p, seed)
				if got[len(got)-1] != s {
					t.Fatalf("%s: last prefix is not the text", p)
				}
				for i, pre := range got {
					if !utf8.ValidString(pre) || !strings.HasPrefix(s, pre) {
						t.Fatalf("%s seed %d prefix %d invalid: %q", p, seed, i, pre)
					}
					if i > 0 && len(pre) <= len(got[i-1]) {
						t.Fatalf("%s seed %d prefix %d does not grow", p, seed, i)
					}
				}
			}
		}
	}
}
