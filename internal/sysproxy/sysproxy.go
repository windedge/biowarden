// Package sysproxy is the seam for reading the OS-configured proxy.
package sysproxy

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ErrUnsupported is the reason a platform has no system proxy reader; Supported
// exposes it as a capability so callers need no runtime.GOOS checks.
var ErrUnsupported = errors.New("sysproxy: system proxy reading is not supported on this platform")

// Config is a resolved system proxy setting.
type Config struct {
	HTTP     string // host:port or full URL, empty if none
	HTTPS    string
	SOCKS    string
	Override string // WinINET bypass list; recorded only, never applied as exclusion rules
}

// Reader reports the OS-configured proxy.
type Reader interface {
	// System returns the OS-configured proxy. ok=false when the OS has no proxy configured.
	System() (Config, bool)
}

// ParseProxyServer interprets a non-empty WinINET ProxyServer value. A bare
// address covers HTTP and HTTPS; per-protocol entries may omit https, in which
// case WinINET reuses the http entry.
func ParseProxyServer(raw string) Config {
	var cfg Config
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cfg
	}
	if !strings.Contains(raw, "=") {
		addr := normalizeAddress(raw)
		cfg.HTTP, cfg.HTTPS = addr, addr
		return cfg
	}

	for _, part := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "http":
			cfg.HTTP = normalizeAddress(value)
		case "https":
			cfg.HTTPS = normalizeAddress(value)
		case "socks":
			cfg.SOCKS = normalizeAddress(value)
		}
	}
	if cfg.HTTPS == "" {
		cfg.HTTPS = cfg.HTTP
	}
	return cfg
}

func normalizeAddress(raw string) string {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		return ""
	}
	if strings.Contains(addr, "://") {
		if u, err := url.Parse(addr); err == nil && u.Host != "" {
			addr = u.Host
		} else {
			addr = addr[strings.Index(addr, "://")+len("://"):]
		}
	}
	// SplitHostPort keeps bracketed IPv6 hosts from losing their port.
	if host, port, err := net.SplitHostPort(addr); err == nil {
		return net.JoinHostPort(host, port)
	}
	return addr
}
