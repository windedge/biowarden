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
	if got := pref.Host(); got != "" {
		t.Fatalf("Host on a fresh root = %q, want empty", got)
	}
	if got := pref.Port(); got != "" {
		t.Fatalf("Port on a fresh root = %q, want empty", got)
	}
	if got := pref.User(); got != "" {
		t.Fatalf("User on a fresh root = %q, want empty", got)
	}
}

func TestProxyPrefRoundTrip(t *testing.T) {
	root := newTestRoot(t)
	pref := NewProxyPrefWithRoot(root)
	pref.Set(ProxyModeSOCKS5, "127.0.0.1", "1080", "alice")

	if got := pref.Mode(); got != ProxyModeSOCKS5 {
		t.Errorf("Mode = %q, want %q", got, ProxyModeSOCKS5)
	}
	if got := pref.Host(); got != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", got)
	}
	if got := pref.Port(); got != "1080" {
		t.Errorf("Port = %q, want 1080", got)
	}
	if got := pref.User(); got != "alice" {
		t.Errorf("User = %q, want alice", got)
	}

	key, err := registry.OpenKey(root, proxyKeyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("open key: %v", err)
	}
	defer key.Close()
	stored, _, err := key.GetStringValue(proxyURLValueName)
	if err != nil {
		t.Fatalf("read ProxyURL: %v", err)
	}
	if stored != "127.0.0.1:1080" {
		t.Errorf("stored ProxyURL = %q, want host:port without a scheme", stored)
	}

	pref.Set(ProxyModeNone, "", "", "")
	if got := pref.Mode(); got != ProxyModeNone {
		t.Errorf("Mode after overwrite = %q, want %q", got, ProxyModeNone)
	}
	if got := pref.Host(); got != "" {
		t.Errorf("Host after overwrite = %q, want empty", got)
	}
	if got := pref.User(); got != "" {
		t.Errorf("User after overwrite = %q, want empty", got)
	}
}

func TestProxyPrefReset(t *testing.T) {
	root := newTestRoot(t)
	pref := NewProxyPrefWithRoot(root)
	pref.Set(ProxyModeHTTP, "proxy", "8080", "alice")
	pref.Reset()

	if got := pref.Mode(); got != ProxyModeSystem {
		t.Errorf("Mode after Reset = %q, want %q", got, ProxyModeSystem)
	}
	if got := pref.Host(); got != "" {
		t.Errorf("Host after Reset = %q, want empty", got)
	}
	if got := pref.Port(); got != "" {
		t.Errorf("Port after Reset = %q, want empty", got)
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

	pref.Set(ProxyMode("tunnel"), "proxy", "8080", "alice")
	if got := pref.Mode(); got != ProxyModeSystem {
		t.Fatalf("Mode after Set with an unknown value = %q, want %q", got, ProxyModeSystem)
	}

	if got := ParseProxyMode(""); got != ProxyModeSystem {
		t.Errorf("ParseProxyMode(empty) = %q, want %q", got, ProxyModeSystem)
	}
}

// Older builds stored a scheme in ProxyURL and named the choice manual,
// environment or off. Reads normalize those values without rewriting the key.
func TestProxyPrefMigratesLegacyValuesOnRead(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		address string
		want    ProxyMode
		host    string
		port    string
	}{
		{"manual http", "manual", "http://proxy:8080", ProxyModeHTTP, "proxy", "8080"},
		{"manual socks5", "manual", "socks5://127.0.0.1:1080", ProxyModeSOCKS5, "127.0.0.1", "1080"},
		{"manual socks5h", "manual", "socks5h://127.0.0.1:1080", ProxyModeSOCKS5, "127.0.0.1", "1080"},
		{"environment", "environment", "", ProxyModeSystem, "", ""},
		{"off", "off", "", ProxyModeNone, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newTestRoot(t)
			key, _, err := registry.CreateKey(root, proxyKeyPath, registry.SET_VALUE)
			if err != nil {
				t.Fatalf("create key: %v", err)
			}
			for name, value := range map[string]string{
				proxyModeValueName: tc.mode,
				proxyURLValueName:  tc.address,
				proxyUserValueName: "alice",
			} {
				if err := key.SetStringValue(name, value); err != nil {
					key.Close()
					t.Fatalf("write %s: %v", name, err)
				}
			}
			key.Close()

			pref := NewProxyPrefWithRoot(root)
			if got := pref.Mode(); got != tc.want {
				t.Errorf("Mode = %q, want %q", got, tc.want)
			}
			if got := pref.Host(); got != tc.host {
				t.Errorf("Host = %q, want %q", got, tc.host)
			}
			if got := pref.Port(); got != tc.port {
				t.Errorf("Port = %q, want %q", got, tc.port)
			}
			if got := pref.User(); got != "alice" {
				t.Errorf("User = %q, want alice", got)
			}
		})
	}
}
