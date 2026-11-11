package bubbleteandmd

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/iodesystems/bubbleteandmd/internal/corpus"
	"github.com/iodesystems/bubbleteandmd/internal/directive"
)

// payloads are terminal control sequences a model reply can carry.
var payloads = []struct{ name, text string }{
	{"osc52", "\x1b]52;c;ZWNobw==\x07"},
	{"osc0", "\x1b]0;title\x07"},
	{"osc2", "\x1b]2;title\x1b\\"},
	{"osc8", "\x1b]8;;http://evil.example/\x07click\x1b]8;;\x07"},
	{"csi2J", "\x1b[2J"},
	{"cursor", "\x1b[H\x1b[3A\x1b[10;20H"},
	{"sgr", "\x1b[31mred\x1b[0m"},
	{"c1csi", "\u009b2J"},
	{"c1raw", "\x9b2J"},
	{"dcs", "\x1bPq#0;2;0;0;0\x1b\\"},
	{"apc", "\x1b_Gf=24;AAAA\x1b\\"},
	{"cr", "safe\rEVIL"},
	{"bs", "abc\b\b\bXYZ"},
	{"bel", "ding\x07"},
	{"bidi", "abc‮dcba⁦x⁩"},
	{"nul", "a\x00b"},
	{"del", "a\x7fb"},
}

// contexts place X (the payload) in each Markdown construct.
var contexts = []struct{ name, text string }{
	{"prose", "Hello X world.\n"},
	{"heading", "# Head X\n"},
	{"list", "- item X\n- two\n"},
	{"table", "| a | b |\n|---|---|\n| X | c |\n"},
	{"quote", "> quote X\n"},
	{"inline-code", "Run `cmd X` now.\n"},
	{"fence", "```\ncode X\n```\n"},
	{"fence-lang", "```go\nfmt.Println(\"X\")\n```\n"},
	{"link-text", "[text X](https://example.com)\n"},
	{"link-url", "[text](https://example.com/X)\n"},
	{"autolink", "<https://example.com/X>\n"},
	{"image-alt", "![alt X](https://example.com/i.png)\n"},
	{"html", "<div>\nX\n</div>\n"},
}

type payloadDoc struct{ name, text string }

func payloadDocs() []payloadDoc {
	var ds []payloadDoc
	for _, c := range contexts {
		for _, p := range payloads {
			ds = append(ds, payloadDoc{c.name + "/" + p.name, "Intro.\n\n" + strings.ReplaceAll(c.text, "X", p.text) + "\nOutro.\n"})
		}
	}
	return ds
}

// foreign returns a description of the first byte sequence in out that a
// terminal reads as a command and that is not one glamour or lipgloss
// emits itself, or "" for none. Allowed: SGR (ESC [ params m) and OSC 8
// hyperlinks (ESC ] 8 ; params ; url BEL, or ST) whose body has no
// controls. glamour v2.0.1 emits OSC 8 for links, ended by BEL.
func foreign(out string) string {
	for i := 0; i < len(out); {
		c := out[i]
		if c == 0x1b {
			rest := out[i:]
			switch {
			case strings.HasPrefix(rest, "\x1b["):
				j := 2
				for j < len(rest) && (rest[j] >= '0' && rest[j] <= '9' || rest[j] == ';' || rest[j] == ':') {
					j++
				}
				if j >= len(rest) || rest[j] != 'm' {
					return fmt.Sprintf("non-SGR CSI at %d: %q", i, rest[:min(len(rest), j+1)])
				}
				i += j + 1
				continue
			case strings.HasPrefix(rest, "\x1b]8;"):
				end, n := strings.IndexByte(rest, 0x07), 1
				if st := strings.Index(rest, "\x1b\\"); st >= 0 && (end < 0 || st < end) {
					end, n = st, 2
				}
				if end < 0 {
					return fmt.Sprintf("unterminated OSC 8 at %d", i)
				}
				for _, b := range []byte(rest[4:end]) {
					if b < 0x20 || b == 0x7f || b >= 0x80 && b <= 0x9f {
						return fmt.Sprintf("control 0x%02x in OSC 8 at %d: %q", b, i, rest[:end+n])
					}
				}
				i += end + n
				continue
			}
			return fmt.Sprintf("ESC at %d: %q", i, rest[:min(len(rest), 8)])
		}
		r, n := utf8.DecodeRuneInString(out[i:])
		if r == utf8.RuneError && n == 1 {
			return fmt.Sprintf("invalid UTF-8 byte 0x%02x at %d", c, i)
		}
		if !keep(r, n) {
			return fmt.Sprintf("control U+%04X at %d", r, i)
		}
		i += n
	}
	return ""
}

