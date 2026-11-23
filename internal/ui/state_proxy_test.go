package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/internal/auth"
	"github.com/windedge/biowarden/internal/registry"
	"github.com/windedge/biowarden/internal/secrets"
	"github.com/windedge/biowarden/locales"
)

type memoryProxyPref struct {
	mode, address, user string
}

func (p *memoryProxyPref) Mode() registry.ProxyMode { return registry.ParseProxyMode(p.mode) }
func (p *memoryProxyPref) Address() string          { return p.address }
func (p *memoryProxyPref) User() string             { return p.user }
func (p *memoryProxyPref) Set(mode registry.ProxyMode, address, user string) {
	p.mode, p.address, p.user = string(mode), address, user
}
func (p *memoryProxyPref) Reset() { p.mode, p.address, p.user = "", "", "" }

type memoryProxySecrets struct {
	password string
	puts     int
	removes  int
}

func (s *memoryProxySecrets) GetProxyPassword() (string, error) {
	if s.password == "" {
		return "", secrets.ErrNotFound
	}
	return s.password, nil
}
func (s *memoryProxySecrets) PutProxyPassword(password string) error {
	s.password = password
	s.puts++
	return nil
}
func (s *memoryProxySecrets) RemoveProxyPassword() error {
	s.password = ""
	s.removes++
	return nil
}

func TestProxyModesHideUnsupportedSystem(t *testing.T) {
	if got := proxyModes(false); len(got) != 2 || got[0] != proxyManual {
		t.Fatalf("proxyModes(false) = %v", got)
	}
	if got := proxyModes(true); len(got) != 3 || got[0] != proxySystem {
		t.Fatalf("proxyModes(true) = %v", got)
	}
}

func TestOldEnvironmentProxyModeFallsBackToSystem(t *testing.T) {
	if got := proxyModeFromStored(registry.ProxyModeEnvironment); got != proxySystem {
		t.Fatalf("proxyModeFromStored(environment) = %v, want %v", got, proxySystem)
	}
	if got := proxySystem.authMode(); got != auth.ProxyModeSystem {
		t.Fatalf("system auth mode = %v, want %v", got, auth.ProxyModeSystem)
	}
}

func TestRuntimeProxySaveWritesAndOffClearsPassword(t *testing.T) {
	pref := &memoryProxyPref{}
	store := &memoryProxySecrets{password: "old"}
	p := &runtimeProxyPref{pref: pref, secrets: store}

	p.Save(proxyManual, "  socks5://127.0.0.1:1080 ", " alice ", "s3cret")
	if pref.mode != "manual" || pref.address != "socks5://127.0.0.1:1080" || pref.user != "alice" {
		t.Fatalf("stored preference = %+v", pref)
	}
	if store.password != "s3cret" || store.puts != 1 {
		t.Fatalf("stored password = %q, puts = %d", store.password, store.puts)
	}
	p.Save(proxyManual, pref.address, pref.user, "")
	if store.password != "" || store.removes != 1 {
		t.Fatalf("blank password kept = %q, removes = %d", store.password, store.removes)
	}
	store.password = "s3cret"
	// A mode without credentials clears the password even when the field still
	// holds one, which is what happens when the human switches straight to Off.
	p.Save(proxyOff, pref.address, pref.user, "s3cret")
	if store.password != "" || store.removes != 2 || pref.mode != "off" {
		t.Fatalf("off left password = %q, removes = %d, mode = %q", store.password, store.removes, pref.mode)
	}
}

func proxyFixture() (*fakeProxyPref, settingsState) {
	fake := &fakeProxyPref{
		mode:     proxyManual,
		address:  "old:8080",
		user:     "old",
		password: "oldpw",
	}
	s := settingsState{
		proxy:           fake,
		proxyMode:       proxyManual,
		proxyAddress:    "old:8080",
		proxyUser:       "old",
		proxyPassword:   "oldpw",
		proxyTestResult: startAsync[proxyTestOutcome](1),
		probe: func() proxyTestOutcome {
			return proxyTestOutcome{state: proxyTestOK}
		},
	}
	return fake, s
}

