package ui

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/windedge/biowarden/internal/auth"
	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/locales"
)

var logLevels = []logLevel{logLevelError, logLevelInfo, logLevelDebug}

// logLevel is the Settings page's log-level choice. It is an enum, not a
// display string, so its label follows the language and choosing one never
// writes a translated label back as the value.
type logLevel int

const (
	logLevelError logLevel = iota
	logLevelInfo
	logLevelDebug
)

// label names the level in the language the app shows.
func (l logLevel) label() string {
	switch l {
	case logLevelError:
		return locales.SettingsLogLevelError()
	case logLevelDebug:
		return locales.SettingsLogLevelDebug()
	default:
		return locales.SettingsLogLevelInfo()
	}
}

// helloOutcome is one background Available result, handed to tick through a
// channel so the blocking COM call never runs on the UI thread.
type helloOutcome struct {
	ok  bool
	err error
}

// proxyTestState is the Test connection row's outcome. The wording stays a
// state, not a string, so a language change rewords the line at the next frame.
type proxyTestState int

const (
	proxyTestIdle proxyTestState = iota
	proxyTestTesting
	proxyTestOK
	proxyTestRefused
	proxyTestUnreachable
)

func (s proxyTestState) label(host string) string {
	if host == "" {
		switch s {
		case proxyTestTesting:
			return "Testing..."
		case proxyTestOK:
			return "The server answered through the proxy; sign-in is not checked."
		case proxyTestUnreachable:
			return "Cannot reach the server through the proxy."
		default:
			return "Not tested"
		}
	}
	switch s {
	case proxyTestTesting:
		return locales.SettingsProxyTestTesting(host)
	case proxyTestOK:
		return locales.SettingsProxyTestOK(host)
	case proxyTestRefused:
		return locales.SettingsProxyTestRefused()
	case proxyTestUnreachable:
		return locales.SettingsProxyTestUnreachable(host)
	default:
		return locales.SettingsProxyTestIdle(host)
	}
}

type proxyTestOutcome struct{ state proxyTestState }

// proxyTestHost names the server the probe contacts, without scheme or path.
// An unparseable address leaves no name to show, so the generic wording is
// used instead.
func (s *settingsState) proxyTestHost() string {
	if s.serverURL == nil {
		return ""
	}
	raw := strings.TrimSpace(s.serverURL())
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}

// settingsState is the Settings page's working data. The login item and the
// Hello status are real; the tray switch and the log level stay in memory.
type settingsState struct {
	backend settingsBackend
	hello   hello.Verifier
	// locale is the app's language, which the Language row reads and sets.
	locale localeBackend
	// marker records whether the Language row's choice was explicit, so the
	// row can tell it apart from the system's own languages on the next launch.
	marker localeMarker
	// languageTag is the Language row's choice: "" follows the system's
	// languages. mygo keeps the choice across launches and reports only the
	// language it shows, so newSettingsState guesses it back from the marker
	// when there is one, and from mygo's shown language otherwise.
	languageTag string
	// appearance is the Appearance row's choice and appearancePref remembers
	// it under HKCU. setSource is where the choice reaches the app; it is a
	// field so a test can watch it without mygo's app.
	appearance     appearance
	appearancePref appearancePref
	setSource      func(mygo.ThemeSource)

	// proxy is the live preference the auth transport also reads; the form
	// fields below are a draft, committed on a mode change or when Settings
	// is left. Typing alone never writes a value.
	proxy         proxyPref
	proxyMode     proxyMode
	proxyAddress  string
	proxyUser     string
	proxyPassword string
	// proxyUseCredentials shows the username and password rows: most proxies
	// are anonymous, but stored credentials check the box at startup.
	proxyUseCredentials bool
	// showProxyPassword follows the login window's eye button.
	showProxyPassword bool
	// serverURL picks the account's own server for the proxy test; the default
	// is the official cloud when there is no account.
	serverURL func() string
	// probe is a field so a test can keep the async check off the network.
	probe           func() proxyTestOutcome
	proxyTesting    bool
	proxyTest       proxyTestState
	proxyTestResult asyncRunner[proxyTestOutcome]

	launchAtLogin bool
	showTray      bool

	helloChecking bool
	helloOK       bool
	helloResult   asyncRunner[helloOutcome]

	logLevel logLevel
	logPath  string

	notice pageNotice
}

