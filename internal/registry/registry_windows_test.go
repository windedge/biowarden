package registry

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The registry is machine state, so the tests only ever write under a fresh
// HKCU\Software\Biowarden\Test\<random> key and a temp manifest dir; the real
// browser keys are never read, written or deleted.
type testEnv struct {
	reg  Registrar
	root registry.Key
	dir  string
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	rootPath := `Software\Biowarden\Test\` + hex.EncodeToString(b[:])
	root, _, err := registry.CreateKey(registry.CURRENT_USER, rootPath, registry.ALL_ACCESS)
	if err != nil {
		t.Fatalf("create test key %s: %v", rootPath, err)
	}
	t.Cleanup(func() {
		root.Close()
		deleteTree(registry.CURRENT_USER, rootPath)
	})

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

func deleteTree(root registry.Key, path string) {
	key, err := registry.OpenKey(root, path, registry.ALL_ACCESS)
	if err != nil {
		return
	}
	names, err := key.ReadSubKeyNames(-1)
	key.Close()
	if err != nil {
		return
	}
	for _, name := range names {
		deleteTree(root, path+`\`+name)
	}
	registry.DeleteKey(root, path)
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

// A value written by something else (for example the official desktop client)
// must be reported as not ours and left untouched by Uninstall.
func TestForeignRegistration(t *testing.T) {
	env := newTestEnv(t)
	foreign := filepath.Join(env.dir, "official-client.json")
	key, _, err := registry.CreateKey(env.root, browserKeyPaths[Chrome], registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create foreign key: %v", err)
	}
	if err := key.SetStringValue("", foreign); err != nil {
		t.Fatalf("write foreign value: %v", err)
	}
	key.Close()

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
	key, err = registry.OpenKey(env.root, browserKeyPaths[Chrome], registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("foreign key gone after Uninstall: %v", err)
	}
	defer key.Close()
	got, _, err := key.GetStringValue("")
	if err != nil || got != foreign {
		t.Fatalf("foreign value = %q, %v, want %q", got, err, foreign)
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
