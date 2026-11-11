// Ported from llx-markdown packages/llx-markdown/src/core/blocks-fuzz.test.tsx (MIT, see NOTICE).

package corpus

import "strings"

// FuzzDoc builds the document for seed: 2 to 14 lines drawn from the lines where one parse
// reads across blocks, joined by "\n".
func FuzzDoc(seed uint32) string {
	r := Seeded(seed)
	n := r.Int(2, 14)
	lines := make([]string, 0, n)
	for ; n > 0; n-- {
		lines = append(lines, fuzzLines[r.Int(0, len(fuzzLines)-1)])
	}
	return strings.Join(lines, "\n")
}

// Fuzz returns n documents for seeds from, from+1, ... (the TS test uses from = 1).
func Fuzz(from uint32, n int) []string {
	docs := make([]string, n)
	for i := range docs {
		docs[i] = FuzzDoc(from + uint32(i))
	}
	return docs
}
