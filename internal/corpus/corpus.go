// Ported from llx-markdown packages/llx-markdown/test/corpus.ts (MIT, see NOTICE).

package corpus

import (
	"embed"
	"sort"
	"strings"
)

//go:embed testdata/*.md
var files embed.FS

// Doc is one static corpus document. Name is the file name without .md; the TS keys
// "example: <form>" are "example-<form>" here.
type Doc struct{ Name, Text string }

// Docs returns the static corpus (the TS CORPUS), sorted by name. Text is byte-exact.
func Docs() []Doc {
	entries, err := files.ReadDir("testdata")
	if err != nil {
		panic(err)
	}
	var docs []Doc
	for _, e := range entries {
		b, err := files.ReadFile("testdata/" + e.Name())
		if err != nil {
			panic(err)
		}
		docs = append(docs, Doc{strings.TrimSuffix(e.Name(), ".md"), string(b)})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Name < docs[j].Name })
	return docs
}
