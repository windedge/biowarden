package registry

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The registry is machine state, so the tests write only under a private app
// hive (newTestRoot) and a temp manifest dir; the real HKCU and the browser
// keys are never read, written or deleted.
type testEnv struct {
	reg  Registrar
	root registry.Key
	dir  string
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	root := newTestRoot(t)
	dir := t.TempDir()
	return testEnv{
		reg: NewWithOptions(Options{
			Root:        root,
			ManifestDir: dir,
			Executable:  `C:\test\biowarden.exe`,
		}),
		root: root,
		dir:  dir,
	}
}

func writeValue(t *testing.T, root registry.Key, keyPath, value string) {
	t.Helper()
	key, _, err := registry.CreateKey(root, keyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create %s: %v", keyPath, err)
	}
	if err := key.SetStringValue("", value); err != nil {
		key.Close()
		t.Fatalf("write %s: %v", keyPath, err)
	}
	key.Close()
}

func readValue(t *testing.T, root registry.Key, keyPath string) (string, bool) {
	t.Helper()
	key, err := registry.OpenKey(root, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer key.Close()
	value, _, err := key.GetStringValue("")
	if err != nil {
		return "", false
	}
	return value, true
}

func TestStatusUnregistered(t *testing.T) {
	env := newTestEnv(t)
	st, err := env.reg.Status(Chrome)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Registered || st.Path != "" || st.IsOurs {
		t.Fatalf("Status = %+v, want empty", st)
	}
}

func TestInstallThenStatus(t *testing.T) {
	env := newTestEnv(t)
	if err := env.reg.Install(Chrome); err != nil {
		t.Fatalf("Install: %v", err)
	}
	st, err := env.reg.Status(Chrome)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	want := filepath.Join(env.dir, "chrome.json")
	if !st.Registered || !st.IsOurs || st.Path != want {
		t.Fatalf("Status = %+v, want registered ours at %s", st, want)
	}

	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	if m["name"] != hostName || m["path"] != `C:\test\biowarden.exe` || m["type"] != "stdio" {
		t.Fatalf("manifest = %v", m)
	}
	origins, ok := m["allowed_origins"].([]any)
	if !ok || len(origins) != 3 {
		t.Fatalf("allowed_origins = %v, want 3", m["allowed_origins"])
	}
}

func TestInstallAllBrowsers(t *testing.T) {
	env := newTestEnv(t)
	for _, b := range Browsers {
		if err := env.reg.Install(b); err != nil {
			t.Fatalf("Install(%s): %v", b, err)
		}
		st, err := env.reg.Status(b)
		if err != nil {
			t.Fatalf("Status(%s): %v", b, err)
		}
		if !st.Registered || !st.IsOurs {
			t.Fatalf("Status(%s) = %+v, want ours", b, st)
		}
	}

	firefox, err := os.ReadFile(filepath.Join(env.dir, "firefox.json"))
	if err != nil {
		t.Fatalf("read firefox manifest: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(firefox, &m); err != nil {
		t.Fatalf("firefox manifest is not JSON: %v", err)
	}
	exts, ok := m["allowed_extensions"].([]any)
	if !ok || len(exts) != 1 || exts[0] != firefoxExtensions[0] {
		t.Fatalf("allowed_extensions = %v, want %v", m["allowed_extensions"], firefoxExtensions)
	}
	if _, ok := m["allowed_origins"]; ok {
		t.Fatalf("firefox manifest must not carry allowed_origins")
	}
}

func TestChromeInstallCoversBothKeys(t *testing.T) {
	env := newTestEnv(t)
	if err := env.reg.Install(Chrome); err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := filepath.Join(env.dir, "chrome.json")
	for _, keyPath := range browserKeyPaths[Chrome] {
		got, ok := readValue(t, env.root, keyPath)
		if !ok || got != want {
			t.Fatalf("key %s = %q, %v, want %q", keyPath, got, ok, want)
		}
	}
}

func TestChromeStatusAggregatesKeys(t *testing.T) {
	tests := []struct {
		name       string
		google     string
		chromium   string
		registered bool
		isOurs     bool
	}{
		{"none", "", "", false, false},
		{"google ours", "our", "", true, true},
		{"chromium ours", "", "our", true, true},
		{"both foreign", "foreign-google", "foreign-chromium", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			ourPath := filepath.Join(env.dir, "chrome.json")
			values := []string{tt.google, tt.chromium}
			for i, keyPath := range browserKeyPaths[Chrome] {
				v := values[i]
				if v == "" {
					continue
				}
				if v == "our" {
					v = ourPath
				}
				writeValue(t, env.root, keyPath, v)
			}

			st, err := env.reg.Status(Chrome)
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if st.Registered != tt.registered || st.IsOurs != tt.isOurs {
				t.Fatalf("Status = %+v, want registered=%v ours=%v", st, tt.registered, tt.isOurs)
			}
			if tt.isOurs && st.Path != ourPath {
				t.Fatalf("Status.Path = %q, want %q", st.Path, ourPath)
			}
			// The Google key wins when both keys hold a foreign value.
			if !tt.isOurs && tt.google != "" && st.Path != "foreign-google" {
				t.Fatalf("Status.Path = %q, want foreign-google", st.Path)
			}
			if !tt.isOurs && tt.google == "" && tt.chromium != "" && st.Path != "foreign-chromium" {
				t.Fatalf("Status.Path = %q, want foreign-chromium", st.Path)
			}
		})
	}
}

func TestChromeUninstallClearsBothKeys(t *testing.T) {
	env := newTestEnv(t)
	if err := env.reg.Install(Chrome); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := env.reg.Uninstall(Chrome); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	for _, keyPath := range browserKeyPaths[Chrome] {
		if _, ok := readValue(t, env.root, keyPath); ok {
			t.Fatalf("key %s still set after Uninstall", keyPath)
		}
	}
}

func TestChromeUninstallSkipsForeignAndMissing(t *testing.T) {
	env := newTestEnv(t)
	foreignGoogle := filepath.Join(env.dir, "official-google.json")
	foreignChromium := filepath.Join(env.dir, "official-chromium.json")
	writeValue(t, env.root, browserKeyPaths[Chrome][0], foreignGoogle)
	writeValue(t, env.root, browserKeyPaths[Chrome][1], foreignChromium)

	if err := env.reg.Uninstall(Chrome); err != nil {
		t.Fatalf("Uninstall foreign: %v", err)
	}
	if got, _ := readValue(t, env.root, browserKeyPaths[Chrome][0]); got != foreignGoogle {
		t.Fatalf("google value = %q, want untouched %q", got, foreignGoogle)
	}
	if got, _ := readValue(t, env.root, browserKeyPaths[Chrome][1]); got != foreignChromium {
		t.Fatalf("chromium value = %q, want untouched %q", got, foreignChromium)
	}

	// Idempotent: a second uninstall with nothing of ours present is fine.
	if err := env.reg.Uninstall(Chrome); err != nil {
		t.Fatalf("Uninstall again: %v", err)
	}
}

// A value written by something else (for example the official desktop client)
// must be reported as not ours and left untouched by Uninstall.
func TestForeignRegistration(t *testing.T) {
	env := newTestEnv(t)
	foreign := filepath.Join(env.dir, "official-client.json")
	writeValue(t, env.root, browserKeyPaths[Chrome][0], foreign)

	st, err := env.reg.Status(Chrome)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Registered || st.IsOurs || st.Path != foreign {
		t.Fatalf("Status = %+v, want registered not ours at %s", st, foreign)
	}

	if err := env.reg.Uninstall(Chrome); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	got, ok := readValue(t, env.root, browserKeyPaths[Chrome][0])
	if !ok || got != foreign {
		t.Fatalf("foreign value = %q, %v, want %q", got, ok, foreign)
	}
}

// Our manifest file outlives a registration, which is how the GUI tells a
// browser that once had biowarden from one it never touched.
func TestStatusOursInstalled(t *testing.T) {
	t.Run("manifest present and key ours", func(t *testing.T) {
		env := newTestEnv(t)
		if err := env.reg.Install(Chrome); err != nil {
			t.Fatalf("Install: %v", err)
		}
		st, err := env.reg.Status(Chrome)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if !st.IsOurs || !st.OursInstalled {
			t.Fatalf("Status = %+v, want ours and oursInstalled", st)
		}
	})

	t.Run("manifest present but key foreign", func(t *testing.T) {
		env := newTestEnv(t)
		manifest := filepath.Join(env.dir, "chrome.json")
		if err := os.WriteFile(manifest, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		foreign := filepath.Join(env.dir, "official-client.json")
		writeValue(t, env.root, browserKeyPaths[Chrome][0], foreign)

		st, err := env.reg.Status(Chrome)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if st.IsOurs || !st.OursInstalled {
			t.Fatalf("Status = %+v, want foreign with oursInstalled", st)
		}
	})

	t.Run("manifest absent and key foreign", func(t *testing.T) {
		env := newTestEnv(t)
		foreign := filepath.Join(env.dir, "official-client.json")
		writeValue(t, env.root, browserKeyPaths[Chrome][0], foreign)

		st, err := env.reg.Status(Chrome)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if st.IsOurs || st.OursInstalled {
			t.Fatalf("Status = %+v, want foreign without oursInstalled", st)
		}
	})
}

// A hijacked browser has our manifest file on disk while the key points at
// another client: Uninstall must drop our file and leave the foreign value.
func TestUninstallHijackedKeepsForeignRemovesManifest(t *testing.T) {
	env := newTestEnv(t)
	manifest := filepath.Join(env.dir, "chrome.json")
	if err := os.WriteFile(manifest, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	foreign := filepath.Join(env.dir, "official-client.json")
	writeValue(t, env.root, browserKeyPaths[Chrome][0], foreign)

	if err := env.reg.Uninstall(Chrome); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(manifest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("our manifest still present: %v", err)
	}
	if got, _ := readValue(t, env.root, browserKeyPaths[Chrome][0]); got != foreign {
		t.Fatalf("foreign value = %q, want untouched %q", got, foreign)
	}
}

func TestUninstall(t *testing.T) {
	env := newTestEnv(t)
	if err := env.reg.Install(Edge); err != nil {
		t.Fatalf("Install: %v", err)
	}
	st, err := env.reg.Status(Edge)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	manifest := st.Path

	if err := env.reg.Uninstall(Edge); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	st, err = env.reg.Status(Edge)
	if err != nil {
		t.Fatalf("Status after Uninstall: %v", err)
	}
	if st.Registered || st.IsOurs {
		t.Fatalf("Status after Uninstall = %+v, want empty", st)
	}
	if _, err := os.Stat(manifest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest still present: %v", err)
	}
}

func TestUninstallMissing(t *testing.T) {
	env := newTestEnv(t)
	if err := env.reg.Uninstall(Firefox); err != nil {
		t.Fatalf("Uninstall missing = %v, want nil", err)
	}
}

func TestUnsupportedBrowser(t *testing.T) {
	env := newTestEnv(t)
	if err := env.reg.Install(Browser("opera")); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("Install unsupported = %v, want unsupported error", err)
	}
}
