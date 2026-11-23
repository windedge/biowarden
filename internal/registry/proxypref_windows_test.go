package registry

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestProxyPrefDefaults(t *testing.T) {
	pref := NewProxyPrefWithRoot(newTestRoot(t))
	if got := pref.Mode(); got != ProxyModeSystem {
		t.Fatalf("Mode on a fresh root = %q, want %q", got, ProxyModeSystem)
	}
	if got := pref.Address(); got != "" {
		t.Fatalf("Address on a fresh root = %q, want empty", got)
	}
	if got := pref.User(); got != "" {
		t.Fatalf("User on a fresh root = %q, want empty", got)
	}
}

func TestProxyPrefRoundTrip(t *testing.T) {
	root := newTestRoot(t)
	pref := NewProxyPrefWithRoot(root)
	pref.Set(ProxyModeManual, "socks5://127.0.0.1:1080", "alice")

	if got := pref.Mode(); got != ProxyModeManual {
		t.Errorf("Mode = %q, want %q", got, ProxyModeManual)
	}
	if got := pref.Address(); got != "socks5://127.0.0.1:1080" {
		t.Errorf("Address = %q, want the stored URL", got)
	}
	if got := pref.User(); got != "alice" {
		t.Errorf("User = %q, want alice", got)
	}

	pref.Set(ProxyModeOff, "", "")
	if got := pref.Mode(); got != ProxyModeOff {
		t.Errorf("Mode after overwrite = %q, want %q", got, ProxyModeOff)
	}
	if got := pref.Address(); got != "" {
		t.Errorf("Address after overwrite = %q, want empty", got)
	}
	if got := pref.User(); got != "" {
		t.Errorf("User after overwrite = %q, want empty", got)
	}
}

func TestProxyPrefReset(t *testing.T) {
	root := newTestRoot(t)
	pref := NewProxyPrefWithRoot(root)
	pref.Set(ProxyModeManual, "http://proxy:8080", "alice")
	pref.Reset()

	if got := pref.Mode(); got != ProxyModeSystem {
		t.Errorf("Mode after Reset = %q, want %q", got, ProxyModeSystem)
	}
	if got := pref.Address(); got != "" {
		t.Errorf("Address after Reset = %q, want empty", got)
	}
	if got := pref.User(); got != "" {
		t.Errorf("User after Reset = %q, want empty", got)
	}

	key, err := registry.OpenKey(root, proxyKeyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("open key: %v", err)
	}
	defer key.Close()
	for _, name := range []string{proxyModeValueName, proxyURLValueName, proxyUserValueName} {
		if _, _, err := key.GetStringValue(name); err == nil {
			t.Errorf("value %s survived Reset", name)
		}
	}

	pref.Reset()
	if got := pref.Mode(); got != ProxyModeSystem {
		t.Errorf("Mode after a second Reset = %q, want %q", got, ProxyModeSystem)
	}
}

func TestProxyPrefUnknownModeFallsBackToSystem(t *testing.T) {
	root := newTestRoot(t)
	key, _, err := registry.CreateKey(root, proxyKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := key.SetStringValue(proxyModeValueName, "tunnel"); err != nil {
		key.Close()
		t.Fatalf("write mode: %v", err)
	}
	key.Close()

	pref := NewProxyPrefWithRoot(root)
	if got := pref.Mode(); got != ProxyModeSystem {
		t.Fatalf("Mode = %q, want %q for an unknown value", got, ProxyModeSystem)
	}

	pref.Set(ProxyMode("tunnel"), "http://proxy:8080", "alice")
	if got := pref.Mode(); got != ProxyModeSystem {
		t.Fatalf("Mode after Set with an unknown value = %q, want %q", got, ProxyModeSystem)
	}

	if got := ParseProxyMode(""); got != ProxyModeSystem {
		t.Errorf("ParseProxyMode(empty) = %q, want %q", got, ProxyModeSystem)
	}
}
