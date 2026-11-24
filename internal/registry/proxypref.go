package registry

import (
	"net"
	"strings"
)

// Proxy preference. The username is not a secret, so it lives beside the mode
// and address under HKCU\Software\biowarden; the password belongs in the
// credential store instead.

// ProxyMode is one of the four ways the GUI can reach the network. The stored
// value names the type; the address holds only host:port.
type ProxyMode string

const (
	ProxyModeNone   ProxyMode = "none"
	ProxyModeHTTP   ProxyMode = "http"
	ProxyModeSOCKS5 ProxyMode = "socks5"
	ProxyModeSystem ProxyMode = "system"

	proxyModeLegacyManual      ProxyMode = "manual"
	proxyModeLegacyEnvironment ProxyMode = "environment"
	proxyModeLegacyOff         ProxyMode = "off"
)

// ParseProxyMode reads a stored mode without an address to classify a legacy
// Manual value. Callers holding the address should use
// ParseProxyModeWithAddress.
func ParseProxyMode(value string) ProxyMode {
	return ParseProxyModeWithAddress(value, "")
}

// ParseProxyModeWithAddress reads a stored mode and, for the legacy Manual
// value, derives the type from the address's scheme. Unknown values fall back
// to System so a half-written key cannot select a mode the app cannot name.
func ParseProxyModeWithAddress(value, address string) ProxyMode {
	switch ProxyMode(value) {
	case ProxyModeNone, ProxyModeHTTP, ProxyModeSOCKS5, ProxyModeSystem:
		return ProxyMode(value)
	case proxyModeLegacyManual:
		scheme := strings.ToLower(strings.TrimSpace(address))
		if i := strings.Index(scheme, "://"); i >= 0 {
			scheme = scheme[:i]
		}
		if scheme == "socks5" || scheme == "socks5h" {
			return ProxyModeSOCKS5
		}
		return ProxyModeHTTP
	case proxyModeLegacyEnvironment:
		return ProxyModeSystem
	case proxyModeLegacyOff:
		return ProxyModeNone
	default:
		return ProxyModeSystem
	}
}

// normalizeAddress strips a scheme from an address written by an older build.
func normalizeAddress(address string) string {
	if i := strings.Index(address, "://"); i >= 0 {
		return address[i+3:]
	}
	return address
}

// splitHostPort separates the stored host:port without losing an IPv6 literal.
func splitHostPort(address string) (host, port string) {
	address = normalizeAddress(strings.TrimSpace(address))
	if h, p, err := net.SplitHostPort(address); err == nil {
		return h, p
	}
	return address, ""
}

// joinHostPort writes the form ProxyURL holds: host:port with no scheme.
func joinHostPort(host, port string) string {
	host, port = strings.TrimSpace(host), strings.TrimSpace(port)
	if host == "" {
		if port == "" {
			return ""
		}
		return ":" + port
	}
	if port == "" {
		return host
	}
	return net.JoinHostPort(host, port)
}

// ProxyPref remembers the proxy choice made in Settings. Reads and writes are
// best-effort by contract: a stored preference must never block a launch.
type ProxyPref interface {
	// Mode reports the stored choice, the system proxy when there is none.
	Mode() ProxyMode
	// Host reports the Manual proxy host, empty when unset.
	Host() string
	// Port reports the Manual proxy port, empty when unset.
	Port() string
	// User reports the Manual proxy username, empty when unset.
	User() string
	// Set records the mode, host, port and username together.
	Set(mode ProxyMode, host, port, user string)
	// Reset removes all values, restoring the defaults.
	Reset()
}