// newSettingsState takes the assembled dependencies: the settings backend
// (the Log page receives the same instance), the verifier the Hello row
// checks with, the appearance preference, the language marker and the proxy
// seam.
func newSettingsState(backend settingsBackend, verifier hello.Verifier, appearancePref appearancePref, marker localeMarker, proxy proxyPref) settingsState {
	s := settingsState{
		backend:         backend,
		hello:           verifier,
		locale:          mygoLocale{},
		marker:          marker,
		appearancePref:  appearancePref,
		setSource:       mygo.Theme.SetSource,
		helloResult:     startAsync[helloOutcome](1),
		proxy:           proxy,
		proxyTestResult: startAsync[proxyTestOutcome](1),
		showTray:        true,
		logLevel:        logLevelInfo,
		logPath:         backend.logPath(),
	}
	s.probe = s.probeProxy
	if s.proxy != nil {
		s.proxyMode = s.proxy.Mode()
		s.proxyAddress = s.proxy.Address()
		s.proxyUser = s.proxy.User()
		if password, ok := s.proxy.Password(); ok {
			s.proxyPassword = password
		}
	}
	s.proxyUseCredentials = s.proxy != nil && s.proxy.HasCredentials()
	if selected := s.proxyOptions(); !containsMode(selected, s.proxyMode) {
		// A preference written by a build with a system reader must still be
		// editable on one without it.
		s.proxyMode = proxySystem
	}
	s.languageTag = s.guessLanguageTag()
	s.appearance = s.appearancePref.load()
	if on, err := backend.autostart(); err != nil {
		s.notice = pageNotice{kind: noticeSettingsAutostartReadFailed, err: err}
	} else {
		s.launchAtLogin = on
	}
	// Ask once at startup so the page shows the machine's real Hello state.
	s.startHelloCheck()
	return s
}

// tick settles the background Hello check. The goroutine's result arrives
// through the runner; until then it keeps a frame scheduled so the page stops
// showing "checking" as soon as the answer lands.
func (s *settingsState) tick(c *ui.Context) {
	if s.helloChecking && !s.helloResult.tick(func(r helloOutcome) {
		s.helloChecking = false
		s.helloOK = r.err == nil && r.ok
		switch {
		case r.err != nil:
			s.notice = pageNotice{kind: noticeSettingsHelloCheckFailed, err: r.err}
		case r.ok:
			s.notice = pageNotice{kind: noticeSettingsHelloAvailable, ok: true}
		default:
			s.notice = pageNotice{kind: noticeSettingsHelloUnavailable}
		}
	}) {
		c.After(100 * time.Millisecond)
	}
	s.tickProxyTest(c)
}

// tickProxyTest lands the finished probe on the UI thread and keeps a frame
// coming while the request is on the wire.
func (s *settingsState) tickProxyTest(c *ui.Context) {
	if !s.proxyTesting {
		return
	}
	if !s.proxyTestResult.tick(func(r proxyTestOutcome) {
		s.proxyTesting = false
		s.proxyTest = r.state
	}) {
		c.After(100 * time.Millisecond)
	}
}

func (s *settingsState) startHelloCheck() {
	if s.hello == nil || s.helloChecking {
		return
	}
	s.helloChecking = true
	verifier := s.hello
	s.helloResult.run(func() helloOutcome {
		ok, err := verifier.Available()
		return helloOutcome{ok: ok, err: err}
	})
}

// setAutostart writes the login item and puts the switch back when the write
// fails, so the page never shows an entry the OS refused.
func (s *settingsState) setAutostart() {
	if err := s.backend.setAutostart(s.launchAtLogin); err != nil {
		s.launchAtLogin = !s.launchAtLogin
		s.notice = pageNotice{kind: noticeSettingsAutostartWriteFailed, err: err}
		return
	}
	if s.launchAtLogin {
		s.notice = pageNotice{kind: noticeSettingsAutostartOn, ok: true}
	} else {
		s.notice = pageNotice{kind: noticeSettingsAutostartOff, ok: true}
	}
}

// appearanceSegmented renders the Appearance row and reports the choice made
// this frame, if any. The value the control carries is the index of the choice
// in appearances, so the labels follow the language and are never stored.
func (s *settingsState) appearanceSegmented(c *ui.Context) (appearance, bool) {
	labels := make([]string, len(appearances))
	for i, a := range appearances {
		labels[i] = a.label()
	}
	selected := int(s.appearance)
	// The segment is content-wide, not form-wide: inside a form's value column a
	// child stretches across the column, which drew a track as wide as the page
	// with three small pills in its corner. Aligning it to the start leaves it
	// at its own width, the way the language dropdown above it sits.
	ui.Segmented(c, &selected, labels...).Label(locales.SettingsAppearance()).AlignSelf(ui.Start)
	if selected == int(s.appearance) {
		return s.appearance, false
	}
	return appearance(selected), true
}

