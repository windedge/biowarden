//go:build windows

package registry

import (
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Tests must never write the user's real registry. RegLoadAppKey opens a hive
// file as a private root key: CreateKey and OpenKey work against it exactly as
// against HKCU, but everything it holds lives in a temp file and disappears
// when the test ends.
var (
	advapi32      = windows.NewLazySystemDLL("advapi32.dll")
	regLoadAppKey = advapi32.NewProc("RegLoadAppKeyW")
)

// newTestRoot returns a fresh, empty root key backed by a hive file under the
// test's temp dir. The caller writes paths relative to it and never touches
// HKCU.
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
	t.Cleanup(func() {
		root.Close()
	})
	return root
}
