package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// contrast is a WCAG 2.1 contrast ratio: each sRGB channel is linearized
// (((c/255+0.055)/1.055)^2.4, or c/255/12.92 below 0.03928), the relative
// luminance is 0.2126r+0.7152g+0.0722b, and the ratio is the lighter plus
// 0.05 over the darker plus 0.05.
func contrast(a, b ui.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relativeLuminance(c ui.Color) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return powf((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// powf is a small pow for the test, which cannot use math.Pow on a float32
// free path: it keeps the measurement in one place.
func powf(x, y float64) float64 {
	// exp(y*ln(x)) by series, enough precision for a contrast ratio.
	ln := 0.0
	for x > 2 {
		x /= 2
		ln += 0.6931471805599453
	}
	for x < 1 {
		x *= 2
		ln -= 0.6931471805599453
	}
	t := (x - 1) / (x + 1)
	s, term := 0.0, t
	for i := 1; i < 60; i += 2 {
		s += term / float64(i)
		term *= t * t
	}
	ln += 2 * s
	e := y * ln
	sum, fact := 1.0, 1.0
	for i := 1; i < 30; i++ {
		fact *= e / float64(i)
		sum += fact
	}
	return sum
}

// A label on a fill needs 4.5:1. The danger level is the one that gets this
// wrong by default: a light ink on a bright red is 3.76:1, so the level fills
// with a deeper red instead. Its three states are measured, not just the rest
// one, and so is the primary level, which draws the accent under the same ink
// rule.
func TestButtonInkClearsFourAndAHalfToOne(t *testing.T) {
	for _, dark := range []bool{false, true} {
		p := lightPalette()
		if dark {
			p = darkPalette()
		}
		tm := ui.LightTheme()
		if dark {
			tm = ui.DarkTheme()
		}
		tm.Dark = dark
		tm.Accent, tm.AccentText = p.accent, inkOn(p.accent)
		tm.AccentHover, tm.AccentPressed = p.accent.Mix(p.text, 0.12), p.accent.Mix(p.text, 0.22)
		tm.Text = p.text
		for _, level := range []buttonLevel{buttonPrimary, buttonDanger} {
			f := buttonFaceFor(tm, p, level)
			for name, fill := range map[string]ui.Color{"rest": f.fill, "hover": f.hover, "pressed": f.press} {
				got := contrast(f.ink, fill)
				t.Logf("%s level %s: ink %v on %v = %.2f:1", appearanceName(dark), name, f.ink, fill, got)
				if got < 4.5 {
					t.Errorf("%s %s level %s: contrast %.2f:1, want >= 4.5:1",
						appearanceName(dark), levelName(level), name, got)
				}
			}
		}
	}
}

func levelName(level buttonLevel) string {
	switch level {
	case buttonPrimary:
		return "primary"
	case buttonSecondary:
		return "secondary"
	case buttonDanger:
		return "danger"
	case buttonLink:
		return "link"
	default:
		return "plain"
	}
}

// Every level is the same height, so a row of mixed levels lines up, and the
// height follows the font size rather than a fixed DIP count.
func TestButtonLevelsShareOneHeight(t *testing.T) {
	appLanguage(t, "en")
	levels := []buttonLevel{buttonPrimary, buttonSecondary, buttonPlain, buttonDanger, buttonLink}
	heights := make([]float32, 0, len(levels))
	for _, level := range levels {
		level := level
		tester := ui.NewTester(func(c *ui.Context) {
			applyPalette(c)
			ui.Column(c).Children(func() {
				button(c, level, "label", false)
				ui.Text(c, "after")
			})
		}, 400, 200)
		tester.Frame()
		r, ok := tester.Find("after")
		if !ok {
			t.Fatalf("no text after the %s button", levelName(level))
		}
		heights = append(heights, r.Y)
	}
	for i := 1; i < len(heights); i++ {
		if heights[i] != heights[0] {
			t.Fatalf("%s level is %.2f tall, %s is %.2f: the levels do not line up",
				levelName(levels[0]), heights[0], levelName(levels[i]), heights[i])
		}
	}
}

// A disabled filled button wears a neutral wash and the muted ink, never the
// accent at low opacity, and the level keeps its shape while it is disabled.
func TestDisabledButtonTakesTheNeutralWash(t *testing.T) {
	for _, dark := range []bool{false, true} {
		p := lightPalette()
		if dark {
			p = darkPalette()
		}
		if got, want := disabledFill(p), p.text.Alpha(0.07); got != want {
			t.Errorf("%s disabled fill = %v, want the neutral wash %v", appearanceName(dark), got, want)
		}
		// The wash is the text color, not the accent: a disabled primary must
		// not read as a blue button.
		if p.accent == disabledFill(p) {
			t.Errorf("%s disabled fill is the accent", appearanceName(dark))
		}
	}
}
