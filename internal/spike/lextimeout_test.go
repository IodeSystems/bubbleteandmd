package spike_test

import (
	"os"
	"testing"

	"github.com/iodesystems/bubbleteandmd"
)

// TestMain stops regexp2's match timeouts: these tests lex code through
// chroma, whose 250 ms per-rule timeout writes an Error token on valid code
// under load (root lextimeout_test.go).
func TestMain(m *testing.M) {
	bubbleteandmd.DisableLexTimeout()
	os.Exit(m.Run())
}
