package registry

import "golang.org/x/sys/windows/registry"

const (
	appearanceKeyPath   = `Software\biowarden`
	appearanceValueName = "Appearance"
)

type hkcuAppearancePref struct{ root registry.Key }

// NewAppearancePref returns the HKCU backed appearance preference.
func NewAppearancePref() AppearancePref { return NewAppearancePrefWithRoot(registry.CURRENT_USER) }

// NewAppearancePrefWithRoot is NewAppearancePref against an injected root,
// which is how tests keep off the real HKCU.
func NewAppearancePrefWithRoot(root registry.Key) AppearancePref {
	return &hkcuAppearancePref{root: root}
}

func (p *hkcuAppearancePref) Current() Appearance {
	key, err := registry.OpenKey(p.root, appearanceKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return AppearanceSystem
	}
	defer key.Close()

	value, _, err := key.GetStringValue(appearanceValueName)
	if err != nil {
		// A missing value and a wrong-typed one both mean no choice: the
		// system's own, which Remember overwrites with a proper value.
		return AppearanceSystem
	}
	return ParseAppearance(value)
}

func (p *hkcuAppearancePref) Remember(a Appearance) {
	key, _, err := registry.CreateKey(p.root, appearanceKeyPath, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.SetStringValue(appearanceValueName, string(ParseAppearance(string(a))))
}