// setAppearance shows a at once and remembers it. The app's next frame reads
// the appearance from the theme, which applyPalette picks the palette by, so
// the open windows repaint in it; a failed write only costs the next launch.
func (s *settingsState) setAppearance(a appearance) {
	s.appearance = a
	if s.setSource != nil {
		s.setSource(a.source())
	}
	if s.appearancePref != nil {
		s.appearancePref.save(a)
	}
}

// proxyOptions is the mode list the row offers: System only exists when the
// platform seam can read it, so no page has to know the operating system.
func (s *settingsState) proxyOptions() []proxyMode {
	supported := s.proxy != nil && s.proxy.SystemSupported()
	return proxyModes(supported)
}

func containsMode(modes []proxyMode, want proxyMode) bool {
	for _, m := range modes {
		if m == want {
			return true
		}
	}
	return false
}

// proxyModeSelect renders the mode row and reports the choice made this
// frame. The value is an enum, so a translated label never becomes the stored
// value.
func (s *settingsState) proxyModeSelect(c *ui.Context) (proxyMode, bool) {
	selected := s.proxyMode
	if !containsMode(s.proxyOptions(), selected) {
		selected = s.proxyOptions()[0]
	}
	t := c.Theme()
	sel := ui.SelectBase(c, &selected)
	sel.Trigger.Gap(spaceSnug).Padding(spaceTiny, spaceRow).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).
		Justify(ui.SpaceBetween).MaxWidth(settingsSelectWidth).
		Children(func() {
			ui.Text(c, selected.label()).SingleLine()
			ui.Text(c, "▾").TextColor(t.TextMuted)
		})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(spaceTiny, 0, 0, 0).Padding(spaceTiny).Radius(radiusPopup).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 6, 20, 0, paletteFor(c).popShadow())
		for _, mode := range s.proxyOptions() {
			item := sel.Item(mode).Padding(spaceSnug, spaceRow).Radius(t.Radius)
			if item.Highlighted() {
				item.Background(t.Accent).TextColor(t.AccentText)
			}
			item.Children(func() { ui.Text(c, mode.label()).SingleLine() })
		}
	})
	return selected, sel.Trigger.Changed() && selected != s.proxyMode
}

// setProxyMode applies a new choice at once: the new mode and the values in
// the draft go to the preference together, so a switch never saves a stale
// address or password alongside it.
func (s *settingsState) setProxyMode(mode proxyMode) {
	s.proxyMode = mode
	if mode == proxyOff {
		s.proxyPassword = ""
		s.proxyTest = proxyTestIdle
		s.proxyTesting = false
	}
	s.commitProxy()
}

// startProxyTest probes the committed preference, not the draft: the answer
// describes the configuration auth would really use, and the transport reads
// the seam on every request. Switching mode or leaving Settings commits first.
func (s *settingsState) startProxyTest() {
	if s.proxyTesting || s.probe == nil {
		return
	}
	s.proxyTesting = true
	s.proxyTest = proxyTestTesting
	probe := s.probe
	s.proxyTestResult.run(func() proxyTestOutcome { return probe() })
}

// probeProxy checks the account's API root, falling back to the official
// cloud. Only /api/config is requested: the answer says the proxy and server
// are reachable, not that a login would succeed.
func (s *settingsState) probeProxy() proxyTestOutcome {
	base := defaultServerURL
	if s.serverURL != nil {
		if v := trimSpace(s.serverURL()); v != "" {
			base = v
		}
	}
	cfg, err := auth.NewServerConfig(base)
	if err != nil {
		return proxyTestOutcome{state: proxyTestUnreachable}
	}
	return probeProxyURL(cfg.API() + "/api/config")
}

// probeProxyURL builds the ten-second client from auth's own factory so the
// test travels through the same shared transport as login and refresh.
func probeProxyURL(rawurl string) proxyTestOutcome {
	client := auth.NewClient(auth.DefaultServerConfig(), "proxy-check").WithTimeout(10 * time.Second)
	status, err := client.Probe(context.Background(), rawurl)
	return proxyTestOutcome{state: classifyProxyProbe(status, err)}
}

// classifyProxyProbe keeps the three honest outcomes distinct. A 407 answer,
// whether returned as a status or from CONNECT, is a proxy that refused or
// asked for credentials; everything else that failed is reported as an
// unreachable route, and any HTTP status means the server was reached.
func classifyProxyProbe(status int, err error) proxyTestState {
	if err == nil {
		if status == http.StatusProxyAuthRequired {
			return proxyTestRefused
		}
		return proxyTestOK
	}
	message := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, auth.ErrNetwork) &&
		(strings.Contains(message, "407") || strings.Contains(message, "proxy authentication required")):
		return proxyTestRefused
	case errors.Is(err, auth.ErrNetwork) && strings.Contains(message, "proxyconnect") &&
		(strings.Contains(message, "403") || strings.Contains(message, "502") ||
			strings.Contains(message, "tunnel") || strings.Contains(message, "bad gateway")):
		return proxyTestRefused
	default:
		return proxyTestUnreachable
	}
}

