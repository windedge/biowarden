package ui

import (
	"errors"

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
}

// newRealBrowsers wraps a registrar; nil selects the real HKCU-backed one, so
// tests inject an in-memory double through the same constructor.
func newRealBrowsers(reg registry.Registrar) *realBrowsers {
	if reg == nil {
		reg = registry.New()
	}
	return &realBrowsers{reg: reg}
}

func (b *realBrowsers) refresh() ([]browserRow, error) {
	rows := make([]browserRow, 0, len(registry.Browsers))
	for _, br := range registry.Browsers {
		st, err := b.reg.Status(br)
		if err != nil {
			return nil, err
		}
		rows = append(rows, browserRow{
			browser: br,
			name:    br.String(),
			path:    st.Path,
			state:   browserStateOf(st),
		})
	}
	return rows, nil
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
