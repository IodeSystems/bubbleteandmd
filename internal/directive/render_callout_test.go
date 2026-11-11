package directive

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
)

// calloutCtx builds a Ctx whose Inner renders through a real glamour
// TermRenderer with no margin and word wrap at the inner width.
func calloutCtx(t *testing.T, name string, width int) Ctx {
	cfg := *styles.DefaultStyles[name]
	zero := uint(0)
	cfg.Document.Margin = &zero
	return Ctx{
		Width: width,
		Theme: ThemeFrom(cfg, name == "notty"),
		Inner: func(md string, w int) (string, error) {
			r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(w))
			if err != nil {
				return "", err
			}
			out, err := r.Render(md)
			if err != nil {
				return "", err
			}
			var ls []string
			for _, l := range strings.Split(strings.Trim(out, "\n"), "\n") {
				ls = append(ls, calloutTrimPad.ReplaceAllString(l, ""))
			}
			return strings.Join(ls, "\n"), nil
		},
	}
}

// calloutTrimPad drops trailing pad cells: plain or styled spaces.
var calloutTrimPad = regexp.MustCompile(`(?: |\x1b\[[0-9;]*m +\x1b\[m)+$`)

const calloutMD = "Alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho.\n\n- one **bold**\n- two `code`\n"

func calloutCases() map[string]Directive {
	return map[string]Directive{
		"callout-note":    {Kind: KindCallout, Callout: &Callout{Kind: "note", Body: calloutMD}},
		"callout-warning": {Kind: KindCallout, Callout: &Callout{Kind: "warning", Title: "Careful with this long title that wraps", Body: calloutMD}},
		"details":         {Kind: KindDetails, Details: &Details{Title: "More about it", Body: calloutMD}},
		"details-default": {Kind: KindDetails, Details: &Details{Body: "short\n"}},
		"attribution":     {Kind: KindAttribution, Attribution: &Attribution{Quote: calloutMD, Source: "Ada *Lovelace*, 1843"}},
	}
}

func TestCalloutDetailsAttribution(t *testing.T) {
	for name, d := range calloutCases() {
		for _, style := range []string{"dark", "light", "notty"} {
			for _, w := range []int{36, 76} {
				t.Run(fmt.Sprintf("%s/%s/%d", name, style, w), func(t *testing.T) {
					lines, _, err := Render(d, calloutCtx(t, style, w))
					if err != nil {
						t.Fatal(err)
					}
					if len(lines) == 0 {
						t.Fatal("no lines")
					}
					checkWidth(t, lines, w)
					got := strings.Join(lines, "\n") + "\n"
					if style == "notty" && strings.Contains(got, "\x1b") {
						t.Errorf("SGR in notty: %q", got)
					}
					golden(t, fmt.Sprintf("%s-%s-%d", name, style, w), got)
				})
			}
		}
	}
}

func TestCalloutDetailsAttributionNarrow(t *testing.T) {
	for name, d := range calloutCases() {
		for _, style := range []string{"dark", "notty"} {
			for _, w := range []int{1, 2, 3, 10} {
				lines, _, err := Render(d, calloutCtx(t, style, w))
				if err != nil {
					t.Fatalf("%s %d: %v", name, w, err)
				}
				checkWidth(t, lines, w)
			}
		}
	}
}
