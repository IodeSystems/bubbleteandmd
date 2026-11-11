// Ported from llx-markdown packages/llx-markdown/test/chunks.ts (MIT, see NOTICE).

package corpus

import (
	"strings"
	"unicode/utf16"
)

// Pattern is how a host hands the renderer a reply that is still arriving.
type Pattern string

const (
	// Token: 1 to 12 characters at a time, as an LLM API streams.
	Token Pattern = "token"
	// Frame: 20 characters per update.
	Frame Pattern = "frame"
	// Line: a flush on every newline.
	Line Pattern = "line"
)

// Prefixes returns the growing prefixes of text for pattern, ending with text itself. Chunk sizes
// count UTF-16 code units, as the TS does; seed is used by Token only (TS default: 42).
func Prefixes(text string, pattern Pattern, seed uint32) []string {
	var out []string
	if pattern == Line {
		for at := strings.IndexByte(text, '\n'); at != -1; {
			out = append(out, text[:at+1])
			next := strings.IndexByte(text[at+1:], '\n')
			if next == -1 {
				break
			}
			at += 1 + next
		}
	} else {
		units := utf16.Encode([]rune(text))
		r := Seeded(seed)
		last := 0
		for end := 0; end < len(units); {
			step := 20
			if pattern != Frame {
				step = r.Int(1, 7)
				if r.Next() < 0.1 {
					step += 5
				}
			}
			end = min(len(units), end+step)
			// Never cut inside a surrogate pair: move back to the rune boundary.
			cut := end
			if cut < len(units) && utf16.IsSurrogate(rune(units[cut])) && units[cut] >= 0xdc00 {
				cut--
			}
			if cut == last {
				continue
			}
			last = cut
			out = append(out, string(utf16.Decode(units[:cut])))
		}
	}
	if len(out) == 0 || out[len(out)-1] != text {
		out = append(out, text)
	}
	return out
}

// TokenPrefixes is Prefixes(s, Token, 42).
func TokenPrefixes(s string) []string { return Prefixes(s, Token, 42) }

// FramePrefixes is Prefixes(s, Frame, 0).
func FramePrefixes(s string) []string { return Prefixes(s, Frame, 0) }

// LinePrefixes is Prefixes(s, Line, 0).
func LinePrefixes(s string) []string { return Prefixes(s, Line, 0) }
