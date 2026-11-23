package registry

// Proxy preference. The username is not a secret, so it lives beside the mode
// and address under HKCU\Software\biowarden; the password belongs in the
// credential store instead.

// ProxyMode is one of the four ways the GUI can reach the network.
type ProxyMode string

const (
	ProxyModeSystem      ProxyMode = "system"
	ProxyModeEnvironment ProxyMode = "environment"
	ProxyModeManual      ProxyMode = "manual"
	ProxyModeOff         ProxyMode = "off"
)

// ParseProxyMode reads a stored value. An unknown value is the system proxy,
// so a hand-edited or half-written key cannot select a mode the app cannot
// name.
func ParseProxyMode(value string) ProxyMode {
	switch ProxyMode(value) {
	case ProxyModeSystem, ProxyModeEnvironment, ProxyModeManual, ProxyModeOff:
		return ProxyMode(value)
	}
	return ProxyModeSystem
}

// ProxyPref remembers the proxy choice made in Settings. Reads and writes are
// best-effort by contract: a stored preference must never block a launch.
type ProxyPref interface {
	// Mode reports the stored choice, the system proxy when there is none.
	Mode() ProxyMode
	// Address reports the Manual proxy URL, empty when unset.
	Address() string
	// User reports the Manual proxy username, empty when unset.
	User() string
	// Set records the mode, address and username together.
	Set(mode ProxyMode, address, user string)
	// Reset removes all three values, restoring the defaults.
	Reset()
}
