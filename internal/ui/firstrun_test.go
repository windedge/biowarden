package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/windedge/biowarden/internal/registry"
)

// fakeFirstRun is an in-memory firstRunBackend: it records the calls the setup
// made, so a test can prove what it registered, what it skipped, and where it
// stopped.
type fakeFirstRun struct {
	completed bool
	doneErr   error

	markCalls int
	markErr   error

	keys    int
	keysErr error

	detected    []registry.Browser
	statuses    map[registry.Browser]registry.Status
	statusErr   error
	installs    []registry.Browser
	installErrs map[registry.Browser]error
}

func newFakeFirstRun() *fakeFirstRun {
	return &fakeFirstRun{
		statuses:    map[registry.Browser]registry.Status{},
		installErrs: map[registry.Browser]error{},
	}
}

func (f *fakeFirstRun) done() (bool, error) { return f.completed, f.doneErr }

func (f *fakeFirstRun) mark() error {
	f.markCalls++
	return f.markErr
}

func (f *fakeFirstRun) storedKeys() (int, error) { return f.keys, f.keysErr }

func (f *fakeFirstRun) installed() []registry.Browser { return f.detected }

func (f *fakeFirstRun) status(b registry.Browser) (registry.Status, error) {
	if f.statusErr != nil {
		return registry.Status{}, f.statusErr
	}
	return f.statuses[b], nil
}

func (f *fakeFirstRun) install(b registry.Browser) error {
	if err := f.installErrs[b]; err != nil {
		return err
	}
	f.installs = append(f.installs, b)
	return nil
}

func TestNeedsFirstRun(t *testing.T) {
	tests := []struct {
		name string
		keys int
		ours bool
		want bool
	}{
		{"zero configuration", 0, false, true},
		{"a key is stored", 1, false, false},
		{"a browser is already ours", 0, true, false},
		{"both", 2, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsFirstRun(tt.keys, tt.ours); got != tt.want {
				t.Fatalf("needsFirstRun(%d, %v) = %v, want %v", tt.keys, tt.ours, got, tt.want)
			}
		})
	}
}

// The aggregate only counts a browser that is really ours, and a failing read
// must not count as one.
func TestAnyRegisteredOurs(t *testing.T) {
	tests := []struct {
		name   string
		status registry.Status
		err    error
		want   bool
	}{
		{"missing", registry.Status{}, nil, false},
		{"foreign", registry.Status{Registered: true, Path: `C:\official\chrome.json`}, nil, false},
		{"hijacked", registry.Status{Registered: true, Path: `C:\official\chrome.json`, OursInstalled: true}, nil, false},
		{"ours", registry.Status{Registered: true, IsOurs: true}, nil, true},
		{"read failure", registry.Status{Registered: true, IsOurs: true}, errors.New("access denied"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := anyRegisteredOurs(func(registry.Browser) (registry.Status, error) {
				return tt.status, tt.err
			})
			if got != tt.want {
				t.Fatalf("anyRegisteredOurs = %v, want %v", got, tt.want)
			}
		})
	}
}

// A first run that is really one registers what it can, reports the
// registrations that may belong to the official client instead of overwriting
// them, and leaves the marker behind.
func TestFirstRunSetupSkipsForeignAndHijacked(t *testing.T) {
	backend := newFakeFirstRun()
	backend.detected = []registry.Browser{registry.Edge, registry.Chrome, registry.Firefox}
	backend.statuses[registry.Edge] = registry.Status{Registered: true, Path: `C:\official\edge.json`}
	backend.statuses[registry.Firefox] = registry.Status{
		Registered:    true,
		Path:          `C:\official\firefox.json`,
		OursInstalled: true,
	}

	result, firstRun := newFirstRunState(backend).setup()
	if !firstRun {
		t.Fatal("setup = false for a zero-configuration launch")
	}
	if len(backend.installs) != 1 || backend.installs[0] != registry.Chrome {
		t.Fatalf("installs = %v, want [chrome]", backend.installs)
	}
	if len(result.registered) != 1 || result.registered[0] != "Chrome" {
		t.Fatalf("registered = %v, want [Chrome]", result.registered)
	}
	if len(result.skipped) != 2 || result.skipped[0] != "Edge" || result.skipped[1] != "Firefox" {
		t.Fatalf("skipped = %v, want [Edge Firefox]", result.skipped)
	}
	if len(result.absent) != 0 || len(result.failed) != 0 || result.markErr != nil {
		t.Fatalf("result = %+v, want no absent, failed or mark error", result)
	}
	if backend.markCalls != 1 {
		t.Fatalf("mark calls = %d, want 1", backend.markCalls)
	}

	notice, ok := result.notice()
	for _, want := range []string{"已注册 1 个浏览器（Chrome）", "跳过 2 个", "Edge、Firefox"} {
		if !strings.Contains(notice, want) {
			t.Fatalf("notice %q does not contain %q", notice, want)
		}
	}
	if ok {
		t.Fatalf("notice %q was marked as good news despite the skips", notice)
	}
}

