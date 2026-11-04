package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootUsesLocalAppData(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)

	got, err := Root()
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if want := filepath.Join(dir, "biowarden"); got != want {
		t.Fatalf("Root = %q, want %q", got, want)
	}
}

// With LOCALAPPDATA cleared the root comes from the user cache dir. On Windows
// the cache dir is LOCALAPPDATA itself, so there is nothing left to fall back
// to and Root has to report that instead of inventing a path.
func TestRootFallsBackToUserCacheDir(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)

	cacheDir, cacheErr := os.UserCacheDir()
	got, err := Root()
	if cacheErr != nil {
		if err == nil {
			t.Fatalf("Root = %q, want the cache dir error %v", got, cacheErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if want := filepath.Join(cacheDir, "biowarden"); got != want {
		t.Fatalf("Root = %q, want %q", got, want)
	}
}

func TestRootOrTemp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	if got, want := RootOrTemp(), filepath.Join(dir, "biowarden"); got != want {
		t.Fatalf("RootOrTemp = %q, want %q", got, want)
	}

	t.Setenv("LOCALAPPDATA", "")
	if _, err := os.UserCacheDir(); err == nil {
		t.Skip("the cache dir resolves here, so there is no fallback to test")
	}
	if got, want := RootOrTemp(), filepath.Join(os.TempDir(), "biowarden"); got != want {
		t.Fatalf("RootOrTemp = %q, want %q", got, want)
	}
}

// Every named entry is exactly one level under the root, so moving the root
// moves the whole layout.
func TestLayoutSitsUnderRoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	root := filepath.Join(dir, "biowarden")

	cases := []struct {
		name    string
		resolve func() (string, error)
	}{
		{HostLogName, HostLog},
		{GUILogName, GUILog},
		{ConnectionsDirName, Connections},
		{ManifestsDirName, Manifests},
	}
	for _, c := range cases {
		got, err := c.resolve()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if want := filepath.Join(root, c.name); got != want {
			t.Errorf("%s = %q, want %q", c.name, got, want)
		}
	}
}

func TestHelpersReportUnresolvableRoot(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	if _, err := os.UserCacheDir(); err == nil {
		t.Skip("the cache dir resolves here, so the root is never unresolvable")
	}
	if got, err := HostLog(); err == nil {
		t.Fatalf("HostLog = %q, want an error", got)
	}
}
