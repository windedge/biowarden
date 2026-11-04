package ui

import (
	"errors"
	"sync"
	"time"

	"github.com/windedge/biowarden/internal/hoststate"
	"github.com/windedge/biowarden/internal/registry"
)

// errForeignRegistration reports an uninstall the registry declined because
// the value belongs to another client. The page surfaces it so a human is not
// told the official client's registration is gone when it is still there.
var errForeignRegistration = errors.New("ui: registration belongs to another client")

// realBrowsers is the registry-backed browsersBackend: it reports every
// browser's manifest state and rewrites the ones the human asks for.
type realBrowsers struct {
	reg registry.Registrar
	// connections reads each browser id's last connection time. A read failure
	// only loses the indicator, so it never fails the rows.
	connections func() (map[string]time.Time, error)
	// detect reports the browsers installed on this machine. It is cached, so
	// the App Paths probe runs at most once per backend.
	detect func() []registry.Browser
}

// newRealBrowsers wraps a registrar; nil selects the real HKCU-backed one, so
// tests inject an in-memory double through the same constructor.
func newRealBrowsers(reg registry.Registrar) *realBrowsers {
	return newRealBrowsersWith(reg, hoststate.LastConnections, registry.Detect)
}

// newRealBrowsersWith injects the connection reader and the install probe too,
// so a test never reads the tester's real profile directory or registry. A nil
// detect reports nothing, which is the safe default for a backend built
// without the real one.
func newRealBrowsersWith(reg registry.Registrar, connections func() (map[string]time.Time, error), detect func() []registry.Browser) *realBrowsers {
	if reg == nil {
		reg = registry.New()
	}
	if connections == nil {
		connections = func() (map[string]time.Time, error) { return nil, nil }
	}
	if detect == nil {
		detect = func() []registry.Browser { return nil }
	}
	return &realBrowsers{reg: reg, connections: connections, detect: cachedDetect(detect)}
}

// cachedDetect memoizes the App Paths probe. The page reloads its rows on
// every frame, but a browser install does not change while the app runs, so
// one probe per backend is enough and the registry stays untouched after it.
func cachedDetect(detect func() []registry.Browser) func() []registry.Browser {
	var (
		once      sync.Once
		installed []registry.Browser
	)
	return func() []registry.Browser {
		once.Do(func() { installed = detect() })
		return installed
	}
}

func (b *realBrowsers) refresh() ([]browserRow, error) {
	// A state read failure is not worth failing the page: the row's indicator
	// just stays at "extension never connected" until the host has something to say.
	connections, _ := b.connections()
	installed := make(map[registry.Browser]bool)
	for _, br := range b.detect() {
		installed[br] = true
	}
	rows := make([]browserRow, 0, len(registry.Browsers))
	for _, br := range registry.Browsers {
		st, err := b.reg.Status(br)
		if err != nil {
			return nil, err
		}
		self, err := b.reg.ManifestPath(br)
		if err != nil {
			return nil, err
		}
		rows = append(rows, browserRow{
			browser:         br,
			name:            br.String(),
			path:            st.Path,
			installPath:     self,
			state:           browserStateOf(st),
			installed:       installed[br],
			lastConnectedAt: lastConnectedFor(connections, br),
		})
	}
	return rows, nil
}

// browserIDs lists the host-side ids that reach one registration row. Chrome
// covers both chrome.exe and chromium.exe because install writes the Google
// and Chromium keys together; the recorder still keeps them apart, so this is
// the one place the two are deliberately merged.
func browserIDs(b registry.Browser) []string {
	switch b {
	case registry.Chrome:
		return []string{hoststate.BrowserChrome, hoststate.BrowserChromium}
	case registry.Edge:
		return []string{hoststate.BrowserEdge}
	case registry.Firefox:
		return []string{hoststate.BrowserFirefox}
	}
	return nil
}

// lastConnectedFor is the row's newest connection among its ids, zero when the
// host has never recorded any of them.
func lastConnectedFor(connections map[string]time.Time, b registry.Browser) time.Time {
	var latest time.Time
	for _, id := range browserIDs(b) {
		if at := connections[id]; at.After(latest) {
			latest = at
		}
	}
	return latest
}

// browserStateOf is the mapping the page renders: no value means missing, our
// own manifest is ours, a foreign value with our file still on disk means the
// official client overwrote us, and any other value is plain foreign.
func browserStateOf(st registry.Status) int {
	switch {
	case !st.Registered:
		return browserMissing
	case st.IsOurs:
		return browserOurs
	case st.OursInstalled:
		return browserHijacked
	default:
		return browserForeign
	}
}

func (b *realBrowsers) install(row browserRow) error {
	// The page already ran the two-step confirmation for a foreign manifest;
	// overwriting is what Install does, so no second check belongs here.
	return b.reg.Install(row.browser)
}

func (b *realBrowsers) uninstall(row browserRow) error {
	// Registrar.Uninstall only clears its own value, so a foreign one has to
	// be refused before the call for the page to say "left in place". A
	// hijacked row still has our leftover manifest file to remove, so it is
	// allowed through: the foreign key survives, our file does not.
	st, err := b.reg.Status(row.browser)
	if err != nil {
		return err
	}
	if st.Registered && !st.IsOurs && !st.OursInstalled {
		return errForeignRegistration
	}
	return b.reg.Uninstall(row.browser)
}
