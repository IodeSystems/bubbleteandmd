package codestream

import (
	"os"
	"testing"

	"github.com/iodesystems/bubbleteandmd/internal/lextimeout"
)

// TestMain stops regexp2's match timeouts: these tests lex code through
// chroma, whose 250 ms per-rule timeout writes an Error token on valid code
// under load (root lextimeout_test.go).
func TestMain(m *testing.M) {
	lextimeout.Disable()
	os.Exit(m.Run())
}