// Stored credentials decide whether the checkbox starts checked: an anonymous
// proxy shows a short form, while a saved login is visible on arrival.
func TestProxyCredentialsCheckboxDefaultsToStoredCredentials(t *testing.T) {
	cases := []struct {
		name string
		pref fakeProxyPref
		want bool
	}{
		{"anonymous", fakeProxyPref{mode: proxyManual}, false},
		{"registry user", fakeProxyPref{mode: proxyManual, user: "alice"}, true},
		{"credential store password", fakeProxyPref{mode: proxyManual, password: "s3cret"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pref := tc.pref
			// A settled marker keeps the language guess off mygo's UI thread.
			s := newSettingsState(fakeSettings{}, nil, &fakeAppearancePref{}, &fakeMarker{has: true}, &pref)
			if s.proxyUseCredentials != tc.want {
				t.Fatalf("proxyUseCredentials = %v, want %v", s.proxyUseCredentials, tc.want)
			}
		})
	}
}

// Unchecking hides the draft without clearing it; checking it shows it again.
func TestProxyCredentialsCheckboxTogglesWithoutClearing(t *testing.T) {
	appLanguage(t, "en")
	fake, s := proxyFixture()
	s.proxyUseCredentials = false
	app := &appState{settings: s}
	tester := ui.NewTester(app.settingsPage, 1200, 900)
	tester.Frame()

	if _, found := tester.Find(locales.SettingsProxyCredentials()); !found {
		t.Fatalf("no credentials checkbox: %v", tester.Texts())
	}
	if _, found := tester.Find(locales.SettingsProxyUser()); found {
		t.Fatal("unchecked box still shows the username")
	}

	if err := tester.Click(locales.SettingsProxyCredentials()); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !app.settings.proxyUseCredentials {
		t.Fatal("click did not check the box")
	}
	if _, found := tester.Find(locales.SettingsProxyUser()); !found {
		t.Fatal("checked box does not show the username")
	}
	if app.settings.proxyUser != "old" || app.settings.proxyPassword != "oldpw" {
		t.Fatalf("checking changed the draft: user=%q password=%q", app.settings.proxyUser, app.settings.proxyPassword)
	}

	if err := tester.Click(locales.SettingsProxyCredentials()); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if app.settings.proxyUseCredentials {
		t.Fatal("second click did not uncheck the box")
	}
	if _, found := tester.Find(locales.SettingsProxyUser()); found {
		t.Fatal("unchecked box still shows the username")
	}
	if app.settings.proxyUser != "old" || app.settings.proxyPassword != "oldpw" {
		t.Fatalf("unchecking changed the draft: user=%q password=%q", app.settings.proxyUser, app.settings.proxyPassword)
	}
	if len(fake.saveCalls) != 0 {
		t.Fatalf("toggling wrote: %+v", fake.saveCalls)
	}
}

func TestProxyFormModeSwitchCommitsDraft(t *testing.T) {
	fake, s := proxyFixture()
	s.proxyAddress, s.proxyUser, s.proxyPassword = "proxy:8080", "alice", "pw"
	s.setProxyMode(proxyOff)
	if len(fake.saveCalls) != 1 {
		t.Fatalf("save calls = %+v", fake.saveCalls)
	}
	want := proxySave{mode: proxyOff, address: "proxy:8080", user: "alice", password: ""}
	if fake.saveCalls[0] != want {
		t.Fatalf("save = %+v, want %+v", fake.saveCalls[0], want)
	}
	if fake.mode != proxyOff || fake.password != "" || fake.clearCalls != 1 {
		t.Fatalf("off left mode=%v password=%q clears=%d", fake.mode, fake.password, fake.clearCalls)
	}
	if s.proxyPassword != "" {
		t.Fatalf("off kept the draft password %q", s.proxyPassword)
	}
}