// Only installed browsers are touched, and the ones the probe did not find are
// named so the summary explains their absence.
func TestFirstRunSetupRegistersOnlyInstalledBrowsers(t *testing.T) {
	backend := newFakeFirstRun()
	backend.detected = []registry.Browser{registry.Edge}

	result, firstRun := newFirstRunState(backend).setup()
	if !firstRun {
		t.Fatal("setup = false for a zero-configuration launch")
	}
	if len(backend.installs) != 1 || backend.installs[0] != registry.Edge {
		t.Fatalf("installs = %v, want [edge]", backend.installs)
	}
	wantAbsent := []string{"Chrome", "Firefox"}
	if len(result.absent) != len(wantAbsent) {
		t.Fatalf("absent = %v, want %v", result.absent, wantAbsent)
	}
	for i := range wantAbsent {
		if result.absent[i] != wantAbsent[i] {
			t.Fatalf("absent = %v, want %v", result.absent, wantAbsent)
		}
	}
	notice, ok := result.notice()
	if !ok {
		t.Fatalf("notice %q was not good news, but nothing needs a human", notice)
	}
	if !strings.Contains(notice, "未安装：Chrome、Firefox") {
		t.Fatalf("notice %q does not name the missing browsers", notice)
	}
}

// Every outcome has a place in the one summary line.
func TestFirstRunNoticeRendersEveryOutcome(t *testing.T) {
	result := firstRunResult{
		registered: []string{"Chrome", "Edge"},
		skipped:    []string{"Firefox"},
		failed:     []string{"Edge"},
		absent:     []string{"Chrome"},
		markErr:    errors.New("access denied"),
	}
	notice, ok := result.notice()
	for _, want := range []string{
		"首启自动配置",
		"已注册 2 个浏览器（Chrome、Edge）",
		"跳过 1 个（注册可能属于官方客户端，未覆盖：Firefox）",
		"失败：Edge",
		"未安装：Chrome",
		"首启标记异常：access denied",
	} {
		if !strings.Contains(notice, want) {
			t.Fatalf("notice %q does not contain %q", notice, want)
		}
	}
	if ok {
		t.Fatal("a result with skips and failures was marked as good news")
	}
}

func TestFirstRunSetupStopsWhenMarkerIsDone(t *testing.T) {
	backend := newFakeFirstRun()
	backend.completed = true
	backend.detected = []registry.Browser{registry.Chrome}

	_, firstRun := newFirstRunState(backend).setup()
	if firstRun {
		t.Fatal("setup ran again after the marker was written")
	}
	if len(backend.installs) != 0 || backend.markCalls != 0 {
		t.Fatalf("installs = %v, mark calls = %d; want none", backend.installs, backend.markCalls)
	}
}

func TestFirstRunSetupStopsWhenAKeyIsStored(t *testing.T) {
	backend := newFakeFirstRun()
	backend.keys = 1
	backend.detected = []registry.Browser{registry.Chrome}

	_, firstRun := newFirstRunState(backend).setup()
	if firstRun {
		t.Fatal("setup ran on a machine that already has a stored key")
	}
	if len(backend.installs) != 0 || backend.markCalls != 0 {
		t.Fatalf("installs = %v, mark calls = %d; want none", backend.installs, backend.markCalls)
	}
}

// An existing registration of ours is not a first run, and it is not reported
// as an absence either.
func TestFirstRunSetupStopsWhenOurRegistrationExists(t *testing.T) {
	backend := newFakeFirstRun()
	backend.detected = []registry.Browser{registry.Chrome}
	backend.statuses[registry.Chrome] = registry.Status{Registered: true, IsOurs: true}

	_, firstRun := newFirstRunState(backend).setup()
	if firstRun {
		t.Fatal("setup ran on a machine already registered to us")
	}
	if len(backend.installs) != 0 || backend.markCalls != 0 {
		t.Fatalf("installs = %v, mark calls = %d; want none", backend.installs, backend.markCalls)
	}
}

// Without the credential store's answer the launch cannot be told apart from a
// first one, so the setup stays quiet instead of guessing.
func TestFirstRunSetupStopsWhenStoredKeysCannotBeRead(t *testing.T) {
	backend := newFakeFirstRun()
	backend.keysErr = errors.New("credential store unavailable")
	backend.detected = []registry.Browser{registry.Chrome}

	_, firstRun := newFirstRunState(backend).setup()
	if firstRun {
		t.Fatal("setup ran without knowing whether keys are stored")
	}
	if len(backend.installs) != 0 || backend.markCalls != 0 {
		t.Fatalf("installs = %v, mark calls = %d; want none", backend.installs, backend.markCalls)
	}
}

