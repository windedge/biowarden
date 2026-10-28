package ui

import "github.com/egoist/mygo/ui"

// The five button levels. A page leads with one primary action, offers the
// rest as secondary, and keeps dismissals and view housekeeping as plain or
// link text; the level that ends something is danger, so "sign out",
// "uninstall" and "delete" can never be mistaken for a normal button.
type buttonLevel int

const (
	// buttonPrimary is the one action a page or a dialog leads to: the only
	// accent-filled control on it (install, log in, save).
	buttonPrimary buttonLevel = iota
	// buttonSecondary is every other action that writes something: a neutral
	// wash of the text color, which reads as a button on the page's gray and
	// on a card's white alike.
	buttonSecondary
	// buttonPlain is a quiet action with no face until the pointer is on it:
	// a dismissal, a copy, a view reset.
	buttonPlain
	// buttonDanger ends something: a sign-out, a manifest removal, a deleted
	// key. It is the only red fill the app draws.
	buttonDanger
	// buttonLink is a text action that reads as a link: the accent ink, with
	// the accent's wash under the pointer.
	buttonLink
)

// The danger level's fill, the same in both appearances: it is a fill with a
// light ink on it, not an ink on the palette's surface, so it does not follow
// the palette. Hover and press darken it, which only raises the contrast.
//
// Measured contrast (WCAG 2.1: relative luminance of each sRGB channel
// linearized as ((c/255+0.055)/1.055)^2.4 below 0.03928 and c/255/12.92 under
// it, ratio (Llight+0.05)/(Ldark+0.05)) of the ink #f7f7fa on each state:
//
//	rest  #b91c1c: 6.05:1
//	hover #a31919: 7.25:1
//	pressed #901616: 8.49:1
//
// The reason for a deeper red than the palette's is that a light ink does not
// work on one: mygo's dark Danger #ef4444 carries white at 3.76:1 and the
// palette's dark danger #f28b82 at 2.4:1, both under the 4.5:1 a label needs.
var (
	dangerFill  = ui.Hex("#b91c1c")
	dangerHover = dangerFill.Mix(ui.RGB(0, 0, 0), 0.12)
	dangerPress = dangerFill.Mix(ui.RGB(0, 0, 0), 0.22)
)

// buttonFace is one level's colors: the fill at rest, under the pointer and
// held down, and the ink drawn on it. filled says the level draws a fill at
// rest, which is what the disabled form replaces with a neutral wash.
type buttonFace struct {
	fill, hover, press ui.Color
	ink                ui.Color
	filled             bool
}

// buttonFaceFor is the level's colors in one appearance. Primary takes the
// theme's accent trio outright, so it stays the app's blue; the neutral
// levels are washes of the palette's text color, which sit on a card and on
// the page's gray without a border to tell them apart from it.
func buttonFaceFor(t *ui.Theme, p palette, level buttonLevel) buttonFace {
	switch level {
	case buttonPrimary:
		return buttonFace{fill: t.Accent, hover: t.AccentHover, press: t.AccentPressed,
			ink: t.AccentText, filled: true}
	case buttonSecondary:
		return buttonFace{fill: p.text.Alpha(0.07), hover: p.text.Alpha(0.12),
			press: p.text.Alpha(0.18), ink: t.Text, filled: true}
	case buttonDanger:
		return buttonFace{fill: dangerFill, hover: dangerHover, press: dangerPress,
			ink: inkLight, filled: true}
	case buttonLink:
		return buttonFace{fill: ui.Transparent, hover: p.accentSoft,
			press: p.accentSoft.Mix(p.accent, 0.2), ink: t.Accent}
	default:
		return buttonFace{fill: ui.Transparent, hover: p.text.Alpha(0.07),
			press: p.text.Alpha(0.12), ink: t.Text}
	}
}

// disabledFill is the face of a disabled filled button: the same neutral wash
// a secondary button wears at rest, never the accent at low opacity. The ink
// turns muted on every level, so a disabled button says "not now" in gray
// whatever it would have been.
func disabledFill(p palette) ui.Color { return p.text.Alpha(0.07) }

// button draws one level of the hierarchy. All five share the metrics (a
// single line, the theme's control padding, the control radius), so a row of
// them lines up whatever their levels.
//
// disabled is a parameter rather than a chained call because the level's face
// is chosen while the button is built: a chained Disabled would be read a
// frame late.
func button(c *ui.Context, level buttonLevel, label string, disabled bool) ui.Element {
	t := c.Theme()
	p := paletteFor(c)
	f := buttonFaceFor(t, p, level)
	padX := t.Space(3.5)
	if level == buttonPlain || level == buttonLink {
		padX = t.Space(2)
	}
	b := ui.ButtonBase(c).Key(label).Label(label).Shrink(0).AlignItems(ui.Center).
		Radius(radiusControl).Padding(t.Space(1.5), padX).TextColor(f.ink)
	fill := f.fill
	switch {
	case disabled:
		b = b.TextColor(t.TextMuted)
		if f.filled {
			fill = disabledFill(p)
		}
	case b.Pressed():
		fill = f.press
	case b.Hovered():
		fill = f.hover
	}
	b = b.Background(b.AnimateColor("button-fill", fill, ui.Smooth)).Disabled(disabled)
	if !disabled {
		b = b.Cursor(ui.CursorPointer)
	}
	if label != "" {
		b = b.Children(func() { ui.Text(c, label).SingleLine() })
	}
	return b
}

// The five levels, named at the call sites that pick one: who has to read
// the call knows what the button is without counting iota constants.
func primaryButton(c *ui.Context, label string, disabled bool) ui.Element {
	return button(c, buttonPrimary, label, disabled)
}

func secondaryButton(c *ui.Context, label string, disabled bool) ui.Element {
	return button(c, buttonSecondary, label, disabled)
}

func plainButton(c *ui.Context, label string, disabled bool) ui.Element {
	return button(c, buttonPlain, label, disabled)
}

func dangerButton(c *ui.Context, label string, disabled bool) ui.Element {
	return button(c, buttonDanger, label, disabled)
}

func linkButton(c *ui.Context, label string, disabled bool) ui.Element {
	return button(c, buttonLink, label, disabled)
}

// badgeTone is a status pill's color language: the severity of what it says,
// plus a neutral for a state that is merely a fact (a browser that is not
// registered anywhere).
type badgeTone int

const (
	badgeNeutral badgeTone = iota
	badgeInfo
	badgeWarning
	badgeDanger
)

// badge is a status pill: a text in the tone's ink over the tone's wash. It
// takes the same colors notice.go gives a banner, so a state said in a row
// and the same state reported in a banner are one color language.
func badge(c *ui.Context, label string, tone badgeTone) ui.Element {
	t := c.Theme()
	p := paletteFor(c)
	ink, wash := t.TextMuted, p.text.Alpha(0.07)
	switch tone {
	case badgeInfo:
		ink, wash = p.accent, p.accentSoft
	case badgeWarning:
		ink, wash = p.warning, p.warningSoft
	case badgeDanger:
		ink, wash = p.danger, p.dangerSoft
	}
	return ui.Row(c).Shrink(0).AlignItems(ui.Center).Radius(radiusControl + 2).
		Padding(spaceHair, spaceSnug).Background(wash).Children(func() {
		ui.Text(c, label).FontSize(t.Rem(fontCaption)).TextColor(ink).SingleLine()
	})
}
