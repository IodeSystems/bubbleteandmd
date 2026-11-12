// Command demo streams one Markdown reply into two panes side by side:
// bubbleteandmd on the left, glamour rendering the whole reply per token on
// the right, with render times per pane. Not part of the library API.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2/styles"
	"github.com/iodesystems/bubbleteandmd"
)

//go:embed showcase.md
var showcase string

func main() {
	style := flag.String("style", "dark", "dark, light or notty")
	width := flag.Int("width", 0, "width of each pane (0: half the terminal)")
	rate := flag.Float64("rate", 60, "tokens per second")
	file := flag.String("file", "", "stream this Markdown file in place of the showcase")
	wide := flag.Bool("wide", true, "left pane: cut wide code blocks and tables at the pane (WideBlocks)")
	seed := flag.Uint("seed", 42, "token size seed")
	flag.Parse()

	cfg := config{styleName: *style, width: *width, rate: max(*rate, 1), wide: *wide, src: showcase, seed: uint32(*seed)}
	switch *style {
	case "dark":
		cfg.style = styles.DarkStyleConfig
	case "light":
		cfg.style = styles.LightStyleConfig
	case "notty":
		cfg.style = styles.NoTTYStyleConfig
	default:
		fail(fmt.Errorf("--style %q: want dark, light or notty", *style))
	}
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fail(err)
		}
		cfg.src = string(b)
	}

	// chroma gives each lexer rule a 250 ms wall-clock match timeout. The
	// right pane renders the whole reply per token and keeps the CPU busy;
	// on a loaded machine a match on valid code then times out and one
	// character draws in the Error style (red). The setting is for the
	// whole process; this program is the only user of regexp2 in it.
	bubbleteandmd.DisableLexTimeout()

	if _, err := tea.NewProgram(newModel(cfg)).Run(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "demo:", err)
	os.Exit(1)
}
