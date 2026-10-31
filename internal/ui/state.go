package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/registry"
	"github.com/windedge/biowarden/internal/secrets"
)

func itoa(n int) string { return strconv.Itoa(n) }

// navPage is which page the sidebar has chosen. The Account page is the
// landing page, as the reference desktop client lands on the vault.
type navPage int

const (
	pageAccount navPage = iota
	pageBrowsers
	pageSettings
	pageLog
)

// appState is the GUI's whole state: which page is showing, that page's
// working data, and the shared notice lines. Page-specific fields live with
// their pages, in state_<page>.go, so parallel work on one page does not
// collide with another.
type appState struct {
	page navPage

	browsers browsersState
	account  accountState
	settings settingsState
	log      logState
	// login is the modal login window's state; the account page opens it
	// instead of drawing a login form of its own.
	login loginWindow
	// firstRun is the launch-time zero-configuration setup: it registers what
	// it can and opens the login window once.
	firstRun firstRunState
	// loginWin is the native window while it is up; nil once it closed.
	loginWin *mygo.Window
	// loginLocaleOff stops the login window's language listener; nil while no
	// window is up. The window is rebuilt per attempt, so its listener is too.
	loginLocaleOff func()
	// browsersWin is the bulk-overwrite confirmation dialog while it is up;
	// nil once it closed.
	browsersWin *mygo.Window
	// browsersConfirmLocaleOff stops the confirmation dialog's language
	// listener; nil while no dialog is up. Like the login window, the dialog
	// is rebuilt per bulk install, so its listener is too.
	browsersConfirmLocaleOff func()

	// mainWindow is the window start creates; the login window is its modal
	// child, and the login state needs it to open one.
	mainWindow *mygo.Window
}

// newAppState wires the pages onto their backends and is the GUI process's
// one assembly point: the credential store, the Hello verifier and the
// registry seams are built here, once, and handed down to every constructor.
// hello.New owns an OS thread and a hidden window per instance, so one
// verifier per process is a hard requirement; passing it in is what
// guarantees the single instance, with no sync.Once left to enforce it.
func newAppState() *appState {
	store := secrets.New()
	verifier := hello.New()
	reg := registry.New()
	prefs := newServerPref()
	settings := newRealSettings(verifier)
	app := &appState{
		browsers: newBrowsersState(reg),
		settings: newSettingsState(settings, verifier, newAppearancePref()),
		log:      newLogState(settings),
	}
	app.account = newRealAccountState(store, verifier, newWincredSessionStore(secrets.SessionPrefix), prefs)
	app.login = newLoginWindow(&app.account)
	app.browsers.openConfirmWindow = app.showBrowsersConfirmWindow
	app.browsers.closeConfirmWindow = app.hideBrowsersConfirmWindow
	app.firstRun = newFirstRunState(newRealFirstRun(store, reg, registry.NewMarker()))
	app.login.openWindow = app.showLoginWindow
	app.login.closeWindow = app.hideLoginWindow
	app.login.resizeWindow = app.resizeLoginWindow
	return app
}

// tick advances simulated work and asks for the frame that finishes it.
// Every page calls it first, so a pending action still settles if the human
// switches pages before it ends.
func (app *appState) tick(c *ui.Context) {
	app.browsers.tick(c)
	app.account.tick(c)
	app.settings.tick(c)
	// The login state ticks here as well as in its own window: a cancel that
	// lands while that window closes must still reach the account page.
	app.login.tick(c)
}

// noticeSource is a status line that knows how loud it is. The pages keep the
// notice struct rather than rendered text, so a language change re-says the
// line at the next frame and the severity comes from the kind, not the words.
type noticeSource interface {
	text() string
	severity() noticeSeverity
	glyph() *ui.SVG
}

// The banner's padding: a line of secondary text plus the room that keeps the
// row one notice line high on the pages.
const (
	noticePadY float32 = spaceSnug
	noticePadX float32 = spaceSmall
)

// noticeOf draws a page's status banner from the notice that carries its own
// severity.
func noticeOf(c *ui.Context, n noticeSource) {
	if n == nil {
		return
	}
	noticeBanner(c, n.text(), n.severity(), n.glyph())
}

// notice draws a status line whose only severity signal is whether it reports
// a success; see plainNotice.
func notice(c *ui.Context, text string, ok bool) {
	noticeOf(c, plainNotice{line: text, ok: ok})
}

// noticeBanner draws one status line as a banner: the severity's soft wash
// under the whole content width, its mark at the leading edge, and the wording
// in the severity's ink. Empty text draws nothing, so a settled page keeps its
// layout.
func noticeBanner(c *ui.Context, text string, sev noticeSeverity, glyph *ui.SVG) {
	if text == "" {
		return
	}
	t := c.Theme()
	ink, wash := paletteFor(c).tone(sev)
	ui.Row(c).FillWidth().Padding(noticePadY, noticePadX).Radius(t.Radius).
		Background(wash).Gap(spaceSmall).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, glyph).FontSize(t.Rem(fontBody)).Shrink(0).TextColor(ink)
		ui.Text(c, text).Grow(1).FontSize(t.Rem(fontSecondary)).FontWeight(600).TextColor(ink)
	})
}

func muted(c *ui.Context, text string) {
	t := c.Theme()
	ui.Text(c, text).FontSize(t.Rem(fontSecondary)).TextColor(t.TextMuted)
}

// groupBox stands in for the reference's GroupBox: visible grouping without
// a real form layout behind it.
func groupBox(c *ui.Context, title string, body func()) {
	t := c.Theme()
	ui.Column(c).Padding(spaceMedium).Gap(spaceSmall).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Text(c, title).Bold()
		body()
	})
}

// shortID keeps a long userId from pushing the row buttons out of view.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "..."
}

// trimSpace is the pages' shared input hygiene.
func trimSpace(s string) string { return strings.TrimSpace(s) }

// when formats a timestamp the way the reference's rows do.
func when(t time.Time) string { return t.Format("2006-01-02 15:04") }
