package registry

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

type hkcuMarker struct{ root registry.Key }

// NewMarker returns the HKCU backed first-run marker.
func NewMarker() Marker { return NewMarkerWithRoot(registry.CURRENT_USER) }

// NewMarkerWithRoot is NewMarker against an injected root, which is how tests
// keep off the real HKCU.
func NewMarkerWithRoot(root registry.Key) Marker { return &hkcuMarker{root: root} }

func (m *hkcuMarker) Done() (bool, error) {
	key, err := registry.OpenKey(m.root, firstRunKeyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("registry: cannot open %s: %w", firstRunKeyPath, err)
	}
	defer key.Close()

	if _, _, err := key.GetIntegerValue(firstRunValueName); err != nil {
		// A missing value and a value of the wrong type both mean the setup
		// was never completed here; Mark overwrites the stray value, so the
		// launch must not fail over it.
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, registry.ErrUnexpectedType) {
			return false, nil
		}
		return false, fmt.Errorf("registry: cannot read %s: %w", firstRunValueName, err)
	}
	return true, nil
}

func (m *hkcuMarker) Mark() error {
	key, _, err := registry.CreateKey(m.root, firstRunKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("registry: cannot create %s: %w", firstRunKeyPath, err)
	}
	defer key.Close()

	if err := key.SetDWordValue(firstRunValueName, 1); err != nil {
		return fmt.Errorf("registry: cannot write %s: %w", firstRunValueName, err)
	}
	return nil
}
