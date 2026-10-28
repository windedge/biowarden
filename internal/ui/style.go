package ui

import "github.com/egoist/mygo/ui"

// Biowarden's design tokens. Every view takes its sizes and colors from here,
// so the four pages and the two dialogs look like one app, and the light and
// dark appearances are one design drawn in two palettes rather than two looks.
//
// The font sizes are rem multiples of the theme's font size, not DIPs: the
// desktop's text scale then grows the text and the room around it together,
// which a fixed ui.FontSize would not do. The comments name the DIPs they come
// out at on a default desktop.
const (
	fontCaption   float32 = 0.79 // 11: paths, timestamps, secondary meta text
	fontSecondary float32 = 0.86 // 12: hints, notices, a row's second line
	fontBody      float32 = 1    // 14: running text and controls
	fontEmphasis  float32 = 1.14 // 16: a dialog's title
	fontHeadline  float32 = 1.29 // 18: a page's title
)

// The spacing scale is the set of gaps the views use, from the hairline
// between a name and its caption up to a page's edge. The numbers are not
// arbitrary: they are the sizes the pages were laid out with, so naming them
// changes no layout.
const (
	spaceHair   float32 = 2  // between a name and the line under it
	spaceTiny   float32 = 4  // between stacked labels of one field
	spaceSnug   float32 = 6  // between a row's label and its value
	spaceSmall  float32 = 8  // between the lines of one block
	spaceRow    float32 = 10 // between the controls of one row
	spaceMedium float32 = 12 // between a page's blocks
	spaceLarge  float32 = 16 // a page's padding and the gap between sections
	// pagePadding is how far a page's content stays off the window's edge. The
	// layout tests measure the content area by it.
	pagePadding = spaceLarge
)

// The radii are three steps: a control's corner, a panel's, and a popup's,
// which sits inside a panel and is rounded a touch more so the two edges do
// not appear to fight.
const (
	radiusControl float32 = 6
	radiusPanel   float32 = 8
	radiusPopup   float32 = radiusControl + 2
)

// inkDark and inkLight are the two readable inks on a colored fill.
var (
	inkDark  = ui.Hex("#1c1c21")
	inkLight = ui.Hex("#f7f7fa")
)

// palette is one appearance's colors: a soft gray window, white (or near
// black) cards with a hairline edge, ink text in two grays, and one blue accent
// used sparingly enough that it still means "the action here". The soft colors
// are the wash under a badge or a row that is tinted rather than filled.
type palette struct {
	dark                                    bool
	bg, surface, surfaceHover, surfacePress ui.Color
	border                                  ui.Color
	text, textMuted                         ui.Color
	accent, accentSoft                      ui.Color
	danger, dangerSoft                      ui.Color
	warning, warningSoft                    ui.Color
	success, successSoft                    ui.Color
}

// lightPalette is the day appearance: the gray of the window, the white of a
// card, and the blue the app has always drawn its primary button in.
func lightPalette() palette {
	return palette{
		bg: ui.Hex("#f4f4f6"), surface: ui.Hex("#ffffff"),
		surfaceHover: ui.Hex("#f1f1f4"), surfacePress: ui.Hex("#e9e9ee"),
		border: ui.Hex("#e3e3e8"),
		text:   ui.Hex("#1c1c21"), textMuted: ui.Hex("#71717a"),
		accent: ui.Hex("#2563eb"), accentSoft: ui.Hex("#e8effd"),
		danger: ui.Hex("#dc2626"), dangerSoft: ui.Hex("#fbe9e9"),
		warning: ui.Hex("#b45309"), warningSoft: ui.Hex("#fdf3e2"),
		success: ui.Hex("#189a58"), successSoft: ui.Hex("#e4f6ec"),
	}
}

// darkPalette is the night appearance: the window a shade darker than the
// cards it holds, so a card still reads as a card, and the accent lifted to a
// light blue that carries on a dark surface.
func darkPalette() palette {
	return palette{
		dark:         true,
		bg:           ui.Hex("#1a1a1e"),
		surface:      ui.Hex("#232327"),
		surfaceHover: ui.Hex("#2d2d33"), surfacePress: ui.Hex("#36363d"),
		border: ui.Hex("#333339"),
		text:   ui.Hex("#ededf1"), textMuted: ui.Hex("#a1a1aa"),
		accent: ui.Hex("#60a5fa"), accentSoft: ui.Hex("#1e2a44"),
		danger: ui.Hex("#f28b82"), dangerSoft: ui.Hex("#45272a"),
		warning: ui.Hex("#f2b544"), warningSoft: ui.Hex("#3f3220"),
		success: ui.Hex("#6fd99b"), successSoft: ui.Hex("#1e3a2b"),
	}
}

// applyPalette makes the frame use biowarden's palette for the appearance the
// system is in, which MyGo reports as the theme it handed out for this frame.
// Every view that owns a window calls it first, before anything reads
// c.Theme(): the widgets built after the call see the palette, and the ones
// already built kept the frame's default.
//
// It is exported to the package rather than to callers so the tests can drive
// a window of their own through the same path: a test that built its own root
// and skipped this would paint the default theme and pass whatever the palette
// did.
func applyPalette(c *ui.Context) {
	p := lightPalette()
	if c.Theme().Dark {
		p = darkPalette()
	}
	t := *c.Theme()
	t.Dark = p.dark
	t.Background = p.bg
	t.Surface = p.surface
	t.SurfaceHover = p.surfaceHover
	t.SurfacePressed = p.surfacePress
	t.Border = p.border
	t.Text = p.text
	t.TextMuted = p.textMuted
	t.Accent = p.accent
	t.AccentHover = p.accent.Mix(p.text, 0.12)
	t.AccentPressed = p.accent.Mix(p.text, 0.22)
	t.AccentText = inkOn(p.accent)
	t.Danger = p.danger
	t.Warning = p.warning
	t.Success = p.success
	t.Selection = p.accent.Alpha(0.25)
	t.Focus = p.accent.Alpha(0.5)
	t.Scrollbar = p.text.Alpha(0.28)
	t.Radius = radiusControl
	c.SetTheme(&t)
}

// inkOn returns the ink to read text drawn on fill. The palette's own text
// color is not it: the question is only what stands out on that one fill, so a
// light accent takes dark ink and a dark one takes light ink. It is about a
// label on a colored badge, not about the accent's role in the page.
func inkOn(fill ui.Color) ui.Color {
	if luminance(fill) > 0.55 {
		return inkDark
	}
	return inkLight
}

// luminance is a color's perceived brightness, 0 for black and 1 for white, by
// the coefficients sRGB weights the channels with.
func luminance(c ui.Color) float32 {
	return (0.2126*float32(c.R) + 0.7152*float32(c.G) + 0.0722*float32(c.B)) / 255
}
