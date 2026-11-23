//go:build windows

package sysproxy

import (
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Copied from internal/registry/apphive_windows_test.go. RegLoadAppKey backs the
// root with a private hive file, so these tests never write the real HKCU.
var (
	advapi32      = windows.NewLazySystemDLL("advapi32.dll")
	regLoadAppKey = advapi32.NewProc("RegLoadAppKeyW")
)

func newTestRoot(t *testing.T) registry.Key {
	t.Helper()
	hive := filepath.Join(t.TempDir(), "test.hive")
	path, err := windows.UTF16PtrFromString(hive)
	if err != nil {
		t.Fatalf("hive path: %v", err)
	}

	var handle windows.Handle
	ret, _, _ := regLoadAppKey.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&handle)),
		uintptr(registry.ALL_ACCESS),
		0,
		0,
	)
	if ret != 0 {
		t.Fatalf("RegLoadAppKey(%s) = %#x", hive, ret)
	}
	root := registry.Key(handle)
	t.Cleanup(func() { root.Close() })
	return root
}

func TestWindowsReaderSystem(t *testing.T) {
	const single = "127.0.0.1:7890"
	cases := []struct {
		name   string
		setup  func(*testing.T, registry.Key)
		want   Config
		wantOK bool
	}{
		{"key missing", nil, Config{}, false},
		{"proxy disabled", func(t *testing.T, root registry.Key) {
			setDWordValue(t, root, "ProxyEnable", 0)
			setStringValue(t, root, "ProxyServer", single)
		}, Config{}, false},
		{"enable missing", func(t *testing.T, root registry.Key) {
			setStringValue(t, root, "ProxyServer", single)
		}, Config{}, false},
		{"single address", func(t *testing.T, root registry.Key) {
			setDWordValue(t, root, "ProxyEnable", 1)
			setStringValue(t, root, "ProxyServer", single)
		}, Config{HTTP: single, HTTPS: single}, true},
		{"per protocol", func(t *testing.T, root registry.Key) {
			setDWordValue(t, root, "ProxyEnable", 1)
			setStringValue(t, root, "ProxyServer", "http=127.0.0.1:8080;https=127.0.0.1:8443;socks=127.0.0.1:1080")
		}, Config{HTTP: "127.0.0.1:8080", HTTPS: "127.0.0.1:8443", SOCKS: "127.0.0.1:1080"}, true},
		{"server missing", func(t *testing.T, root registry.Key) {
			setDWordValue(t, root, "ProxyEnable", 1)
		}, Config{}, false},
		{"server empty", func(t *testing.T, root registry.Key) {
			setDWordValue(t, root, "ProxyEnable", 1)
			setStringValue(t, root, "ProxyServer", "")
		}, Config{}, false},
		{"override round trip", func(t *testing.T, root registry.Key) {
			setDWordValue(t, root, "ProxyEnable", 1)
			setStringValue(t, root, "ProxyServer", single)
			setStringValue(t, root, "ProxyOverride", "*.local;<local>")
		}, Config{HTTP: single, HTTPS: single, Override: "*.local;<local>"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newTestRoot(t)
			if tc.setup != nil {
				tc.setup(t, root)
			}
			got, ok := NewWindowsReaderWithRoot(root).System()
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("System() = (%+v, %v), want (%+v, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestSupported(t *testing.T) {
	if !Supported() {
		t.Fatal("Supported() = false on Windows")
	}
}

func setDWordValue(t *testing.T, root registry.Key, name string, value uint32) {
	t.Helper()
	key, _, err := registry.CreateKey(root, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create settings key: %v", err)
	}
	defer key.Close()
	if err := key.SetDWordValue(name, value); err != nil {
		t.Fatalf("set %s: %v", name, err)
	}
}

func setStringValue(t *testing.T, root registry.Key, name, value string) {
	t.Helper()
	key, _, err := registry.CreateKey(root, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create settings key: %v", err)
	}
	defer key.Close()
	if err := key.SetStringValue(name, value); err != nil {
		t.Fatalf("set %s: %v", name, err)
	}
}