// logLevelSelect renders the log-level row with the enum's labels, as
// languageSelect does, so the options follow the language while the value
// stays an enum the page can read back.
func (s *settingsState) logLevelSelect(c *ui.Context) {
	t := c.Theme()
	sel := ui.SelectBase(c, &s.logLevel)
	sel.Trigger.Gap(spaceSnug).Padding(spaceTiny, spaceRow).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).
		Justify(ui.SpaceBetween).MaxWidth(settingsSelectWidth).
		Label(locales.SettingsLogLevelLabel()).
		Children(func() {
			ui.Text(c, s.logLevel.label()).SingleLine()
			ui.Text(c, "▾").TextColor(t.TextMuted)
		})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(spaceTiny, 0, 0, 0).Padding(spaceTiny).Radius(radiusPopup).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 6, 20, 0, paletteFor(c).popShadow())
		for _, lv := range logLevels {
			item := sel.Item(lv).Padding(spaceSnug, spaceRow).Radius(t.Radius)
			if item.Highlighted() {
				item.Background(t.Accent).TextColor(t.AccentText)
			}
			item.Children(func() { ui.Text(c, lv.label()).SingleLine() })
		}
	})
}

// languageOptions returns the Language row's choices: "" for the system's
// languages, then the languages the app is written in, each named in itself so
// a reader finds their own, as mygo's own language menu names them.
func (s *settingsState) languageOptions() (tags, labels []string) {
	tags, labels = []string{""}, []string{locales.LanguageSystem()}
	if s.locale == nil {
		return tags, labels
	}
	for _, tag := range s.locale.languages() {
		name := i18n.Autonym(tag)
		if name == "" {
			name = tag
		}
		tags, labels = append(tags, tag), append(labels, name)
	}
	return tags, labels
}

// languageSelect renders the Language row and reports the tag chosen this
// frame and whether the user chose one. The value bound to the control is the
// tag mygo SetLocale takes, so the row never has to map a display label back
// to a tag: the trigger and the options only show labels.
func (s *settingsState) languageSelect(c *ui.Context) (chosen string, changed bool) {
	tags, labels := s.languageOptions()
	chosen = s.languageTag
	t := c.Theme()
	sel := ui.SelectBase(c, &chosen)
	sel.Trigger.Gap(spaceSnug).Padding(spaceTiny, spaceRow).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).
		Justify(ui.SpaceBetween).MaxWidth(settingsSelectWidth).
		Children(func() {
			ui.Text(c, s.languageLabelFor(chosen)).SingleLine()
			ui.Text(c, "▾").TextColor(t.TextMuted)
		})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(spaceTiny, 0, 0, 0).Padding(spaceTiny).Radius(radiusPopup).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 6, 20, 0, paletteFor(c).popShadow())
		for i, tag := range tags {
			item := sel.Item(tag).Padding(spaceSnug, spaceRow).Radius(t.Radius)
			if item.Highlighted() {
				item.Background(t.Accent).TextColor(t.AccentText)
			}
			item.Children(func() { ui.Text(c, labels[i]).SingleLine() })
		}
	})
	return chosen, sel.Trigger.Changed()
}

// languageLabel names the choice the row shows.
func (s *settingsState) languageLabel() string {
	return s.languageLabelFor(s.languageTag)
}

// languageLabelFor names one choice, the first when the tag is unknown.
func (s *settingsState) languageLabelFor(tag string) string {
	tags, labels := s.languageOptions()
	for i, t := range tags {
		if t == tag {
			return labels[i]
		}
	}
	return labels[0]
}

// guessLanguageTag reports in which language the app shows a choice of its
// own: the persisted marker when a human chose one, otherwise it follows the
// system's languages while the app shows the one they ask for.
func (s *settingsState) guessLanguageTag() string {
	if s.marker != nil {
		if tag, ok := s.marker.load(); ok {
			return tag
		}
	}
	if s.locale == nil {
		return ""
	}
	if shown := s.locale.locale(); shown != "" && shown != systemLanguage(s.locale) {
		return shown
	}
	return ""
}

// setLanguage shows tag at once, "" for the system's languages again. mygo
// keeps it for the next launches; the marker is what tells a later launch the
// choice was explicit rather than the system guess.
func (s *settingsState) setLanguage(tag string) {
	s.languageTag = tag
	if s.locale != nil {
		s.locale.setLocale(tag)
	}
	if s.marker != nil {
		if tag == "" {
			s.marker.clear()
		} else {
			s.marker.save(tag)
		}
	}
}
