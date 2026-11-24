package registry

import "golang.org/x/sys/windows/registry"

const (
	proxyKeyPath       = `Software\biowarden`
	proxyModeValueName = "ProxyMode"
	proxyURLValueName  = "ProxyURL"
	proxyUserValueName = "ProxyUser"
)

type hkcuProxyPref struct{ root registry.Key }

// NewProxyPref returns the HKCU backed proxy preference.
func NewProxyPref() ProxyPref { return NewProxyPrefWithRoot(registry.CURRENT_USER) }

// NewProxyPrefWithRoot is NewProxyPref against an injected root, which is how
// tests keep off the real HKCU.
func NewProxyPrefWithRoot(root registry.Key) ProxyPref {
	return &hkcuProxyPref{root: root}
}

func (p *hkcuProxyPref) readString(name string) string {
	key, err := registry.OpenKey(p.root, proxyKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}

func (p *hkcuProxyPref) Mode() ProxyMode {
	return ParseProxyModeWithAddress(p.readString(proxyModeValueName), p.readString(proxyURLValueName))
}

func (p *hkcuProxyPref) Host() string {
	host, _ := splitHostPort(p.readString(proxyURLValueName))
	return host
}

func (p *hkcuProxyPref) Port() string {
	_, port := splitHostPort(p.readString(proxyURLValueName))
	return port
}

func (p *hkcuProxyPref) User() string { return p.readString(proxyUserValueName) }

func (p *hkcuProxyPref) Set(mode ProxyMode, host, port, user string) {
	key, _, err := registry.CreateKey(p.root, proxyKeyPath, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	// One key holds all values: a partially written choice is a corrupted
	// choice, so write them together and ignore individual failures.
	_ = key.SetStringValue(proxyModeValueName, string(ParseProxyModeWithAddress(string(mode), host)))
	_ = key.SetStringValue(proxyURLValueName, joinHostPort(host, port))
	_ = key.SetStringValue(proxyUserValueName, user)
}

func (p *hkcuProxyPref) Reset() {
	key, err := registry.OpenKey(p.root, proxyKeyPath, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.DeleteValue(proxyModeValueName)
	_ = key.DeleteValue(proxyURLValueName)
	_ = key.DeleteValue(proxyUserValueName)
}