// A blank Manual address must never be stored: the transport would then
// fail every request, with only a debug log to explain why.
func TestProxyFormRejectsBlankManualAddress(t *testing.T) {
	for _, addr := range []string{"", "   "} {
		fake, s := proxyFixture()
		s.proxyAddress = addr
		s.proxyPassword = "newpw"
		if err := s.commitProxy(); !errors.Is(err, auth.ErrNoProxyAddress) {
			t.Fatalf("commit %q error = %v, want ErrNoProxyAddress", addr, err)
		}
		if len(fake.saveCalls) != 0 || fake.address != "old:8080" || fake.password != "oldpw" {
			t.Fatalf("blank address wrote: calls=%+v address=%q password=%q", fake.saveCalls, fake.address, fake.password)
		}
		if s.notice.kind != noticeSettingsProxyAddressEmpty || s.notice.ok {
			t.Fatalf("notice = %+v, want the danger proxy address notice", s.notice)
		}
	}

	// Leaving Settings is the other commit path, and it must refuse too.
	fake, s := proxyFixture()
	s.proxyAddress = ""
	app := &appState{page: pageSettings, settings: s}
	app.setPage(pageLog)
	if len(fake.saveCalls) != 0 || fake.address != "old:8080" {
		t.Fatalf("leaving with a blank address wrote: %+v, address=%q", fake.saveCalls, fake.address)
	}
	if app.settings.notice.kind != noticeSettingsProxyAddressEmpty {
		t.Fatalf("leave notice = %+v, want the proxy address notice", app.settings.notice)
	}

	// The other modes can save without an address.
	for _, mode := range []proxyMode{proxyOff, proxySystem} {
		fake, s := proxyFixture()
		s.proxyMode, s.proxyAddress = mode, ""
		if err := s.commitProxy(); err != nil {
			t.Fatalf("%v commit with no address: %v", mode, err)
		}
		if len(fake.saveCalls) != 1 || fake.saveCalls[0].mode != mode {
			t.Fatalf("%v saved = %+v", mode, fake.saveCalls)
		}
	}
}

func TestProxyFormTypingDoesNotWrite(t *testing.T) {
	fake, s := proxyFixture()
	s.proxyAddress, s.proxyUser, s.proxyPassword = "new:8080", "alice", "newpw"
	if !s.hasProxyChanges() {
		t.Fatal("edited draft was not detected")
	}
	if len(fake.saveCalls) != 0 || fake.address != "old:8080" || fake.password != "oldpw" {
		t.Fatalf("typing wrote: calls=%+v address=%q password=%q", fake.saveCalls, fake.address, fake.password)
	}
}

func TestProxyFormCommitsOnPageLeave(t *testing.T) {
	fake, s := proxyFixture()
	app := &appState{page: pageSettings, settings: s}
	app.settings.proxyAddress = "new:8080"
	app.settings.proxyPassword = "newpw"
	app.setPage(pageLog)
	if app.page != pageLog {
		t.Fatalf("page = %v, want log", app.page)
	}
	want := proxySave{mode: proxyManual, address: "new:8080", user: "old", password: "newpw"}
	if len(fake.saveCalls) != 1 || fake.saveCalls[0] != want {
		t.Fatalf("save calls = %+v, want %+v", fake.saveCalls, want)
	}
	app.setPage(pageAccount)
	if len(fake.saveCalls) != 1 {
		t.Fatalf("untouched leave wrote again: %+v", fake.saveCalls)
	}

	app.setPage(pageSettings)
	app.settings.proxyPassword = ""
	app.setPage(pageAccount)
	if len(fake.saveCalls) != 2 || fake.saveCalls[1].password != "" {
		t.Fatalf("cleared password not committed: %+v", fake.saveCalls)
	}
	if fake.password != "" || fake.clearCalls != 1 {
		t.Fatalf("stored password = %q, clears = %d", fake.password, fake.clearCalls)
	}
}

