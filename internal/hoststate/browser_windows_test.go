//go:build windows

package hoststate

import (
	"strings"
	"testing"
)

// The toolhelp path is the one piece that needs a real parent: the test binary
// is started by go.exe (or an IDE), so a name must come back even though it is
// not a browser.
func TestParentExeNameResolves(t *testing.T) {
	name, err := parentExeName()
	if err != nil {
		t.Fatalf("parentExeName: %v", err)
	}
	if name == "" {
		t.Fatal("parentExeName returned an empty name")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
		t.Fatalf("parent exe = %q, want an .exe name", name)
	}
}

func TestDetectBrowserNeverPanics(t *testing.T) {
	// The parent here is a test runner, so the id is "" or a known browser on
	// a developer machine whose browser launched the test by hand. Either is
	// valid; the point is that reading the parent is safe.
	_ = DetectBrowser()
}
