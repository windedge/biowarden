//go:build windows

package sysproxy

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const internetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

type wininetReader struct{ root registry.Key }

// NewReader returns the production reader.
func NewReader() Reader { return NewWindowsReader() }

// NewWindowsReader reads the current user's WinINET proxy settings.
func NewWindowsReader() Reader { return NewWindowsReaderWithRoot(registry.CURRENT_USER) }

// NewWindowsReaderWithRoot is NewWindowsReader against an injected root, which
// is how tests keep off the real HKCU.
func NewWindowsReaderWithRoot(root registry.Key) Reader { return &wininetReader{root: root} }

// Supported reports that this platform has a system proxy reader.
func Supported() bool { return true }

func (r *wininetReader) System() (Config, bool) {
	key, err := registry.OpenKey(r.root, internetSettingsPath, registry.QUERY_VALUE)
	if err != nil {
		return Config{}, false
	}
	defer key.Close()

	enabled, kind, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || kind != registry.DWORD || enabled == 0 {
		return Config{}, false
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil || strings.TrimSpace(server) == "" {
		return Config{}, false
	}
	cfg := ParseProxyServer(server)
	if override, _, err := key.GetStringValue("ProxyOverride"); err == nil {
		cfg.Override = override
	}
	return cfg, true
}
