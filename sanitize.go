package bubbleteandmd

import (
	"strings"
	"unicode/utf8"
)

// Sanitize removes from src what a terminal reads as a command, not text,
// so model output cannot write the clipboard (OSC 52), set the title, move
// the cursor, clear the screen or overwrite a line. glamour v2.0.1 passes
// these through unchanged, in prose, tables and code.
//
// Removed:
//   - C0 controls (U+0000–U+001F) except tab and newline; "\r\n" becomes
//     "\n" and any other "\r" goes too. ESC goes, so the text of an escape
//     sequence after it stays as plain characters.
//   - DEL (U+007F) and C1 controls (U+0080–U+009F), among them CSI U+009B.
//   - bytes that are not UTF-8 (a raw 0x9B byte is one).
//   - an incomplete UTF-8 sequence at the end of src: it can complete in the
//     next chunk, so it is held back, not shown as an error.
//   - Unicode bidi embeddings, overrides and isolates (U+202A–U+202E,
//     U+2066–U+2069): they reorder the text shown, so a line can read
//     differently from its bytes (spoofing).
//
// Everything else stays, ZWJ and emoji included.
//
// Sanitize is prefix-stable: for any cut k, Sanitize(s[:k]) is a prefix of
// Sanitize(s), so a stream that grows keeps its rendered blocks. It is
// idempotent. Stream.Frame and Stream.Final call it; a caller that renders
// model text some other way must call it first.
func Sanitize(src string) string {
	var b strings.Builder
	clean := 0 // src[:clean] needs no change and is not yet in b
	dirty := false
	for i := 0; i < len(src); {
		r, n := rune(src[i]), 1
		if r >= utf8.RuneSelf {
			r, n = utf8.DecodeRuneInString(src[i:])
		}
		if keep(r, n) {
			i += n
			continue
		}
		if !dirty {
			b.Grow(len(src))
			dirty = true
		}
		b.WriteString(src[clean:i])
		if r == utf8.RuneError && n == 1 && !utf8.FullRuneInString(src[i:]) {
			// An incomplete sequence at the end: hold it back.
			return b.String()
		}
		i += n
		clean = i
	}
	if !dirty {
		return src
	}
	b.WriteString(src[clean:])
	return b.String()
}

// keep reports whether rune r, decoded from n bytes, stays.
func keep(r rune, n int) bool {
	switch {
	case r == '\t' || r == '\n':
		return true
	case r < 0x20 || r == 0x7f:
		return false
	case r < 0x80:
		return true
	case r == utf8.RuneError && n == 1:
		return false // not UTF-8, or incomplete at the end
	case r <= 0x9f:
		return false // C1
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return false // bidi
	}
	return true
}
