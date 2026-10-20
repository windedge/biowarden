package registry

import "golang.org/x/sys/windows/registry"

type hkcuServerPref struct{ root registry.Key }

// NewServerPref returns the HKCU backed remembered server address.
func NewServerPref() ServerPref { return NewServerPrefWithRoot(registry.CURRENT_USER) }

// NewServerPrefWithRoot is NewServerPref against an injected root, which is how
// tests keep off the real HKCU.
func NewServerPrefWithRoot(root registry.Key) ServerPref { return &hkcuServerPref{root: root} }

func (p *hkcuServerPref) Last() (string, bool) {
	key, err := registry.OpenKey(p.root, lastServerKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer key.Close()

	address, _, err := key.GetStringValue(lastServerValueName)
	if err != nil || address == "" {
		// A missing value and a wrong-typed one both mean nothing was
		// remembered; Remember replaces the stray value, so the launch must
		// not fail over it.
		return "", false
	}
	return address, true
}

func (p *hkcuServerPref) Remember(address string) {
	if address == "" {
		return
	}
	key, _, err := registry.CreateKey(p.root, lastServerKeyPath, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.SetStringValue(lastServerValueName, address)
}
