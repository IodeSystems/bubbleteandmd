package directive

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/iodesystems/bubbleteandmd/internal/blocks"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
)

func docIDs(doc string) []FormID {
	var out []FormID
	for _, b := range blocks.Split(doc) {
		out = append(out, IDs(b.Source, b.Offset)...)
	}
	return out
}

type namedDoc struct{ name, text string }

func idDocs(t *testing.T) []namedDoc {
	var docs []namedDoc
	for _, d := range corpus.Docs() {
		docs = append(docs, namedDoc{"corpus/" + d.Name, d.Text})
	}
	for _, s := range corpus.Scenarios() {
		docs = append(docs, namedDoc{"scenario/" + s.Name, s.Text})
	}
	for i, f := range corpus.Fuzz(1, 100) {
		docs = append(docs, namedDoc{"fuzz/" + strconv.Itoa(i), f})
	}
	files, err := filepath.Glob("../../testdata/directives/*.md")
	if err != nil || len(files) == 0 {
		t.Fatalf("testdata/directives: %v, %d files", err, len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, namedDoc{f, string(b)})
	}
	return docs
}

// TestIDsStable: at every frame prefix, a form ID seen before has the same
// Kind. A form the splitter reopens and merges (PROS then CONS) keeps its
// offset, so it stays stable; any change is reported.
func TestIDsStable(t *testing.T) {
	changed, seen := 0, 0
	for _, d := range idDocs(t) {
		if testing.Short() && len(d.text) > 30_000 {
			continue
		}
		kinds := map[string]Kind{}
		for _, p := range corpus.FramePrefixes(d.text) {
			for _, id := range docIDs(p) {
				if old, ok := kinds[id.Key]; ok && old != id.Kind {
					changed++
					if changed <= 10 {
						t.Errorf("%s: key %s changed %v -> %v at prefix len %d", d.name, id.Key, old, id.Kind, len(p))
					}
				}
				kinds[id.Key] = id.Kind
				seen++
			}
		}
	}
	t.Logf("%d ids seen, %d kind changes", seen, changed)
}

// TestIDsUnique: no two forms of a final document share a Key.
func TestIDsUnique(t *testing.T) {
	n, dup := 0, 0
	for _, d := range idDocs(t) {
		keys := map[string]bool{}
		for _, id := range docIDs(d.text) {
			n++
			if keys[id.Key] {
				dup++
				t.Errorf("%s: duplicate key %s (%v)", d.name, id.Key, id.Kind)
			}
			keys[id.Key] = true
		}
	}
	t.Logf("%d ids, %d collisions", n, dup)
}

func find(ids []FormID, kind Kind) []FormID {
	var out []FormID
	for _, id := range ids {
		if id.Kind == kind {
			out = append(out, id)
		}
	}
	return out
}

// TestIDsNested: bodies are read, each under its parent.
func TestIDsNested(t *testing.T) {
	doc := "> [!TABS]\n#### A\n> [!PROS]\n- one\n\n> [!CONS]\n- two\n\n#### B\n> [!PROS]\n- three\n\n" +
		"---\n\n> [!DETAILS] Log\n> text\n>\n> > [!WARNING]\n> > careful\n\n" +
		"> [!NOTE]\n> a tree:\n>\n> ```tree\n> src/\n> ├── a.go\n> └── b/\n>     └── c.go\n> ```\n"
	ids := docIDs(doc)
	tabs := find(ids, KindTabs)
	if len(tabs) != 1 || tabs[0].Panels != 2 || tabs[0].Parent != "" || tabs[0].Offset != 0 {
		t.Fatalf("tabs = %+v", tabs)
	}
	verdicts := find(ids, KindVerdict)
	if len(verdicts) != 2 {
		t.Fatalf("verdicts = %+v", verdicts)
	}
	for _, v := range verdicts {
		if !strings.HasPrefix(v.Parent, tabs[0].Key) && v.Parent != "" {
			t.Errorf("verdict parent %q not under tabs %q", v.Parent, tabs[0].Key)
		}
	}
	if len(find(ids, KindCallout)) != 2 {
		t.Fatalf("callouts = %+v", find(ids, KindCallout))
	}
	details := find(ids, KindDetails)
	if len(details) != 1 || !details[0].Interactive() {
		t.Fatalf("details = %+v", details)
	}
	var inDetails *FormID
	for _, c := range find(ids, KindCallout) {
		if c.Parent == details[0].Key+"/b" {
			inDetails = &c
		}
	}
	if inDetails == nil {
		t.Errorf("no callout under details: %+v", ids)
	}
	trees := find(ids, KindTree)
	if len(trees) != 1 || trees[0].Parent == "" {
		t.Fatalf("trees = %+v", trees)
	}
	if want := []string{"src/#0", "src/b/#2"}; !slices.Equal(trees[0].Paths, want) {
		t.Errorf("paths = %v, want %v", trees[0].Paths, want)
	}
}

// TestIDsTopOffsets: a form's Offset is its absolute byte offset.
func TestIDsTopOffsets(t *testing.T) {
	doc := "intro\n\n> [!NOTE]\n> a\n\ntext\n\n> [!TIP]\n> b\n"
	ids := docIDs(doc)
	if len(ids) != 2 {
		t.Fatalf("ids = %+v", ids)
	}
	for _, id := range ids {
		if !strings.HasPrefix(doc[id.Offset:], "> [!") {
			t.Errorf("offset %d is not a marker: %q", id.Offset, doc[id.Offset:])
		}
	}
}
