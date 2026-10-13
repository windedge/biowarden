package ui

import (
	"errors"
	"testing"

	"github.com/windedge/biowarden/internal/registry"
)

// fakeRegistrar is an in-memory registry.Registrar: it keeps the tests off the
// real HKCU and records exactly which calls the backend made.
type fakeRegistrar struct {
	status      map[registry.Browser]registry.Status
	installed   []registry.Browser
	uninstalled []registry.Browser

	statusErr  error
	installErr error
}

func newFakeRegistrar() *fakeRegistrar {
	return &fakeRegistrar{status: map[registry.Browser]registry.Status{}}
}

func (f *fakeRegistrar) Status(b registry.Browser) (registry.Status, error) {
	if f.statusErr != nil {
		return registry.Status{}, f.statusErr
	}
	return f.status[b], nil
}

func (f *fakeRegistrar) Install(b registry.Browser) error {
	if f.installErr != nil {
		return f.installErr
	}
	f.installed = append(f.installed, b)
	f.status[b] = registry.Status{
		Registered: true,
		Path:       `C:\biowarden\manifests\` + string(b) + ".json",
		IsOurs:     true,
	}
	return nil
}

func (f *fakeRegistrar) Uninstall(b registry.Browser) error {
	// Mirrors the real registrar: a value that is not ours is left alone.
	if st := f.status[b]; st.Registered && st.IsOurs {
		f.uninstalled = append(f.uninstalled, b)
		delete(f.status, b)
	}
	return nil
}

func TestRealBrowsersRefreshMapsStatus(t *testing.T) {
	fake := newFakeRegistrar()
	fake.status[registry.Edge] = registry.Status{Registered: true, Path: `C:\official\edge.json`}
	fake.status[registry.Firefox] = registry.Status{Registered: true, Path: `C:\ours\firefox.json`, IsOurs: true}

	rows, err := newRealBrowsers(fake).refresh()
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	want := []browserRow{
		{browser: registry.Chrome, name: "Chrome", state: browserMissing},
		{browser: registry.Edge, name: "Edge", path: `C:\official\edge.json`, state: browserForeign},
		{browser: registry.Firefox, name: "Firefox", path: `C:\ours\firefox.json`, state: browserOurs},
	}
	if len(rows) != len(want) {
		t.Fatalf("refresh returned %d rows, want %d", len(rows), len(want))
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

func TestRealBrowsersInstallTargetsTheRowBrowser(t *testing.T) {
	fake := newFakeRegistrar()
	backend := newRealBrowsers(fake)

	rows, err := backend.refresh()
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	// The page already confirmed; the backend just forwards the row's browser.
	if err := backend.install(rows[1]); err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(fake.installed) != 1 || fake.installed[0] != registry.Edge {
		t.Fatalf("registrar.Install calls = %v, want [edge]", fake.installed)
	}
}

func TestRealBrowsersInstallPropagatesError(t *testing.T) {
	fake := newFakeRegistrar()
	fake.installErr = errors.New("access denied")
	backend := newRealBrowsers(fake)

	row := browserRow{browser: registry.Chrome, name: "Chrome", state: browserMissing}
	if err := backend.install(row); !errors.Is(err, fake.installErr) {
		t.Fatalf("install error = %v, want %v", err, fake.installErr)
	}
}

func TestRealBrowsersUninstallOursDeletes(t *testing.T) {
	fake := newFakeRegistrar()
	fake.status[registry.Chrome] = registry.Status{Registered: true, Path: `C:\ours\chrome.json`, IsOurs: true}
	backend := newRealBrowsers(fake)

	row := browserRow{browser: registry.Chrome, name: "Chrome", path: `C:\ours\chrome.json`, state: browserOurs}
	if err := backend.uninstall(row); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(fake.uninstalled) != 1 || fake.uninstalled[0] != registry.Chrome {
		t.Fatalf("registrar.Uninstall calls = %v, want [chrome]", fake.uninstalled)
	}
	if _, ok := fake.status[registry.Chrome]; ok {
		t.Fatalf("our registration survived uninstall")
	}
}

func TestRealBrowsersUninstallForeignIsRefused(t *testing.T) {
	fake := newFakeRegistrar()
	fake.status[registry.Edge] = registry.Status{Registered: true, Path: `C:\official\edge.json`}
	backend := newRealBrowsers(fake)

	row := browserRow{browser: registry.Edge, name: "Edge", path: `C:\official\edge.json`, state: browserForeign}
	err := backend.uninstall(row)
	if !errors.Is(err, errForeignRegistration) {
		t.Fatalf("uninstall foreign = %v, want errForeignRegistration", err)
	}
	if len(fake.uninstalled) != 0 {
		t.Fatalf("registrar.Uninstall was called for a foreign value: %v", fake.uninstalled)
	}
	if st := fake.status[registry.Edge]; !st.Registered {
		t.Fatalf("foreign registration was removed")
	}
}

func TestRealBrowsersRefreshPropagatesError(t *testing.T) {
	fake := newFakeRegistrar()
	fake.statusErr = errors.New("cannot open key")
	if _, err := newRealBrowsers(fake).refresh(); !errors.Is(err, fake.statusErr) {
		t.Fatalf("refresh error = %v, want %v", err, fake.statusErr)
	}
}

// fakeBrowsersBackend drives the page state without a registrar, so the notice
// a refused uninstall produces is covered too.
type fakeBrowsersBackend struct {
	rows         []browserRow
	refreshErr   error
	installErr   error
	uninstallErr error
}

func (f *fakeBrowsersBackend) refresh() ([]browserRow, error) {
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	return f.rows, nil
}

func (f *fakeBrowsersBackend) install(browserRow) error { return f.installErr }

func (f *fakeBrowsersBackend) uninstall(browserRow) error { return f.uninstallErr }

func TestBrowsersStateUninstallForeignReportsKept(t *testing.T) {
	backend := &fakeBrowsersBackend{
		rows:         []browserRow{{browser: registry.Edge, name: "Edge", path: `C:\official\edge.json`, state: browserForeign}},
		uninstallErr: errForeignRegistration,
	}
	s := newBrowsersStateWith(backend)
	s.uninstall(&s.rows[0])
	if s.noticeOK {
		t.Fatalf("noticeOK = true for a refused uninstall")
	}
	if s.notice == "" {
		t.Fatalf("refused uninstall produced no notice")
	}
}