func TestProxyFormTestUsesCommittedValues(t *testing.T) {
	fake, s := proxyFixture()
	s.proxyAddress = "draft:8080"
	s.proxyPassword = "draftpw"
	s.startProxyTest()
	if len(fake.saveCalls) != 0 {
		t.Fatalf("test wrote the draft: %+v", fake.saveCalls)
	}
	if !s.proxyTesting || s.proxyTest != proxyTestTesting {
		t.Fatalf("test did not start: testing=%v state=%v", s.proxyTesting, s.proxyTest)
	}
	select {
	case r := <-s.proxyTestResult.Ch():
		s.proxyTest = r.state
	case <-time.After(2 * time.Second):
		t.Fatal("probe queued no result")
	}
	s.proxyTesting = false
	if s.proxyTest != proxyTestOK {
		t.Fatalf("test state = %v, want ok", s.proxyTest)
	}
}

func TestProxyFormEmptyAddressSkipsTest(t *testing.T) {
	appLanguage(t, "en")
	fake, s := proxyFixture()
	s.proxyAddress = "   "
	probeCalls := 0
	s.probe = func() proxyTestOutcome {
		probeCalls++
		return proxyTestOutcome{state: proxyTestOK}
	}
	s.startProxyTest()
	if probeCalls != 0 || s.proxyTesting || s.proxyTest != proxyTestInvalid {
		t.Fatalf("empty address: probes=%d testing=%v state=%v, want 0/false/invalid", probeCalls, s.proxyTesting, s.proxyTest)
	}

	app := &appState{settings: s}
	tester := ui.NewTester(app.settingsPage, 1200, 900)
	tester.Frame()
	if !tester.HasText(locales.SettingsProxyTestInvalid()) {
		t.Errorf("no invalid address status: %v", tester.Texts())
	}
	if !tester.HasText(locales.SettingsProxyAddressEmptyHint()) {
		t.Errorf("no blank address hint: %v", tester.Texts())
	}
	if err := tester.Click(locales.SettingsProxyTest()); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if probeCalls != 0 {
		t.Fatalf("disabled Test button sent %d probes", probeCalls)
	}

	// With an address saved, the same button sends the probe again.
	s.proxyAddress = "new:8080"
	if err := s.commitProxy(); err != nil {
		t.Fatal(err)
	}
	if fake.address != "new:8080" {
		t.Fatalf("address after commit = %q", fake.address)
	}
	s.startProxyTest()
	if !s.proxyTesting || s.proxyTest != proxyTestTesting {
		t.Fatalf("after filling address: testing=%v state=%v", s.proxyTesting, s.proxyTest)
	}
	select {
	case r := <-s.proxyTestResult.Ch():
		s.proxyTest = r.state
	case <-time.After(2 * time.Second):
		t.Fatal("probe queued no result")
	}
	if probeCalls != 1 || s.proxyTest != proxyTestOK {
		t.Fatalf("after filling address: probes=%d state=%v", probeCalls, s.proxyTest)
	}
}

func TestProxySystemHintShowsFallback(t *testing.T) {
	appLanguage(t, "en")
	app := &appState{}
	app.settings.proxyMode = proxySystem
	app.settings.proxy = &fakeProxyPref{mode: proxySystem, supported: true}
	tester := ui.NewTester(app.settingsPage, 1200, 900)
	tester.Frame()
	if !tester.HasText(locales.SettingsProxySystemHint()) {
		t.Errorf("no System fallback hint: %v", tester.Texts())
	}
}

func TestProxyTestRowFollowsProxyMode(t *testing.T) {
	appLanguage(t, "en")
	for _, tc := range []struct {
		name string
		mode proxyMode
		want bool
	}{
		{"off", proxyOff, false},
		{"manual", proxyManual, true},
		{"system", proxySystem, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &appState{}
			app.settings.proxyMode = tc.mode
			app.settings.proxy = &fakeProxyPref{mode: tc.mode, supported: true}
			app.settings.serverURL = func() string { return "https://vault.example.com" }
			app.settings.proxyTest = proxyTestOK
			tester := ui.NewTester(app.settingsPage, 1200, 900)
			tester.Frame()

			_, found := tester.Find(locales.SettingsProxyTest())
			if found != tc.want {
				t.Errorf("Test connection button found = %v, want %v", found, tc.want)
			}
			if got := tester.HasText(locales.SettingsProxyTestOK("vault.example.com")); got != tc.want {
				t.Errorf("test result text found = %v, want %v", got, tc.want)
			}
		})
	}
}