// TestForeignCatchesRaw: the check is not vacuous. glamour, given the raw
// payloads, lets them through (the bug Sanitize fixes).
func TestForeignCatchesRaw(t *testing.T) {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(78))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{
		"hi \x1b]52;c;ZWNobw==\x07 there\n",
		"```\n\x1b[31mred \x1b]0;title\a\n```\n",
		"a \u009b2J b\n", "a \x9b2J b\n", "safe\rEVIL\n", "a\bb\a\n",
		"| a | b |\n|---|---|\n| \x1b]52;c;x\x07 | \r\b |\n",
		"a ‮ b\n",
	} {
		out, err := r.Render(d)
		if err != nil {
			t.Fatal(err)
		}
		if foreign(out) == "" {
			t.Errorf("raw %q: glamour output passes the check; the check is too weak", d)
		}
	}
}

func TestSanitize(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"plain\ttext\n", "plain\ttext\n"},
		{"a\r\nb\r\n", "a\nb\n"},
		{"safe\rEVIL", "safeEVIL"},
		{"\x1b]52;c;ZWNobw==\x07", "]52;c;ZWNobw=="},
		{"\x1b[31mred\x1b[0m", "[31mred[0m"},
		{"a\x00b\x7fc\bd", "abcd"},
		{"a\u009bb\u0085c", "abc"},
		{"a\x9bb", "ab"},
		{"a\xffb\xc0\xafc", "abc"},
		{"a\xed\xa0\x80b", "ab"}, // UTF-8 of a surrogate
		{"a‪b‮c⁦d⁩e", "abcde"},
		{"keep ‍ \U0001F469‍\U0001F4BB   � é", "keep ‍ \U0001F469‍\U0001F4BB   � é"},
		{"end \xe2\x82", "end "},     // incomplete: held back
		{"end \xf0\x9f\x98", "end "}, // incomplete: held back
		{"end \xe2\x82X", "end X"},   // invalid, not at the end
		{"end \xe2\x82\xac", "end €"},
	} {
		if got := Sanitize(c.in); got != c.want {
			t.Errorf("Sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// checkSanitize asserts prefix stability at every byte cut, idempotence and
// that the result is valid UTF-8 with nothing Sanitize removes.
func checkSanitize(t *testing.T, s string) {
	t.Helper()
	full := Sanitize(s)
	if !utf8.ValidString(full) {
		t.Fatalf("Sanitize(%q) = %q: not UTF-8", s, full)
	}
	if again := Sanitize(full); again != full {
		t.Fatalf("not idempotent on %q: %q then %q", s, full, again)
	}
	for _, r := range full {
		if !keep(r, utf8.RuneLen(r)) {
			t.Fatalf("Sanitize(%q) keeps U+%04X", s, r)
		}
	}
	for k := 0; k <= len(s); k++ {
		if p := Sanitize(s[:k]); !strings.HasPrefix(full, p) {
			t.Fatalf("cut %d of %q: Sanitize = %q, not a prefix of %q", k, s, p, full)
		}
	}
}

func sanitizeSeeds() []string {
	var ss []string
	for _, p := range payloads {
		ss = append(ss, p.text)
	}
	for _, d := range payloadDocs() {
		ss = append(ss, d.text)
	}
	for _, d := range corpus.Docs() {
		ss = append(ss, d.Text)
	}
	return append(ss, "\xe2\x82", "\xf0\x9f\x98\x80\xf0", "\r\n\r", "\xed\xa0\x80", "\xc0\xaf", "\U0001F469‍\U0001F4BB")
}

// TestSanitizePrefixStable runs the fuzz property on the seeds and on every
// corpus prefix the invariant streams.
func TestSanitizePrefixStable(t *testing.T) {
	for _, s := range sanitizeSeeds() {
		checkSanitize(t, s)
	}
	for _, s := range corpus.Fuzz(1, 50) {
		checkSanitize(t, s)
	}
}

// TestSanitizeCorpusUnchanged: the corpus has no controls but the "\r" of
// the CRLF scenario, so the invariant cases render as before Sanitize,
// that one with "\n" line ends.
func TestSanitizeCorpusUnchanged(t *testing.T) {
	changed := 0
	for _, c := range invCases() {
		got := Sanitize(c.text)
		if got != strings.ReplaceAll(c.text, "\r", "") {
			t.Errorf("%s: Sanitize removes more than \\r from a corpus case", c.name)
		}
		if got != c.text {
			changed++
			t.Logf("%s: \\r removed", c.name)
		}
	}
	if changed > 1 {
		t.Errorf("%d corpus cases have \\r; want only the CRLF scenario", changed)
	}
}

func FuzzSanitize(f *testing.F) {
	for _, s := range sanitizeSeeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { checkSanitize(t, s) })
}

// bytePrefixes returns s cut at every byte, or, when sparse, every 5th
// byte and after every newline. corpus prefixes count UTF-16 units and so
// change bytes that are not UTF-8; these do not.
func bytePrefixes(s string, sparse bool) []string {
	var ps []string
	for k := 1; k <= len(s); k++ {
		if !sparse || k%5 == 0 || s[k-1] == '\n' || k == len(s) {
			ps = append(ps, s[:k])
		}
	}
	return ps
}

// TestStreamNoForeignEscapes streams each payload document, at every byte
// prefix (every 5th byte and line ends in -short), through Frame and Final in
// three styles and checks no foreign escape reaches the output. It also
// checks the stream invariant on the payload documents.
func TestStreamNoForeignEscapes(t *testing.T) {
	var run, passed atomic.Int64
	t.Cleanup(func() { t.Logf("payload stream: %d prefix checks run, %d passed", run.Load(), passed.Load()) })
	for _, style := range []string{"dark", "light", "notty"} {
		for _, d := range payloadDocs() {
			t.Run(style+"/"+d.name, func(t *testing.T) {
				t.Parallel()
				r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(78))
				if err != nil {
					t.Fatal(err)
				}
				prefixes := bytePrefixes(d.text, testing.Short())
				s := NewStream(r, "k", NewCache(64))
				for _, p := range prefixes {
					for _, k := range []struct {
						name string
						f    func(string) (string, error)
					}{{"Frame", s.Frame}, {"Final", s.Final}} {
						run.Add(1)
						out, err := k.f(p)
						if err != nil {
							t.Fatalf("prefix len %d: %s: %v", len(p), k.name, err)
						}
						if bad := foreign(out); bad != "" {
							t.Fatalf("prefix len %d: %s: %s\nprefix: %q", len(p), k.name, bad, p)
						}
						passed.Add(1)
					}
				}
				var inv, ok atomic.Int64
				checkStream(t, r, d.text, bytePrefixes(d.text, true), &inv, &ok)
			})
		}
	}
}

// payloadDirectiveDocs place the payload in directive titles, labels and bodies.
var payloadDirectiveDocs = []struct{ name, text string }{
	{"callout", "> [!WARNING] Title X\n> body X\n"},
	{"details", "> [!DETAILS] More X\n> body X\n"},
	{"steps", "> [!STEPS] Plan X\n> 1. **Step X** *when X* body X\n> 2. **Two** done\n"},
	{"tabs", "> [!TABS] Pick X\n> #### Label X\n> body X\n> #### Two\n> more\n"},
	{"stats", "> [!STATS] Figures X\n> - **42X** label X\n> - **7** days\n"},
	{"verdict", "> [!PROS] Good X\n> - fast X\n\n> [!CONS] Bad X\n> - slow X\n"},
	{"sources", "> [!SOURCES] Refs X\n> - [title X](https://example.com/X) note X\n"},
	{"attribution", "> quote X\n> — Author X\n"},
	{"tree", "```tree\nsrc/X\n└── a.go  # note X\n```\n"},
	{"code-title", "```go title=\"a X.go\"\nx := \"X\"\n```\n"},
}

// TestDirectiveNoForeignEscapes: the lipgloss form renderers read the same
// text; after Sanitize they emit no foreign escape.
func TestDirectiveNoForeignEscapes(t *testing.T) {
	for _, style := range []string{"dark", "light", "notty"} {
		cfg := *styles.DefaultStyles[style]
		zero := uint(0)
		cfg.Document.Margin = &zero
		c := directive.Ctx{
			Width: 60,
			Theme: directive.ThemeFrom(cfg, style == "notty"),
			Inner: func(md string, w int) (string, error) {
				r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(w))
				if err != nil {
					return "", err
				}
				return r.Render(md)
			},
		}
		raw := 0 // raw payloads that get through: the check is not vacuous
		for _, dd := range payloadDirectiveDocs {
			for _, p := range payloads {
				if d, ok := directive.Parse(strings.ReplaceAll(dd.text, "X", p.text)); ok {
					if lines, err := directive.Render(d, c); err == nil && foreign(strings.Join(lines, "\n")) != "" {
						raw++
					}
				}
				src := Sanitize(strings.ReplaceAll(dd.text, "X", p.text))
				d, ok := directive.Parse(src)
				if !ok {
					t.Errorf("%s/%s/%s: no directive in %q", style, dd.name, p.name, src)
					continue
				}
				lines, err := directive.Render(d, c)
				if err != nil {
					t.Errorf("%s/%s/%s: %v", style, dd.name, p.name, err)
					continue
				}
				if bad := foreign(strings.Join(lines, "\n")); bad != "" {
					t.Errorf("%s/%s/%s: %s", style, dd.name, p.name, bad)
				}
			}
		}
		t.Logf("%s: %d of %d raw directive docs emit a foreign escape", style, raw, len(payloadDirectiveDocs)*len(payloads))
		if raw == 0 {
			t.Errorf("%s: no raw payload gets through the directive renderers; the check is too weak", style)
		}
	}
}
