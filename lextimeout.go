package bubbleteandmd

import "github.com/iodesystems/bubbleteandmd/internal/lextimeout"

// DisableLexTimeout stops regexp2's match timeouts in the whole process, so
// chroma never draws valid code with an Error token because of a timeout.
//
// Why: chroma (v2.27.0) gives every lexer rule a 250 ms regexp2
// MatchTimeout on a wall clock and reads a timeout as no match, so the next
// character gets the Error style (a red background in most styles). A
// goroutine that is off the CPU for 250 ms in the middle of a match (a
// loaded machine, many goroutines) times out on code that is valid.
//
// Trade-off: the setting is process-wide (regexp2 has no per-call
// setting). It removes chroma's guard, and the guard of every other
// regexp2 user in the process, against a runaway (catastrophic
// backtracking) match: such a match then runs until it ends.
//
// Opt-in: the library never calls it. Call it once at program start,
// before any rendering or other regexp2 use (it writes a regexp2 setting
// without a lock).
func DisableLexTimeout() { lextimeout.Disable() }