// A failed write is reported, does not stop the other browsers, and does not
// stop the marker.
func TestFirstRunSetupReportsFailuresAndStillMarks(t *testing.T) {
	backend := newFakeFirstRun()
	backend.detected = []registry.Browser{registry.Chrome, registry.Edge}
	backend.installErrs[registry.Edge] = errors.New("access denied")

	result, firstRun := newFirstRunState(backend).setup()
	if !firstRun {
		t.Fatal("setup = false for a zero-configuration launch")
	}
	if len(backend.installs) != 1 || backend.installs[0] != registry.Chrome {
		t.Fatalf("installs = %v, want [chrome]", backend.installs)
	}
	if len(result.failed) != 1 || result.failed[0] != "Edge" {
		t.Fatalf("failed = %v, want [Edge]", result.failed)
	}
	if backend.markCalls != 1 {
		t.Fatalf("mark calls = %d, want 1 even after a failed write", backend.markCalls)
	}
	notice, ok := result.notice()
	if ok || !strings.Contains(notice, "失败：Edge") {
		t.Fatalf("notice = %q ok = %v, want a visible failure", notice, ok)
	}
}

// A marker that cannot be read or written is reported but never blocks the
// setup.
func TestFirstRunSetupReportsMarkerFailure(t *testing.T) {
	tests := []struct {
		name    string
		doneErr error
		markErr error
		want    string
	}{
		{"read failure", errors.New("cannot open key"), nil, "首启标记异常：cannot open key"},
		{"write failure", nil, errors.New("cannot write value"), "首启标记异常：cannot write value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newFakeFirstRun()
			backend.detected = []registry.Browser{registry.Chrome}
			// A read failure is treated as "not done yet", so the setup runs.
			backend.doneErr = tt.doneErr
			backend.markErr = tt.markErr

			result, firstRun := newFirstRunState(backend).setup()
			if !firstRun {
				t.Fatal("setup = false although this launch could not read the marker")
			}
			if len(backend.installs) != 1 {
				t.Fatalf("installs = %v, want [chrome]", backend.installs)
			}
			notice, ok := result.notice()
			if ok || !strings.Contains(notice, tt.want) {
				t.Fatalf("notice = %q ok = %v, want %q visible", notice, ok, tt.want)
			}
		})
	}
}

// newFirstRunApp is an app with the pieces the launch path needs: the
// first-run state, a login window whose seams count their calls, and a place
// to leave the browser notice.
func newFirstRunApp(t *testing.T, backend firstRunBackend) (*appState, *int) {
	t.Helper()
	page := newAccountStateWith(fakeKeys{}, &stubAccount{sess: loggedOutSession()})
	waitRestore(t, &page)

	app := &appState{firstRun: newFirstRunState(backend)}
	app.account = page
	app.login = newLoginWindow(&app.account)
	opened := 0
	app.login.openWindow = func() { opened++ }
	app.login.closeWindow = func() {}
	return app, &opened
}

// The launch path registers once and opens exactly one login window, however
// many times it is called.
func TestRunFirstRunOpensTheLoginWindowOnce(t *testing.T) {
	backend := newFakeFirstRun()
	backend.detected = []registry.Browser{registry.Chrome}
	app, opened := newFirstRunApp(t, backend)

	app.runFirstRun()
	app.runFirstRun()

	if *opened != 1 {
		t.Fatalf("openWindow calls = %d, want 1", *opened)
	}
	if !app.login.showing {
		t.Fatal("the login window is not showing after a first run")
	}
	if len(backend.installs) != 1 || backend.markCalls != 1 {
		t.Fatalf("installs = %v, mark calls = %d, want one of each", backend.installs, backend.markCalls)
	}
	if !strings.Contains(app.browsers.notice, "首启自动配置") {
		t.Fatalf("browsers notice = %q, want the first-run summary", app.browsers.notice)
	}
}

// Failures in the setup still land the human on the login form: unlocking
// never depended on the browser registration.
func TestRunFirstRunOpensLoginWindowAfterFailures(t *testing.T) {
	backend := newFakeFirstRun()
	backend.detected = []registry.Browser{registry.Chrome, registry.Edge}
	backend.statusErr = errors.New("cannot open key")
	backend.markErr = errors.New("cannot write value")
	app, opened := newFirstRunApp(t, backend)

	app.runFirstRun()

	if *opened != 1 || !app.login.showing {
		t.Fatalf("login window opened %d times, showing = %v; want it up exactly once", *opened, app.login.showing)
	}
	if app.browsers.noticeOK {
		t.Fatalf("notice %q was marked as good news despite the failures", app.browsers.notice)
	}
}

// A machine that is already configured is left completely alone on launch.
func TestRunFirstRunLeavesConfiguredMachinesAlone(t *testing.T) {
	backend := newFakeFirstRun()
	backend.completed = true
	app, opened := newFirstRunApp(t, backend)

	app.runFirstRun()

	if *opened != 0 || app.login.showing {
		t.Fatalf("login window opened on a configured machine: opened = %d, showing = %v", *opened, app.login.showing)
	}
	if app.browsers.notice != "" {
		t.Fatalf("browsers notice = %q, want none", app.browsers.notice)
	}
}
