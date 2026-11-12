// Package lextimeout stops regexp2's match timeouts in the process. The
// root package exports it as DisableLexTimeout; the test binaries of
// internal packages that lex code through chroma call it in TestMain (they
// cannot import the root package, which imports them).
package lextimeout

import (
	"time"

	"github.com/dlclark/regexp2/v2"
)

// Disable sets regexp2's timeout check period to 100 years.
//
// regexp2 reads the time from a clock goroutine that sleeps the check
// period between reads. With a period of 100 years the clock does not move
// after it starts, so no MatchTimeout deadline is reached: chroma's 250 ms
// per-rule timeout (v2.27.0 regexp.go:375) does not fire, also when a
// goroutine is off the CPU under load. Process-wide: it applies to every
// regexp2 user and removes their guard against runaway matches.
func Disable() {
	regexp2.SetTimeoutCheckPeriod(100 * 365 * 24 * time.Hour)
}