// The idle line must name the server the probe will contact, not just say
// that nothing has been tested yet.
func TestProxyTestIdleNamesServerHost(t *testing.T) {
	appLanguage(t, "en")
	app := &appState{}
	app.settings.proxyMode = proxyManual
	app.settings.proxy = &fakeProxyPref{mode: proxyManual, supported: true}
	app.settings.serverURL = func() string { return "https://api.bitwarden.com:443/web" }
	tester := ui.NewTester(app.settingsPage, 1200, 900)
	tester.Frame()

	want := locales.SettingsProxyTestIdle("api.bitwarden.com")
	if !tester.HasText(want) {
		t.Errorf("idle text %q not found", want)
	}
	if got := app.settings.proxyTestHost(); got != "api.bitwarden.com" {
		t.Errorf("probe host = %q, want api.bitwarden.com", got)
	}
}

func TestProxyTestLabelFallsBackWithoutHost(t *testing.T) {
	appLanguage(t, "en")
	app := &appState{}
	app.settings.serverURL = func() string { return "http://bad host" }
	if got := app.settings.proxyTestHost(); got != "" {
		t.Errorf("unparseable host = %q, want empty", got)
	}
	if got := proxyTestOK.label(app.settings.proxyTestHost()); got != "The server answered through the proxy; sign-in is not checked." {
		t.Errorf("fallback label = %q", got)
	}
}

func TestProxyProbeClassification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		err    error
		want   proxyTestState
	}{
		{"200", 200, nil, proxyTestOK},
		{"other status still reached the server", 404, nil, proxyTestOK},
		{"407", 407, nil, proxyTestRefused},
		{"connect 407", 0, fmt.Errorf("%w: proxyconnect tcp: 407 Proxy Authentication Required", auth.ErrNetwork), proxyTestRefused},
		{"tunnel refused", 0, fmt.Errorf("%w: proxyconnect tcp: tunnel forbidden", auth.ErrNetwork), proxyTestRefused},
		{"dead proxy", 0, fmt.Errorf("%w: dial tcp: connection refused", auth.ErrNetwork), proxyTestUnreachable},
		{"missing address", 0, fmt.Errorf("%w: %s", auth.ErrNetwork, auth.ErrNoProxyAddress), proxyTestInvalid},
	}
	for _, tc := range cases {
		if got := classifyProxyProbe(tc.status, tc.err); got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestProxySaveDrivesAuthTransport checks the real seam wiring: the values a
// Save wrote are what auth's shared transport uses on the next request. The
// proxy and target are local test servers, so no real network is involved.
func TestProxySaveDrivesAuthTransport(t *testing.T) {
	var proxyHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{}")
	}))
	defer target.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		if !strings.HasPrefix(r.UserAgent(), "Bitwarden/desktop") {
			t.Errorf("proxy saw User-Agent %q", r.UserAgent())
		}
		fmt.Fprint(w, "{}")
	}))
	defer proxy.Close()

	fake := &fakeProxyPref{supported: true}
	fake.Save(proxyManual, strings.TrimPrefix(proxy.URL, "http://"), "alice", "pw")
	wireProxySources(fake)
	t.Cleanup(func() { auth.ConfigureProxy(nil, nil) })

	cfg, err := auth.NewServerConfig(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	status, err := auth.NewClient(cfg, "proxy-check").Probe(context.Background(), cfg.API()+"/api/config")
	if err != nil || status != 200 {
		t.Fatalf("probe = %d, %v; want 200, nil", status, err)
	}
	if proxyHits.Load() != 1 {
		t.Fatalf("proxy hits = %d, want 1", proxyHits.Load())
	}
}
