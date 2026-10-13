package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const hostName = "com.8bit.bitwarden"

const manifestDescription = "biowarden: Bitwarden browser bridge via Windows Hello"

// The official Bitwarden extension IDs, copied from the macOS reference
// (bw-bio-handler/manifests.go). Chrome and Edge allow them as origins;
// Firefox uses the extension id form.
var chromeOrigins = []string{
	"chrome-extension://nngceckbapebfimnlniiiahkandclblb/",
	"chrome-extension://jbkfoedolllekgbhcbcoahefnbanhhlh/",
	"chrome-extension://ccnckbpmaceehanjmeomladnmlffdjgn/",
}

var firefoxExtensions = []string{
	"{446900e4-71c2-419f-a6a7-df9c091e268b}",
}

// browserKeyPaths maps each browser to the HKCU subkey whose default value
// holds the manifest path.
var browserKeyPaths = map[Browser]string{
	Chrome:  `Software\Google\Chrome\NativeMessagingHosts\` + hostName,
	Edge:    `Software\Microsoft\Edge\NativeMessagingHosts\` + hostName,
	Firefox: `Software\Mozilla\NativeMessagingHosts\` + hostName,
}

// Options overrides the real machine state. Its zero value targets the current
// user's HKCU, %LOCALAPPDATA%\biowarden\manifests and this executable.
type Options struct {
	// Root is the key the registration keys live under; the zero Key means
	// HKCU. Tests pass a scratch subkey so they never read or write a real
	// browser's registration.
	Root registry.Key
	// ManifestDir is where the manifest JSON files are written; empty means
	// %LOCALAPPDATA%\biowarden\manifests.
	ManifestDir string
	// Executable is the host path recorded in the manifests; empty means
	// os.Executable().
	Executable string
}

type registrar struct {
	root        registry.Key
	manifestDir string
	executable  string
}

// New returns the HKCU backed registrar.
func New() Registrar { return NewWithOptions(Options{}) }

// NewWithOptions returns a registrar against injected roots, which is what
// makes the registry testable without touching a real browser.
func NewWithOptions(opts Options) Registrar {
	root := opts.Root
	if root == 0 {
		root = registry.CURRENT_USER
	}
	return &registrar{
		root:        root,
		manifestDir: opts.ManifestDir,
		executable:  opts.Executable,
	}
}

func (r *registrar) Status(b Browser) (Status, error) {
	path, ok, err := r.registeredPath(b)
	if err != nil {
		return Status{}, err
	}
	if !ok {
		return Status{}, nil
	}
	dir, err := r.manifestsDir()
	if err != nil {
		return Status{}, err
	}
	return Status{
		Registered: true,
		Path:       path,
		IsOurs:     path == r.manifestPath(b, dir),
	}, nil
}

func (r *registrar) Install(b Browser) error {
	keyPath, ok := browserKeyPaths[b]
	if !ok {
		return fmt.Errorf("registry: unsupported browser %q", b)
	}
	dir, err := r.manifestsDir()
	if err != nil {
		return err
	}
	exe, err := r.executablePath()
	if err != nil {
		return err
	}
	manifest, err := manifestJSON(b, exe)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("registry: cannot create manifest dir: %w", err)
	}
	manifestPath := r.manifestPath(b, dir)
	if err := writeFileAtomic(manifestPath, manifest); err != nil {
		return fmt.Errorf("registry: cannot write manifest: %w", err)
	}

	key, _, err := registry.CreateKey(r.root, keyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("registry: cannot create %s: %w", keyPath, err)
	}
	defer key.Close()
	if err := key.SetStringValue("", manifestPath); err != nil {
		return fmt.Errorf("registry: cannot write %s: %w", keyPath, err)
	}
	return nil
}

func (r *registrar) Uninstall(b Browser) error {
	keyPath, ok := browserKeyPaths[b]
	if !ok {
		return fmt.Errorf("registry: unsupported browser %q", b)
	}
	// Only clear a registration that points at our manifest: a value the
	// official client wrote must survive an uninstall.
	st, err := r.Status(b)
	if err != nil {
		return err
	}
	if st.Registered && st.IsOurs {
		key, err := registry.OpenKey(r.root, keyPath, registry.SET_VALUE)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("registry: cannot open %s: %w", keyPath, err)
		}
		if err == nil {
			err := key.DeleteValue("")
			key.Close()
			if err != nil && !errors.Is(err, registry.ErrNotExist) {
				return fmt.Errorf("registry: cannot delete %s: %w", keyPath, err)
			}
		}
	}

	dir, err := r.manifestsDir()
	if err != nil {
		return err
	}
	if err := os.Remove(r.manifestPath(b, dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("registry: cannot remove manifest: %w", err)
	}
	return nil
}

// registeredPath reads the manifest path from a browser's registration key,
// reporting false when the key or its default value is absent.
func (r *registrar) registeredPath(b Browser) (string, bool, error) {
	keyPath, ok := browserKeyPaths[b]
	if !ok {
		return "", false, fmt.Errorf("registry: unsupported browser %q", b)
	}
	key, err := registry.OpenKey(r.root, keyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("registry: cannot open %s: %w", keyPath, err)
	}
	defer key.Close()

	path, _, err := key.GetStringValue("")
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("registry: cannot read %s: %w", keyPath, err)
	}
	return path, path != "", nil
}

func (r *registrar) manifestsDir() (string, error) {
	if r.manifestDir != "" {
		return r.manifestDir, nil
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", errors.New("registry: LOCALAPPDATA is not set")
	}
	return filepath.Join(base, "biowarden", "manifests"), nil
}

func (r *registrar) executablePath() (string, error) {
	if r.executable != "" {
		return r.executable, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("registry: cannot locate own executable: %w", err)
	}
	return exe, nil
}

func (r *registrar) manifestPath(b Browser, dir string) string {
	return filepath.Join(dir, string(b)+".json")
}

func manifestJSON(b Browser, executable string) ([]byte, error) {
	m := map[string]any{
		"name":        hostName,
		"description": manifestDescription,
		"path":        executable,
		"type":        "stdio",
	}
	if b == Firefox {
		m["allowed_extensions"] = append([]string(nil), firefoxExtensions...)
	} else {
		m["allowed_origins"] = append([]string(nil), chromeOrigins...)
	}
	return json.MarshalIndent(m, "", "  ")
}

// writeFileAtomic writes through a temp file so a crash cannot leave a browser
// pointed at a half-written manifest.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
