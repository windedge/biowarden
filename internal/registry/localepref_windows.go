package registry

import "golang.org/x/sys/windows/registry"

type hkcuLocalePref struct{ root registry.Key }

// NewLocalePref returns the HKCU backed explicit language choice.
func NewLocalePref() LocalePref { return NewLocalePrefWithRoot(registry.CURRENT_USER) }

// NewLocalePrefWithRoot is NewLocalePref against an injected root, which is how
// tests keep off the real HKCU.
func NewLocalePrefWithRoot(root registry.Key) LocalePref { return &hkcuLocalePref{root: root} }

func (p *hkcuLocalePref) Chosen() (string, bool) {
	key, err := registry.OpenKey(p.root, localeChoiceKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer key.Close()

	tag, _, err := key.GetStringValue(localeChoiceValueName)
	if err != nil || tag == "" {
		// A missing value and a wrong-typed one both mean no choice, so a fresh
		// machine falls back to the system's languages; Remember replaces the
		// stray value, so nothing needs to fail over it.
		return "", false
	}
	return tag, true
}

func (p *hkcuLocalePref) Remember(tag string) {
	if tag == "" {
		return
	}
	key, _, err := registry.CreateKey(p.root, localeChoiceKeyPath, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.SetStringValue(localeChoiceValueName, tag)
}

func (p *hkcuLocalePref) Forget() {
	key, err := registry.OpenKey(p.root, localeChoiceKeyPath, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.DeleteValue(localeChoiceValueName)
}
