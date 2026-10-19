package ui

import (
	"github.com/windedge/biowarden/internal/registry"
	"github.com/windedge/biowarden/internal/secrets"
)

// realFirstRun is the production first-run backend: the HKCU marker, the
// credential store, and the same registry seam the Browsers page writes
// through. It never writes a key or a registry value on its own.
type realFirstRun struct {
	reg    registry.Registrar
	marker registry.Marker
	store  secrets.Store
	detect func() []registry.Browser
}

// newRealFirstRun wires the real seams. detect is injected so the probe can be
// replaced in a test.
func newRealFirstRun() *realFirstRun {
	return &realFirstRun{
		reg:    registry.New(),
		marker: registry.NewMarker(),
		store:  secrets.New(),
		detect: registry.Detect,
	}
}

func (f *realFirstRun) done() (bool, error) { return f.marker.Done() }

func (f *realFirstRun) mark() error { return f.marker.Mark() }

// storedKeys reads the credential store, which is the same source the Account
// page's card shows, so "no keys stored" means the same thing in both places.
func (f *realFirstRun) storedKeys() (int, error) {
	accounts, err := f.store.List()
	if err != nil {
		return 0, err
	}
	return len(accounts), nil
}

func (f *realFirstRun) installed() []registry.Browser { return f.detect() }

func (f *realFirstRun) status(b registry.Browser) (registry.Status, error) {
	return f.reg.Status(b)
}

func (f *realFirstRun) install(b registry.Browser) error { return f.reg.Install(b) }
