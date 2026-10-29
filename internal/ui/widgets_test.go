package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// contrast, relativeLuminance and powf live in style.go: the widgets measure
// their inks with them at paint time, and the tests measure the same numbers.

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

// A status badge is a label on a wash, so it owes the 4.5:1 a label needs -
// with headroom: the mix stops at 5.0:1 because a ratio that only grazes the
// line (the day success ink landed on 4.53:1) reads thin on real screens.
// The tone's own color on its own wash is under even the bare line (the day
// palette's accent on accentSoft is 4.48:1), which is what made "installed"
// unreadable in the day appearance. The neutral pill is measured too, because
// it is the one a browser that is registered nowhere wears.
func TestBadgeInkClearsFiveToOne(t *testing.T) {
	for _, dark := range []bool{false, true} {
		p := lightPalette()
		if dark {
			p = darkPalette()
		}
		tones := map[string]badgeTone{
			"neutral": badgeNeutral,
			"info":    badgeInfo,
			"success": badgeSuccess,
			"warning": badgeWarning,
			"danger":  badgeDanger,
		}
		for name, tone := range tones {
			pattern := ui.LightTheme()
			if dark {
				pattern = ui.DarkTheme()
			}
			pattern.Text = p.text
			ink, wash := badgeColors(pattern, p, tone)
			got := contrast(ink, wash)
			t.Logf("%s badge %s: ink %v on %v = %.2f:1", appearanceName(dark), name, ink, wash, got)
			if got < 5.0 {
				t.Errorf("%s %s badge: contrast %.2f:1, want >= 5.0:1", appearanceName(dark), name, got)
			}
		}
	}
}

// Captions are the muted ink, and the browsers page draws a manifest path in
// it on the page's own gray rather than on a card, so the muted ink is
// measured against both surfaces.
func TestMutedCaptionClearsFourAndAHalfToOne(t *testing.T) {
	for _, dark := range []bool{false, true} {
		p := lightPalette()
		if dark {
			p = darkPalette()
		}
		for name, bg := range map[string]ui.Color{"page": p.bg, "card": p.surface} {
			got := contrast(p.textMuted, bg)
			t.Logf("%s muted on %s: %v on %v = %.2f:1", appearanceName(dark), name, p.textMuted, bg, got)
			if got < 4.5 {
				t.Errorf("%s muted ink on the %s: contrast %.2f:1, want >= 4.5:1", appearanceName(dark), name, got)
			}
		}
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
